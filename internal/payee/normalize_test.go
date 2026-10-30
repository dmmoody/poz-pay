package payee

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()
	got := Normalize("  acme   supplies  llc ")
	if got != "ACME SUPPLIES LLC" {
		t.Fatalf("got %q", got)
	}
}
