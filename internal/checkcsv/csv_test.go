package checkcsv

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

const header = "routing_number,account_number,check_number,amount,payee,issue_date,void\n"

func TestValidateHeader(t *testing.T) {
	t.Parallel()
	if err := ValidateHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHeader("\uFEFF" + header); err != nil {
		t.Fatal(err)
	}
	reordered := "payee,void,issue_date,amount,check_number,account_number,routing_number\n"
	if err := ValidateHeader(reordered); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"",
		"routing_number,account_number\n",
		"routing_number,account_number,check_number,amount,payee,issue_date,void,memo\n",
		"routing_number,routing_number,check_number,amount,payee,issue_date,void\n",
	}
	for _, c := range cases {
		if err := ValidateHeader(c); err == nil {
			t.Fatalf("ValidateHeader(%q) succeeded", c)
		}
	}
	err := ValidateHeader("aba,account_number,check_number,amount,payee,issue_date,void\n")
	if err == nil || !strings.Contains(err.Error(), "column mapping") {
		t.Fatalf("err = %v", err)
	}
}

func TestRead_canonicalAndFieldErrors(t *testing.T) {
	t.Parallel()
	body := header + strings.Join([]string{
		"011000015,100200300,01001,100.50,ACME SUPPLIES LLC,2026-10-01,false",
		"011000015,100200300,01002,$12.5,\"Smith, Jones & Co\",2026-10-02,yes",
		"011000015,100200300,01003,0,ZERO PAYEE,,",
		"not-aba,100200300,01004,1.00,BAD ROUTING,2026-10-01,false",
		"011000015,100200300,01005,1.234,BAD AMOUNT,2026-10-01,false",
		"011000015,100200300,01006,1.00,BAD VOID,2026-10-01,maybe",
		"011000015,100200300,01007",
	}, "\n") + "\n"

	r, err := NewReader(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	first, err := r.Read()
	if err != nil || first.FieldErr != nil {
		t.Fatalf("first = %+v %v", first, err)
	}
	if first.Row != 1 || first.AmountMinor != 10050 || first.CheckNumber != "01001" || first.Void || first.IssueDate == nil {
		t.Fatalf("first = %+v", first)
	}

	second, err := r.Read()
	if err != nil || second.FieldErr != nil {
		t.Fatalf("second = %+v %v", second, err)
	}
	if second.AmountMinor != 1250 || second.Payee != "Smith, Jones & Co" || !second.Void {
		t.Fatalf("second = %+v", second)
	}

	third, err := r.Read()
	if err != nil || third.FieldErr != nil || third.AmountMinor != 0 || third.IssueDate != nil || third.Void {
		t.Fatalf("third = %+v %v", third, err)
	}

	wantCodes := []string{CodeInvalidRouting, CodeInvalidAmount, CodeInvalidVoid, CodeInvalidRow}
	for i, code := range wantCodes {
		rec, err := r.Read()
		if err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
		if rec.FieldErr == nil || rec.FieldErr.Code != code {
			t.Fatalf("row %d = %+v", i, rec.FieldErr)
		}
	}
	if _, err := r.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("eof = %v", err)
	}
}

func TestParseAmount(t *testing.T) {
	t.Parallel()
	ok := map[string]int64{
		"0": 0, "0.00": 0, "100": 10000, "100.5": 10050, "100.50": 10050,
		"$100.50": 10050, ".50": 50, "00.05": 5,
	}
	for raw, want := range ok {
		got, err := parseAmount(raw)
		if err != nil || got != want {
			t.Fatalf("parseAmount(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "-1", "1,000.00", "1.234", "abc", "1.", "1.2.3"} {
		if _, err := parseAmount(raw); err == nil {
			t.Fatalf("parseAmount(%q) succeeded", raw)
		}
	}
}

func TestNewReader_quotedNewline(t *testing.T) {
	t.Parallel()
	body := header + "011000015,100200300,01001,10.00,\"ACME\nSUPPLIES\",2026-10-01,false\n"
	r, err := NewReader(bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := r.Read()
	if err != nil || rec.FieldErr != nil {
		t.Fatalf("%+v %v", rec, err)
	}
	if rec.Payee != "ACME\nSUPPLIES" {
		t.Fatalf("payee = %q", rec.Payee)
	}
}
