package registerupload

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dmmoody/poz-pay/internal/checkcsv"
	"github.com/dmmoody/poz-pay/internal/checkpay"
)

// MaxStoredRejections is how many row failures are kept for one upload.
// rejected_count still counts every row. The cap is a product rule, applied
// before the store is asked to insert.
const MaxStoredRejections = 100

const maxFailureRunes = 500

// ConstraintError is a class-23 integrity failure from a register insert.
// State is the SQLSTATE. The store reports it; this package decides the
// customer-facing rejection.
type ConstraintError struct {
	State string
}

func (e *ConstraintError) Error() string {
	if e == nil {
		return "register write constraint"
	}
	return "register write constraint " + e.State
}

// BatchWriter is the persistence port for one ingest transaction.
// Implementations run SQL. They do not choose rejection codes or messages.
type BatchWriter interface {
	RejectionCount(ctx context.Context) (int, error)
	InsertItem(ctx context.Context, item checkpay.UploadItem, payeeNormalized string) error
	InsertRejection(ctx context.Context, rej Rejection) error
	Advance(ctx context.Context, accepted, rejected, rowsProcessed int) error
}

// InterpretStatus decides whether a stored upload may be ingested.
func InterpretStatus(status string) error {
	switch status {
	case "completed":
		return ErrAlreadyComplete
	case "failed":
		return ErrAlreadyFailed
	case "accepted", "ingesting":
		return nil
	default:
		return fmt.Errorf("unexpected upload status %q", status)
	}
}

// ApplyBatch writes one ingest batch. A constraint violation becomes a row
// rejection when this package recognizes the SQLSTATE. Anything else aborts
// the batch so the caller can retry.
func ApplyBatch(ctx context.Context, w BatchWriter, rowsProcessed int, rows []CommittedRow, keepRejections int) error {
	if len(rows) == 0 {
		return nil
	}
	if keepRejections < 0 {
		keepRejections = 0
	}
	stored, err := w.RejectionCount(ctx)
	if err != nil {
		return err
	}
	accepted := 0
	rejected := 0
	for _, row := range rows {
		switch {
		case row.Item != nil && row.Rejection == nil:
			err := w.InsertItem(ctx, *row.Item, row.NormalizedPayee)
			if err == nil {
				accepted++
				continue
			}
			var violation *ConstraintError
			if !errors.As(err, &violation) {
				return err
			}
			rej, ok := rejectionForState(row.Row, violation.State)
			if !ok {
				return err
			}
			rejected++
			if err := keepRejection(ctx, w, &stored, keepRejections, rej); err != nil {
				return err
			}
		case row.Rejection != nil && row.Item == nil:
			rejected++
			if err := keepRejection(ctx, w, &stored, keepRejections, *row.Rejection); err != nil {
				return err
			}
		default:
			return fmt.Errorf("ingest row %d is not an item or a rejection", row.Row)
		}
	}
	return w.Advance(ctx, accepted, rejected, rowsProcessed)
}

func keepRejection(ctx context.Context, w BatchWriter, stored *int, keep int, rej Rejection) error {
	if *stored >= keep {
		return nil
	}
	rej.Message = clip(rej.Message, maxFailureRunes)
	if err := w.InsertRejection(ctx, rej); err != nil {
		return err
	}
	*stored++
	return nil
}

func rejectionForState(row int, state string) (Rejection, bool) {
	switch state {
	case "23505":
		return Rejection{Row: row, Code: CodeDuplicateCheck, Message: "active check number is already on the register"}, true
	case "23503":
		return Rejection{Row: row, Code: CodeUnknownAccount, Message: "account number is not on this customer"}, true
	case "23514":
		return Rejection{Row: row, Code: checkcsv.CodeInvalidRow, Message: "check row violates a register constraint"}, true
	default:
		return Rejection{}, false
	}
}

func failureReason(reason string) string {
	reason = clip(reason, maxFailureRunes)
	if reason == "" {
		return "ingest failed"
	}
	return reason
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func acceptedPayload(uploadID string) ([]byte, error) {
	body, err := json.Marshal(struct {
		UploadID string `json:"upload_id"`
	}{UploadID: uploadID})
	if err != nil {
		return nil, fmt.Errorf("encode ingest message: %w", err)
	}
	return body, nil
}

func newUploadID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("upload id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// limitBody stops a read after max bytes. One extra byte is a size violation,
// not something the store should measure.
func limitBody(r io.Reader, max int64) io.Reader {
	return &limitedReader{r: r, allow: max + 1}
}

type limitedReader struct {
	r     io.Reader
	allow int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.allow <= 1 {
		if l.allow <= 0 {
			return 0, ErrTooLarge
		}
		var probe [1]byte
		n, err := l.r.Read(probe[:])
		l.allow -= int64(n)
		if n > 0 {
			l.allow = 0
			return 0, ErrTooLarge
		}
		if errors.Is(err, io.EOF) {
			return 0, io.EOF
		}
		return 0, err
	}
	budget := l.allow - 1
	if int64(len(p)) > budget {
		p = p[:budget]
	}
	n, err := l.r.Read(p)
	l.allow -= int64(n)
	return n, err
}
