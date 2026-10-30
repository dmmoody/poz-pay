package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmmoody/poz-pay/internal/checkcsv"
)

func TestSamplesOnDiskMatchGenerator(t *testing.T) {
	t.Parallel()
	for _, spec := range Specs() {
		body, err := Render(spec)
		if err != nil {
			t.Fatal(err)
		}
		disk, err := os.ReadFile(filepath.Join("..", "..", "samples", "check-registers", spec.Filename))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(disk, body) {
			t.Fatalf("%s does not match the generator; regenerate with go run ./scripts/gen-sample-registers", spec.Filename)
		}
		r, err := checkcsv.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		var sawVoid bool
		var sawComma bool
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil || rec.FieldErr != nil {
				t.Fatalf("%s row %d: %v %v", spec.Filename, n, err, rec.FieldErr)
			}
			if rec.RoutingNumber != spec.RoutingNumber || rec.AccountNumber != spec.AccountNumber {
				t.Fatalf("%s row %d identity = %s %s", spec.Filename, rec.Row, rec.RoutingNumber, rec.AccountNumber)
			}
			if rec.Void {
				sawVoid = true
			}
			if rec.Payee == "Smith, Jones & Co" {
				sawComma = true
			}
			n++
		}
		if n != spec.Rows {
			t.Fatalf("%s rows = %d, want %d", spec.Filename, n, spec.Rows)
		}
		if !sawVoid || !sawComma {
			t.Fatalf("%s void=%v comma payee=%v", spec.Filename, sawVoid, sawComma)
		}
	}
}
