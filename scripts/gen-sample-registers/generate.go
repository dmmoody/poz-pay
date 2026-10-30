package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

// Spec is one committed sample file.
type Spec struct {
	Filename      string
	RoutingNumber string
	AccountNumber string
	CheckStart    int
	Rows          int
	Seed          int64
}

// Specs are the files in samples/check-registers. They use the demo
// institutions and account numbers from scripts/seed.sql.
func Specs() []Spec {
	return []Spec{
		{
			Filename: "acme-operating.csv", RoutingNumber: "011000015", AccountNumber: "100200300",
			CheckStart: 10000, Rows: 300, Seed: 1,
		},
		{
			Filename: "acme-treasury.csv", RoutingNumber: "011000015", AccountNumber: "100200301",
			CheckStart: 20000, Rows: 260, Seed: 2,
		},
		{
			Filename: "beta-operating.csv", RoutingNumber: "211274450", AccountNumber: "900100",
			CheckStart: 30000, Rows: 340, Seed: 3,
		},
	}
}

var vendors = []string{
	"ACME SUPPLIES LLC",
	"NORTHWIND TRADING",
	"CONTOSO OFFICE",
	"Smith, Jones & Co",
	"WIDE WORLD IMPORTERS",
	"FABRIKAM INDUSTRIES",
	"ADVENTURE WORKS",
	"LITWARE PAYROLL",
	"PROSEWARE PARTS",
	"ALPINE SKI HOUSE",
	"BLUE YONDER FREIGHT",
	"COHO VINEYARD",
}

// Render writes a canonical CSV. The same spec always produces the same bytes.
func Render(spec Spec) ([]byte, error) {
	if spec.Rows <= 0 {
		return nil, fmt.Errorf("sample %s has no rows", spec.Filename)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{
		"routing_number", "account_number", "check_number", "amount", "payee", "issue_date", "void",
	}); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewSource(spec.Seed))
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < spec.Rows; i++ {
		minor := int64(rng.Intn(250000) + 50)
		void := "false"
		if i > 0 && i%47 == 0 {
			void = "true"
		}
		record := []string{
			spec.RoutingNumber,
			spec.AccountNumber,
			strconv.Itoa(spec.CheckStart + i),
			fmt.Sprintf("%d.%02d", minor/100, minor%100),
			vendors[i%len(vendors)],
			base.AddDate(0, 0, i%40).Format("2006-01-02"),
			void,
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
