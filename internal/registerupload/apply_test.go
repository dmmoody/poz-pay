package registerupload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/dmmoody/poz-pay/internal/store"
)

func TestRejectionForState(t *testing.T) {
	t.Parallel()
	dup, ok := rejectionForState(4, "23505")
	if !ok || dup.Code != CodeDuplicateCheck || dup.Row != 4 {
		t.Fatalf("duplicate = %+v ok=%v", dup, ok)
	}
	unknown, ok := rejectionForState(2, "23503")
	if !ok || unknown.Code != CodeUnknownAccount {
		t.Fatalf("unknown = %+v", unknown)
	}
	if _, ok := rejectionForState(1, "23502"); ok {
		t.Fatal("not-null state should not be a row rejection")
	}
}

func TestApplyBatch_constraintBecomesRejectionAndCapHolds(t *testing.T) {
	t.Parallel()
	w := &fakeBatch{stored: 1}
	rows := []CommittedRow{
		{Row: 1, Item: &store.UploadItem{CheckNumber: "1"}, NormalizedPayee: "A"},
		{Row: 2, Item: &store.UploadItem{CheckNumber: "2"}, NormalizedPayee: "B"},
		{Row: 3, Rejection: &Rejection{Row: 3, Code: CodeUnknownAccount, Message: "missing"}},
	}
	w.failCheck = "2"
	if err := ApplyBatch(context.Background(), w, 3, rows, 1); err != nil {
		t.Fatal(err)
	}
	if w.advanced.accepted != 1 || w.advanced.rejected != 2 || w.advanced.through != 3 {
		t.Fatalf("advance = %+v", w.advanced)
	}
	if len(w.rejections) != 0 {
		t.Fatalf("cap stored %+v", w.rejections)
	}
}

func TestApplyBatch_storesRecognizedConstraint(t *testing.T) {
	t.Parallel()
	w := &fakeBatch{}
	w.failCheck = "9"
	rows := []CommittedRow{{Row: 9, Item: &store.UploadItem{CheckNumber: "9"}, NormalizedPayee: "A"}}
	if err := ApplyBatch(context.Background(), w, 9, rows, 10); err != nil {
		t.Fatal(err)
	}
	if len(w.rejections) != 1 || w.rejections[0].Code != CodeDuplicateCheck {
		t.Fatalf("rejections = %+v", w.rejections)
	}
}

func TestLimitBody(t *testing.T) {
	t.Parallel()
	var exact bytes.Buffer
	if _, err := io.Copy(&exact, limitBody(bytes.NewReader([]byte("abcd")), 4)); err != nil {
		t.Fatal(err)
	}
	if exact.String() != "abcd" {
		t.Fatalf("exact = %q", exact.String())
	}
	var over bytes.Buffer
	_, err := io.Copy(&over, limitBody(bytes.NewReader([]byte("abcde")), 4))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("overflow = %v", err)
	}
	if over.String() != "abcd" {
		t.Fatalf("kept = %q", over.String())
	}
}

func TestInterpretStatus(t *testing.T) {
	t.Parallel()
	if err := InterpretStatus("accepted"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(InterpretStatus("completed"), ErrAlreadyComplete) {
		t.Fatal(InterpretStatus("completed"))
	}
	if err := InterpretStatus("nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatal(err)
	}
}

type advanced struct {
	accepted int
	rejected int
	through  int
}

type fakeBatch struct {
	stored     int
	failCheck  string
	rejections []Rejection
	advanced   advanced
}

func (f *fakeBatch) RejectionCount(context.Context) (int, error) { return f.stored, nil }

func (f *fakeBatch) InsertItem(_ context.Context, item store.UploadItem, _ string) error {
	if item.CheckNumber == f.failCheck {
		return &ConstraintError{State: "23505"}
	}
	return nil
}

func (f *fakeBatch) InsertRejection(_ context.Context, rej Rejection) error {
	f.rejections = append(f.rejections, rej)
	return nil
}

func (f *fakeBatch) Advance(_ context.Context, accepted, rejected, rowsProcessed int) error {
	f.advanced = advanced{accepted, rejected, rowsProcessed}
	return nil
}
