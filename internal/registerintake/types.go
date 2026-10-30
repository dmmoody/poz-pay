package registerintake

import (
	"context"
	"errors"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkregister"
)

// ErrNotFound means the upload is not under this institution and customer.
var ErrNotFound = errors.New("check register upload not found")

// ConstraintError is a class-23 integrity failure from a register insert.
// State is the SQLSTATE. The caller decides the customer-facing rejection.
type ConstraintError struct {
	State string
}

func (e *ConstraintError) Error() string {
	if e == nil {
		return "register write constraint"
	}
	return "register write constraint " + e.State
}

// Intake is the row inserted once a file has been accepted.
type Intake struct {
	ID            string
	FIID          string
	PartyID       string
	RoutingNumber string
	Filename      string
}

// IntakeRow is one stored file locked for parsing.
type IntakeRow struct {
	FIID          string
	PartyID       string
	RoutingNumber string
	Status        string
	RowsProcessed int
	BatchID       string
}

// Rejection is one data row that did not become a register item.
// Row is 1-based among data records.
type Rejection struct {
	Row     int
	Code    string
	Message string
}

// Upload is the intake record for one file.
type Upload struct {
	ID            string
	FIID          string
	PartyID       string
	RoutingNumber string
	Filename      string
	ByteSize      int64
	SHA256        string
	Status        string
	BatchID       string
	Accepted      int
	Rejected      int
	RowsProcessed int
	Failure       string
	ReceivedAt    time.Time
	Rejections    []Rejection
}

// RejectionsTruncated reports that more rows failed than were stored.
func (u Upload) RejectionsTruncated() bool {
	return u.Rejected > len(u.Rejections)
}

// Records is CRUD for one upload. WithinTx and WithinConn only begin, commit, and roll back.
type Records interface {
	WithinTx(ctx context.Context, fn func(context.Context, Tx) error) error
	WithinConn(ctx context.Context, fn func(context.Context, Conn) error) error
	Find(ctx context.Context, fiID, partyID, id string) (Upload, error)
}

// Tx is one database transaction. Each method is a single statement.
type Tx interface {
	InsertIntake(ctx context.Context, row Intake) (time.Time, error)
	InsertChunk(ctx context.Context, id string, seq int, data []byte) error
	SetBytes(ctx context.Context, id string, size int64, sum []byte) error
	InsertMessage(ctx context.Context, topic string, payload []byte) error
	LockIntake(ctx context.Context, id string) (IntakeRow, error)
	InsertBatch(ctx context.Context, fiID, partyID, source string) (string, error)
	SetIngesting(ctx context.Context, id, batchID string) error
	RejectionCount(ctx context.Context, id string) (int, error)
	InsertItem(ctx context.Context, batchID, fiID, partyID string, item checkregister.UploadItem, payeeNormalized string) error
	InsertRejection(ctx context.Context, id string, rej Rejection) error
	Advance(ctx context.Context, id string, accepted, rejected, rowsProcessed int) error
}

// Conn is one database connection. The advisory lock is held on this connection
// across the transactions started with WithinTx.
type Conn interface {
	TryLock(ctx context.Context, key string) (bool, error)
	Unlock(ctx context.Context, key string) error
	WithinTx(ctx context.Context, fn func(context.Context, Tx) error) error
	ReadChunk(ctx context.Context, id string, seq int) ([]byte, bool, error)
	MarkCompleted(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, reason string) error
}
