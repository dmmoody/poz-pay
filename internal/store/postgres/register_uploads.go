package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dmmoody/poz-pay/internal/store"
)

// RegisterUploads is CRUD for check-register intake rows, file chunks,
// rejections, and the checks parsed from a file.
// Receiving a file, hashing it, and parsing it happen before these methods run.
type RegisterUploads struct {
	db *sql.DB
}

func NewRegisterUploads(db *sql.DB) *RegisterUploads {
	return &RegisterUploads{db: db}
}

func (s *RegisterUploads) WithinTx(ctx context.Context, fn func(context.Context, store.UploadTx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, recordTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *RegisterUploads) WithinConn(ctx context.Context, fn func(context.Context, store.UploadConn) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connection: %w", err)
	}
	defer conn.Close()
	return fn(ctx, recordConn{conn: conn})
}

func (s *RegisterUploads) Find(ctx context.Context, fiID, partyID, id string) (store.Upload, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id::text, fi_id::text, party_id::text, routing_number, filename,
		       byte_size, sha256, status, batch_id::text, accepted_count, rejected_count,
		       rows_processed, failure, received_at
		FROM check_register_uploads
		WHERE id = $1::uuid AND fi_id = $2::uuid AND party_id = $3::uuid
	`, id, fiID, partyID)
	up, err := scanIntake(row)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Upload{}, store.ErrNotFound
	}
	if err != nil {
		return store.Upload{}, err
	}
	rejections, err := listRejections(ctx, s.db, id)
	if err != nil {
		return store.Upload{}, err
	}
	up.Rejections = rejections
	return up, nil
}

type recordTx struct{ tx *sql.Tx }

func (t recordTx) InsertIntake(ctx context.Context, row store.Intake) (time.Time, error) {
	var received time.Time
	err := t.tx.QueryRowContext(ctx, `
		INSERT INTO check_register_uploads (
			id, fi_id, party_id, routing_number, filename, byte_size, sha256, status
		) VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4, $5, 0, $6, 'accepted'
		)
		RETURNING received_at
	`, row.ID, row.FIID, row.PartyID, row.RoutingNumber, row.Filename, make([]byte, 32)).Scan(&received)
	if err != nil {
		return time.Time{}, fmt.Errorf("insert intake: %w", err)
	}
	return received, nil
}

func (t recordTx) InsertChunk(ctx context.Context, id string, seq int, data []byte) error {
	if _, err := t.tx.ExecContext(ctx, `
		INSERT INTO check_register_upload_blobs (upload_id, seq, data)
		VALUES ($1::uuid, $2, $3)
	`, id, seq, data); err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	return nil
}

func (t recordTx) SetBytes(ctx context.Context, id string, size int64, sum []byte) error {
	if _, err := t.tx.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET byte_size = $2, sha256 = $3, updated_at = now()
		WHERE id = $1::uuid
	`, id, size, sum); err != nil {
		return fmt.Errorf("set file bytes: %w", err)
	}
	return nil
}

func (t recordTx) InsertMessage(ctx context.Context, topic string, payload []byte) error {
	if _, err := t.tx.ExecContext(ctx, `
		INSERT INTO message_outbox (topic, payload) VALUES ($1, $2::jsonb)
	`, topic, string(payload)); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

func (t recordTx) LockIntake(ctx context.Context, id string) (store.IntakeRow, error) {
	var (
		row     store.IntakeRow
		batchID sql.NullString
	)
	err := t.tx.QueryRowContext(ctx, `
		SELECT fi_id::text, party_id::text, routing_number, status, rows_processed, batch_id::text
		FROM check_register_uploads
		WHERE id = $1::uuid
		FOR UPDATE
	`, id).Scan(&row.FIID, &row.PartyID, &row.RoutingNumber, &row.Status, &row.RowsProcessed, &batchID)
	if errors.Is(err, sql.ErrNoRows) {
		return store.IntakeRow{}, store.ErrNotFound
	}
	if err != nil {
		return store.IntakeRow{}, fmt.Errorf("lock intake: %w", err)
	}
	if batchID.Valid {
		row.BatchID = batchID.String
	}
	return row, nil
}

func (t recordTx) InsertBatch(ctx context.Context, fiID, partyID, source string) (string, error) {
	var id string
	err := t.tx.QueryRowContext(ctx, `
		INSERT INTO check_register_batches (fi_id, party_id, item_count, source)
		VALUES ($1::uuid, $2::uuid, 0, $3)
		RETURNING id::text
	`, fiID, partyID, source).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert batch: %w", err)
	}
	return id, nil
}

func (t recordTx) SetIngesting(ctx context.Context, id, batchID string) error {
	res, err := t.tx.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET status = 'ingesting', batch_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
	`, id, batchID)
	if err != nil {
		return fmt.Errorf("set ingesting: %w", err)
	}
	return oneRow(res, "intake was not updated")
}

func (t recordTx) RejectionCount(ctx context.Context, id string) (int, error) {
	var n int
	err := t.tx.QueryRowContext(ctx, `
		SELECT count(*) FROM check_register_upload_rejections WHERE upload_id = $1::uuid
	`, id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count rejections: %w", err)
	}
	return n, nil
}

func (t recordTx) InsertItem(ctx context.Context, batchID, fiID, partyID string, item store.UploadItem, payeeNormalized string) error {
	if _, err := t.tx.ExecContext(ctx, `SAVEPOINT row_sp`); err != nil {
		return fmt.Errorf("savepoint: %w", err)
	}
	err := insertItem(ctx, t.tx, batchID, fiID, partyID, item, payeeNormalized)
	if err == nil {
		if _, relErr := t.tx.ExecContext(ctx, `RELEASE SAVEPOINT row_sp`); relErr != nil {
			return fmt.Errorf("release savepoint: %w", relErr)
		}
		return nil
	}
	if _, rbErr := t.tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT row_sp`); rbErr != nil {
		return fmt.Errorf("rollback row: %w", rbErr)
	}
	if _, relErr := t.tx.ExecContext(ctx, `RELEASE SAVEPOINT row_sp`); relErr != nil {
		return fmt.Errorf("release savepoint: %w", relErr)
	}
	if state, ok := constraintState(err); ok {
		return &store.ConstraintError{State: state}
	}
	return fmt.Errorf("insert check: %w", err)
}

func (t recordTx) InsertRejection(ctx context.Context, id string, rej store.Rejection) error {
	if _, err := t.tx.ExecContext(ctx, `
		INSERT INTO check_register_upload_rejections (upload_id, row_number, code, message)
		VALUES ($1::uuid, $2, $3, $4)
	`, id, rej.Row, rej.Code, rej.Message); err != nil {
		return fmt.Errorf("insert rejection: %w", err)
	}
	return nil
}

func (t recordTx) Advance(ctx context.Context, id string, accepted, rejected, rowsProcessed int) error {
	res, err := t.tx.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET accepted_count = accepted_count + $2,
		    rejected_count = rejected_count + $3,
		    rows_processed = $4,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND status = 'ingesting'
		  AND rows_processed < $4
	`, id, accepted, rejected, rowsProcessed)
	if err != nil {
		return fmt.Errorf("advance intake: %w", err)
	}
	if err := oneRow(res, fmt.Sprintf("intake %s did not advance to row %d", id, rowsProcessed)); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, `
		UPDATE check_register_batches b
		SET item_count = u.accepted_count
		FROM check_register_uploads u
		WHERE u.id = $1::uuid AND b.id = u.batch_id
	`, id); err != nil {
		return fmt.Errorf("update batch count: %w", err)
	}
	return nil
}

type recordConn struct{ conn *sql.Conn }

func (c recordConn) TryLock(ctx context.Context, key string) (bool, error) {
	var locked bool
	err := c.conn.QueryRowContext(ctx, `
		SELECT pg_try_advisory_lock(hashtextextended($1::text, 0::bigint))
	`, key).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("advisory lock: %w", err)
	}
	return locked, nil
}

func (c recordConn) Unlock(ctx context.Context, key string) error {
	if _, err := c.conn.ExecContext(ctx, `
		SELECT pg_advisory_unlock(hashtextextended($1::text, 0::bigint))
	`, key); err != nil {
		return fmt.Errorf("advisory unlock: %w", err)
	}
	return nil
}

func (c recordConn) WithinTx(ctx context.Context, fn func(context.Context, store.UploadTx) error) error {
	tx, err := c.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, recordTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (c recordConn) ReadChunk(ctx context.Context, id string, seq int) ([]byte, bool, error) {
	var data []byte
	err := c.conn.QueryRowContext(ctx, `
		SELECT data FROM check_register_upload_blobs
		WHERE upload_id = $1::uuid AND seq = $2
	`, id, seq).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read chunk: %w", err)
	}
	return data, true, nil
}

func (c recordConn) MarkCompleted(ctx context.Context, id string) error {
	res, err := c.conn.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET status = 'completed', completed_at = now(), updated_at = now()
		WHERE id = $1::uuid AND status = 'ingesting'
	`, id)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	return oneRow(res, "intake was not ingesting")
}

func (c recordConn) MarkFailed(ctx context.Context, id, reason string) error {
	res, err := c.conn.ExecContext(ctx, `
		UPDATE check_register_uploads
		SET status = 'failed', failure = $2, updated_at = now()
		WHERE id = $1::uuid AND status = 'ingesting'
	`, id, reason)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return oneRow(res, "intake was not ingesting")
}

func oneRow(res sql.Result, what string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%s", what)
	}
	return nil
}

func constraintState(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && len(pgErr.Code) >= 2 && pgErr.Code[:2] == "23" {
		return pgErr.Code, true
	}
	return "", false
}

func scanIntake(row interface{ Scan(dest ...any) error }) (store.Upload, error) {
	var (
		up       store.Upload
		sum      []byte
		batchID  sql.NullString
		failure  sql.NullString
		received time.Time
	)
	err := row.Scan(
		&up.ID, &up.FIID, &up.PartyID, &up.RoutingNumber, &up.Filename,
		&up.ByteSize, &sum, &up.Status, &batchID, &up.Accepted, &up.Rejected,
		&up.RowsProcessed, &failure, &received,
	)
	if err != nil {
		return store.Upload{}, err
	}
	up.SHA256 = hex.EncodeToString(sum)
	up.ReceivedAt = received.UTC()
	if batchID.Valid {
		up.BatchID = batchID.String
	}
	if failure.Valid {
		up.Failure = failure.String
	}
	return up, nil
}

func listRejections(ctx context.Context, q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, id string) ([]store.Rejection, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT row_number, code, message
		FROM check_register_upload_rejections
		WHERE upload_id = $1::uuid
		ORDER BY row_number
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list rejections: %w", err)
	}
	defer rows.Close()
	var out []store.Rejection
	for rows.Next() {
		var rej store.Rejection
		if err := rows.Scan(&rej.Row, &rej.Code, &rej.Message); err != nil {
			return nil, fmt.Errorf("scan rejection: %w", err)
		}
		out = append(out, rej)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rejections: %w", err)
	}
	return out, nil
}
