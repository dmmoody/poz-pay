// Package registerupload accepts a customer's canonical check-register CSV,
// parses it, and asks a database for CRUD of the resulting rows.
//
// Accept reads the file, checks the header, hashes and splits the bytes, then
// inserts the intake row, the chunks, and an outbox message. A broker wakes a
// worker. The worker reads those chunks, parses the rows, and inserts checks
// or rejections. The database package does not receive or parse the file.
// RabbitMQ delivers the wake-up today. Consume accepts any Listener, so
// another broker (for example Google Pub/Sub) can replace it without
// changing ingest.
//
// Column mapping is intentionally absent. Files must already use the
// canonical header in checkcsv. A later map should rewrite the customer's
// header before Accept / the worker's reader.
package registerupload

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkcsv"
	"github.com/dmmoody/poz-pay/internal/payee"
	"github.com/dmmoody/poz-pay/internal/store"
)

const (
	// TopicUploadAccepted is the broker routing key for one stored CSV.
	// The payload is JSON: {"upload_id":"<uuid>"}.
	TopicUploadAccepted = "check_register.upload.accepted"

	// DefaultMaxUploadBytes is the accepted file size when unset (64 MiB).
	DefaultMaxUploadBytes int64 = 64 << 20

	commitBatchSize = 200
	maxHeaderBytes  = 64 << 10
)

// Stored rejection codes that are not produced by the CSV parser.
const (
	CodeRoutingMismatch = "routing_mismatch"
	CodeUnknownAccount  = "unknown_account"
	CodeDuplicateCheck  = "duplicate_check"
)

var (
	// ErrFIInactive means the institution exists but cannot take new registers.
	ErrFIInactive = errors.New("financial institution is inactive")

	// ErrRoutingNumberRequired means the institution row has no routing number.
	ErrRoutingNumberRequired = errors.New("financial institution has no routing number")

	// ErrInvalidCSV means the file is not a canonical check-register CSV.
	ErrInvalidCSV = errors.New("invalid check register csv")

	// ErrTooLarge means the file exceeds the configured limit.
	ErrTooLarge = errors.New("check register file exceeds the upload limit")

	// ErrUploadNotFound means the upload is not under this institution and customer.
	ErrUploadNotFound = store.ErrNotFound

	// ErrBusy means another worker holds the upload. The delivery should be retried.
	ErrBusy = errors.New("check register upload is already being ingested")

	// ErrAlreadyComplete means ingest finished; the delivery can be acknowledged.
	ErrAlreadyComplete = errors.New("check register upload is already complete")

	// ErrAlreadyFailed means ingest stopped on a broken file; do not redeliver.
	ErrAlreadyFailed = errors.New("check register upload already failed")

	// errFileStopped means the file was marked failed and the delivery can be acknowledged.
	errFileStopped = errors.New("check register file stopped")
)

// Customer is one party under one financial institution.
type Customer = store.Customer

// ConstraintError is a class-23 integrity failure from a register insert.
type ConstraintError = store.ConstraintError

// Intake is the row inserted once a file has been accepted.
type Intake = store.Intake

// IntakeRow is one stored file locked for parsing.
type IntakeRow = store.IntakeRow

// Rejection is one data row that did not become a register item.
type Rejection = store.Rejection

// Upload is the intake record for one file.
type Upload = store.Upload

// Records is CRUD for an accepted file.
type Records = store.RegisterUploads

// Tx is one database transaction.
type Tx = store.UploadTx

// Conn is one database connection holding the ingest lock.
type Conn = store.UploadConn

// Tenants loads the institution, customer, and account numbers an upload is filed under.
type Tenants interface {
	Customer(ctx context.Context, fiID, partyID string) (Customer, error)
	Accounts(ctx context.Context, fiID, partyID string) (map[string]string, error)
}

// CommittedRow is one parsed data row. Exactly one of Item or Rejection is set.
// NormalizedPayee is the compare form of Item.Payee.
type CommittedRow struct {
	Row             int
	Item            *store.UploadItem
	NormalizedPayee string
	Rejection       *Rejection
}

// AcceptInput is one CSV upload from a customer of an institution.
type AcceptInput struct {
	FIID     string
	PartyID  string
	Filename string
	Body     io.Reader
}

// Service accepts files, parses them, and persists the rows.
type Service struct {
	tenants   Tenants
	records   Records
	maxBytes  int64
	batchSize int
}

func NewService(tenants Tenants, records Records, maxBytes int64) *Service {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxUploadBytes
	}
	return &Service{tenants: tenants, records: records, maxBytes: maxBytes, batchSize: commitBatchSize}
}

// Accept stores a canonical CSV for the customer and enqueues ingest.
func (s *Service) Accept(ctx context.Context, in AcceptInput) (Upload, error) {
	customer, err := s.tenants.Customer(ctx, in.FIID, in.PartyID)
	if err != nil {
		return Upload{}, err
	}
	if !customer.FIActive {
		return Upload{}, ErrFIInactive
	}
	if !customer.PartyActive {
		return Upload{}, store.ErrPartyNotFound
	}
	if customer.RoutingNumber == "" {
		return Upload{}, ErrRoutingNumberRequired
	}

	br := bufio.NewReaderSize(in.Body, maxHeaderBytes)
	header, err := readHeaderLine(br)
	if err != nil {
		return Upload{}, err
	}
	if err := checkcsv.ValidateHeader(string(header)); err != nil {
		return Upload{}, invalidCSV("%s", err.Error())
	}
	id, err := newUploadID()
	if err != nil {
		return Upload{}, err
	}
	payload, err := acceptedPayload(id)
	if err != nil {
		return Upload{}, err
	}
	return s.save(ctx, Intake{
		ID:            id,
		FIID:          in.FIID,
		PartyID:       in.PartyID,
		RoutingNumber: customer.RoutingNumber,
		Filename:      cleanFilename(in.Filename),
	}, payload, limitBody(io.MultiReader(bytes.NewReader(header), br), s.maxBytes))
}

// Get returns one upload scoped to the institution and customer.
func (s *Service) Get(ctx context.Context, fiID, partyID, uploadID string) (Upload, error) {
	if _, err := s.tenants.Customer(ctx, fiID, partyID); err != nil {
		return Upload{}, err
	}
	return s.records.Find(ctx, fiID, partyID, uploadID)
}

// Ingest writes register items for one upload. A duplicate delivery resumes
// from the last committed row. ErrBusy, and any other error except the
// already-finished sentinels, means the caller should redeliver.
func (s *Service) Ingest(ctx context.Context, uploadID string) error {
	return s.records.WithinConn(ctx, func(ctx context.Context, conn Conn) error {
		ok, err := conn.TryLock(ctx, uploadID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrBusy
		}
		defer func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = conn.Unlock(unlockCtx, uploadID)
		}()

		row, err := s.claim(ctx, conn, uploadID)
		if err != nil {
			return err
		}
		reader, err := checkcsv.NewReader(&chunkReader{ctx: ctx, conn: conn, id: uploadID})
		if err != nil {
			if ferr := conn.MarkFailed(ctx, uploadID, failureReason("stored file is not a canonical check-register csv")); ferr != nil {
				return ferr
			}
			log.Printf("check register upload %s failed: %v", uploadID, err)
			return nil
		}
		accounts, err := s.tenants.Accounts(ctx, row.FIID, row.PartyID)
		if err != nil {
			return err
		}
		if err := s.parse(ctx, conn, uploadID, row, accounts, reader); err != nil {
			if errors.Is(err, errFileStopped) {
				return nil
			}
			return err
		}
		if err := conn.MarkCompleted(ctx, uploadID); err != nil {
			return err
		}
		log.Printf("check register upload %s ingested", uploadID)
		return nil
	})
}

func (s *Service) claim(ctx context.Context, conn Conn, uploadID string) (IntakeRow, error) {
	var row IntakeRow
	err := conn.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		locked, err := tx.LockIntake(ctx, uploadID)
		if err != nil {
			return err
		}
		if err := InterpretStatus(locked.Status); err != nil {
			return err
		}
		if locked.BatchID == "" {
			batchID, err := tx.InsertBatch(ctx, locked.FIID, locked.PartyID, "csv")
			if err != nil {
				return err
			}
			locked.BatchID = batchID
		}
		if err := tx.SetIngesting(ctx, uploadID, locked.BatchID); err != nil {
			return err
		}
		row = locked
		return nil
	})
	return row, err
}

func (s *Service) parse(ctx context.Context, conn Conn, uploadID string, row IntakeRow, accounts map[string]string, reader *checkcsv.Reader) error {
	skipped := 0
	lastRow := row.RowsProcessed
	batch := make([]CommittedRow, 0, s.batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := conn.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
			return ApplyBatch(ctx, rowWriter{tx: tx, id: uploadID, row: row}, lastRow, batch, MaxStoredRejections)
		})
		if err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ferr := conn.MarkFailed(ctx, uploadID, failureReason("csv is not readable past the last committed row")); ferr != nil {
				return ferr
			}
			log.Printf("check register upload %s failed: %v", uploadID, err)
			return errFileStopped
		}
		if skipped < row.RowsProcessed {
			skipped++
			continue
		}
		lastRow = rec.Row
		batch = append(batch, classify(rec, row.RoutingNumber, accounts))
		if len(batch) >= s.batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func classify(rec checkcsv.Record, routingNumber string, accounts map[string]string) CommittedRow {
	if rec.FieldErr != nil {
		return CommittedRow{
			Row: rec.Row,
			Rejection: &Rejection{
				Row:     rec.Row,
				Code:    rec.FieldErr.Code,
				Message: rec.FieldErr.Message,
			},
		}
	}
	if rec.RoutingNumber != routingNumber {
		return CommittedRow{
			Row: rec.Row,
			Rejection: &Rejection{
				Row:     rec.Row,
				Code:    CodeRoutingMismatch,
				Message: "routing number does not match the financial institution",
			},
		}
	}
	accountID, ok := accounts[rec.AccountNumber]
	if !ok {
		return CommittedRow{
			Row: rec.Row,
			Rejection: &Rejection{
				Row:     rec.Row,
				Code:    CodeUnknownAccount,
				Message: "account number is not on this customer",
			},
		}
	}
	item := store.UploadItem{
		AccountID:   accountID,
		CheckNumber: rec.CheckNumber,
		Amount:      rec.AmountMinor,
		Payee:       rec.Payee,
		IssueDate:   rec.IssueDate,
		Void:        rec.Void,
	}
	return CommittedRow{
		Row:             rec.Row,
		Item:            &item,
		NormalizedPayee: payee.Normalize(rec.Payee),
	}
}

func readHeaderLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		frag, err := br.ReadSlice('\n')
		line = append(line, frag...)
		if len(line) > maxHeaderBytes {
			return nil, invalidCSV("header exceeds %d bytes", maxHeaderBytes)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if len(bytes.TrimSpace(line)) == 0 {
				return nil, invalidCSV("file is empty")
			}
			return line, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read csv header: %w", err)
		}
		return line, nil
	}
}

func cleanFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "register.csv"
	}
	runes := []rune(name)
	if len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

type csvError struct{ msg string }

func (e *csvError) Error() string { return e.msg }

func (e *csvError) Unwrap() error { return ErrInvalidCSV }

func invalidCSV(format string, args ...any) error {
	return &csvError{msg: fmt.Sprintf(format, args...)}
}
