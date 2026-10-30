// Package checkcsv reads the canonical check-register CSV.
//
// Customer-specific column names are not interpreted. A later map should
// rewrite a customer's header onto CanonicalColumns (or project each record
// onto Record) and then use this package. Until that exists, unknown columns
// are rejected.
package checkcsv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxAccountNumber = 34
	maxCheckNumber   = 32
	maxPayee         = 140
)

// Rejection codes stored for a data row. They match check_register_rejection_codes.
const (
	CodeInvalidRow         = "invalid_row"
	CodeInvalidRouting     = "invalid_routing"
	CodeInvalidAccount     = "invalid_account"
	CodeInvalidCheckNumber = "invalid_check_number"
	CodeInvalidAmount      = "invalid_amount"
	CodeInvalidPayee       = "invalid_payee"
	CodeInvalidDate        = "invalid_date"
	CodeInvalidVoid        = "invalid_void"
)

// CanonicalColumns is the ingest header. Order in the file may vary.
// issue_date and void cells may be blank; the columns themselves are required
// so a later column map has a stable target.
var CanonicalColumns = []string{
	"routing_number",
	"account_number",
	"check_number",
	"amount",
	"payee",
	"issue_date",
	"void",
}

// FieldError is one data row that cannot become a register item.
type FieldError struct {
	Code    string
	Message string
}

func (e *FieldError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Record is one canonical data row. Row is 1-based among data records; the
// header is not a row. FieldErr is set when the row must be rejected.
type Record struct {
	Row           int
	RoutingNumber string
	AccountNumber string
	CheckNumber   string
	AmountMinor   int64
	Payee         string
	IssueDate     *time.Time
	Void          bool
	FieldErr      *FieldError
}

// Reader streams canonical records from a CSV file.
type Reader struct {
	csv *csv.Reader
	idx map[string]int
	row int
}

// ValidateHeader reports whether line is the canonical header.
// A UTF-8 BOM and one trailing newline are ignored. The stored file should
// still keep those bytes; callers validate a copy of the first line.
func ValidateHeader(line string) error {
	line = strings.TrimPrefix(line, "\uFEFF")
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return errors.New("csv header is empty")
	}
	r := csv.NewReader(strings.NewReader(line))
	r.FieldsPerRecord = -1
	fields, err := r.Read()
	if err != nil {
		return fmt.Errorf("csv header: %w", err)
	}
	return validateFields(fields)
}

// NewReader reads and checks the header, then returns a data-row reader.
func NewReader(r io.Reader) (*Reader, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = false
	fields, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("csv header: %w", err)
	}
	if err := validateFields(fields); err != nil {
		return nil, err
	}
	idx := make(map[string]int, len(fields))
	for i, f := range fields {
		idx[normHeader(f)] = i
	}
	cr.FieldsPerRecord = len(fields)
	return &Reader{csv: cr, idx: idx}, nil
}

// Read returns the next data row. io.EOF ends the file. A non-nil error other
// than a FieldErr on the record means the file can no longer be split into
// rows (for example a broken quote). A column-count mismatch is a FieldErr
// so the caller can keep going.
func (r *Reader) Read() (Record, error) {
	fields, err := r.csv.Read()
	if errors.Is(err, io.EOF) {
		return Record{}, io.EOF
	}
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) && errors.Is(parseErr.Err, csv.ErrFieldCount) {
		r.row++
		return Record{
			Row: r.row,
			FieldErr: &FieldError{
				Code:    CodeInvalidRow,
				Message: "column count does not match the header",
			},
		}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("csv row: %w", err)
	}
	r.row++
	return parseRecord(r.row, fields, r.idx), nil
}

func validateFields(fields []string) error {
	seen := make(map[string]struct{}, len(fields))
	var unknown []string
	for _, f := range fields {
		name := normHeader(f)
		if name == "" {
			return errors.New("csv header has an empty column")
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("csv header repeats %q", name)
		}
		seen[name] = struct{}{}
		if !canonical(name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown column %q; column mapping is not supported", unknown[0])
	}
	for _, name := range CanonicalColumns {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("csv header is missing %q", name)
		}
	}
	return nil
}

func canonical(name string) bool {
	for _, c := range CanonicalColumns {
		if c == name {
			return true
		}
	}
	return false
}

func normHeader(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\uFEFF")))
}

func parseRecord(row int, fields []string, idx map[string]int) Record {
	rec := Record{Row: row}
	rec.RoutingNumber = cell(fields, idx, "routing_number")
	rec.AccountNumber = cell(fields, idx, "account_number")
	rec.CheckNumber = cell(fields, idx, "check_number")
	rec.Payee = cell(fields, idx, "payee")
	amountRaw := cell(fields, idx, "amount")
	issueRaw := cell(fields, idx, "issue_date")
	voidRaw := cell(fields, idx, "void")

	if len(rec.RoutingNumber) != 9 || !digits(rec.RoutingNumber) {
		rec.FieldErr = &FieldError{Code: CodeInvalidRouting, Message: "routing number must be 9 digits"}
		return rec
	}
	if rec.AccountNumber == "" || utf8.RuneCountInString(rec.AccountNumber) > maxAccountNumber {
		rec.FieldErr = &FieldError{Code: CodeInvalidAccount, Message: "account number is required and must be at most 34 characters"}
		return rec
	}
	if rec.CheckNumber == "" || utf8.RuneCountInString(rec.CheckNumber) > maxCheckNumber {
		rec.FieldErr = &FieldError{Code: CodeInvalidCheckNumber, Message: "check number is required and must be at most 32 characters"}
		return rec
	}
	amount, err := parseAmount(amountRaw)
	if err != nil {
		rec.FieldErr = &FieldError{Code: CodeInvalidAmount, Message: "amount must be a non-negative decimal with at most 2 fractional digits"}
		return rec
	}
	rec.AmountMinor = amount
	if rec.Payee == "" || utf8.RuneCountInString(rec.Payee) > maxPayee {
		rec.FieldErr = &FieldError{Code: CodeInvalidPayee, Message: "payee is required and must be at most 140 characters"}
		return rec
	}
	if issueRaw != "" {
		t, err := time.Parse("2006-01-02", issueRaw)
		if err != nil {
			rec.FieldErr = &FieldError{Code: CodeInvalidDate, Message: "issue date must be YYYY-MM-DD"}
			return rec
		}
		rec.IssueDate = &t
	}
	if voidRaw != "" {
		v, err := parseVoid(voidRaw)
		if err != nil {
			rec.FieldErr = &FieldError{Code: CodeInvalidVoid, Message: "void must be true, false, yes, no, y, n, 1, or 0"}
			return rec
		}
		rec.Void = v
	}
	return rec
}

func cell(fields []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[i])
}

// parseAmount interprets a canonical amount as major currency units and
// returns minor units. "100", "100.5", "100.50", and "$100.50" are accepted.
// Thousands separators are not; a later column map can strip them.
func parseAmount(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") || strings.ContainsAny(s, ",") {
		return 0, errors.New("amount")
	}
	dollarsRaw, centsRaw, hasDot := strings.Cut(s, ".")
	if strings.Contains(centsRaw, ".") {
		return 0, errors.New("amount")
	}
	if dollarsRaw == "" {
		dollarsRaw = "0"
	}
	if !digits(dollarsRaw) {
		return 0, errors.New("amount")
	}
	cents := "00"
	if hasDot {
		switch len(centsRaw) {
		case 0:
			return 0, errors.New("amount")
		case 1:
			cents = centsRaw + "0"
		case 2:
			cents = centsRaw
		default:
			return 0, errors.New("amount")
		}
		if !digits(cents) {
			return 0, errors.New("amount")
		}
	}
	dollars, err := strconv.ParseInt(dollarsRaw, 10, 64)
	if err != nil {
		return 0, errors.New("amount")
	}
	frac, err := strconv.ParseInt(cents, 10, 64)
	if err != nil {
		return 0, errors.New("amount")
	}
	if dollars > (math.MaxInt64-frac)/100 {
		return 0, errors.New("amount")
	}
	return dollars*100 + frac, nil
}

func parseVoid(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "y", "1":
		return true, nil
	case "false", "no", "n", "0":
		return false, nil
	default:
		return false, errors.New("void")
	}
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
