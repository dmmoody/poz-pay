package registerbatch

import (
	"context"
	"errors"
	"testing"
)

func TestUpload_rejectsUnknownInstitutionAndParty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("institution", func(t *testing.T) {
		t.Parallel()
		svc := NewService(memAccounts{}, &memRegister{})
		_, err := svc.Upload(ctx, "fi", "party", nil)
		if !errors.Is(err, ErrFINotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("party", func(t *testing.T) {
		t.Parallel()
		svc := NewService(memAccounts{fi: true}, &memRegister{})
		_, err := svc.Upload(ctx, "fi", "party", nil)
		if !errors.Is(err, ErrPartyNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUpload_keepsCallerIndexesAndNormalizesPayee(t *testing.T) {
	t.Parallel()
	reg := &memRegister{}
	svc := NewService(memAccounts{fi: true, party: true, account: "acct-1"}, reg)
	items := []UploadItem{
		{AccountID: "", CheckNumber: "1", Payee: "x"},
		{AccountID: "acct-1", CheckNumber: "2", Amount: 10, Payee: "  acme   supplies "},
		{AccountID: "other", CheckNumber: "3", Payee: "y"},
		{AccountID: "acct-1", CheckNumber: "4", Amount: -1, Payee: "z"},
	}

	result, err := svc.Upload(context.Background(), "fi", "party", items)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if result.BatchID != "batch-1" || result.Accepted != 0 || result.Rejected != 4 {
		t.Fatalf("result = %+v", result)
	}
	if len(reg.prepared) != 1 || reg.prepared[0].CheckNumber != "2" || reg.normalized != "ACME SUPPLIES" {
		t.Fatalf("prepared = %+v normalized %q", reg.prepared, reg.normalized)
	}
	if len(result.Errors) != 4 ||
		result.Errors[0].Index != 0 ||
		result.Errors[1].Index != 2 ||
		result.Errors[2].Index != 3 ||
		result.Errors[3].Index != 1 ||
		result.Errors[3].Message != "duplicate" {
		t.Fatalf("errors = %+v", result.Errors)
	}
}

type memAccounts struct {
	fi      bool
	party   bool
	account string
}

func (m memAccounts) FIExists(context.Context, string) (bool, error) { return m.fi, nil }

func (m memAccounts) PartyBelongsToFI(context.Context, string, string) (bool, error) {
	return m.party, nil
}

func (m memAccounts) AccountOnParty(_ context.Context, _, _, accountID string) (bool, error) {
	return m.account != "" && m.account == accountID, nil
}

type memRegister struct {
	prepared   []UploadItem
	normalized string
}

func (m *memRegister) InsertBatch(_ context.Context, _, _ string, items []UploadItem, payeeNorm func(string) string) (UploadBatchResult, error) {
	m.prepared = items
	if len(items) > 0 {
		m.normalized = payeeNorm(items[0].Payee)
	}
	return UploadBatchResult{
		BatchID:  "batch-1",
		Rejected: 1,
		Errors:   []UploadItemError{{Index: 0, Message: "duplicate"}},
	}, nil
}
