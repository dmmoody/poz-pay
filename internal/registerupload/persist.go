package registerupload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkpay"
)

const fileChunkBytes = 256 << 10

func (s *Service) save(ctx context.Context, row Intake, payload []byte, body io.Reader) (Upload, error) {
	var (
		received time.Time
		size     int64
		sum      []byte
	)
	err := s.records.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		received, err = tx.InsertIntake(ctx, row)
		if err != nil {
			return err
		}
		size, sum, err = writeChunks(ctx, tx, row.ID, body)
		if err != nil {
			return err
		}
		if err = tx.SetBytes(ctx, row.ID, size, sum); err != nil {
			return err
		}
		return tx.InsertMessage(ctx, TopicUploadAccepted, payload)
	})
	if err != nil {
		return Upload{}, err
	}
	return Upload{
		ID:            row.ID,
		FIID:          row.FIID,
		PartyID:       row.PartyID,
		RoutingNumber: row.RoutingNumber,
		Filename:      row.Filename,
		ByteSize:      size,
		SHA256:        hex.EncodeToString(sum),
		Status:        "accepted",
		ReceivedAt:    received.UTC(),
	}, nil
}

// writeChunks splits an accepted file into byte slices and hashes them.
// The database inserts those slices. It does not read the upload stream.
func writeChunks(ctx context.Context, tx Tx, id string, r io.Reader) (int64, []byte, error) {
	hash := sha256.New()
	buf := make([]byte, 0, fileChunkBytes)
	tmp := make([]byte, 32<<10)
	seq := 0
	var written int64
	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		chunk := make([]byte, len(buf))
		copy(chunk, buf)
		if err := tx.InsertChunk(ctx, id, seq, chunk); err != nil {
			return err
		}
		seq++
		buf = buf[:0]
		return nil
	}
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			written += int64(n)
			_, _ = hash.Write(tmp[:n])
			buf = append(buf, tmp[:n]...)
			if len(buf) >= fileChunkBytes {
				if err := flush(); err != nil {
					return 0, nil, err
				}
			}
		}
		if err == io.EOF {
			if err := flush(); err != nil {
				return 0, nil, err
			}
			if written == 0 {
				return 0, nil, fmt.Errorf("empty file")
			}
			return written, hash.Sum(nil), nil
		}
		if err != nil {
			return 0, nil, err
		}
	}
}

// chunkReader turns stored chunks into a stream for the CSV parser.
// It keeps ctx for this one parse because encoding/csv.Reader has no context parameter.
type chunkReader struct {
	ctx  context.Context
	conn Conn
	id   string
	seq  int
	buf  []byte
	off  int
	eof  bool
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.off >= len(r.buf) {
		if r.eof {
			return 0, io.EOF
		}
		data, ok, err := r.conn.ReadChunk(r.ctx, r.id, r.seq)
		if err != nil {
			return 0, err
		}
		if !ok {
			r.eof = true
			return 0, io.EOF
		}
		r.seq++
		r.buf = data
		r.off = 0
		if len(data) == 0 {
			return r.Read(p)
		}
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

// rowWriter applies one parsed batch through CRUD statements.
type rowWriter struct {
	tx  Tx
	id  string
	row IntakeRow
}

func (w rowWriter) RejectionCount(ctx context.Context) (int, error) {
	return w.tx.RejectionCount(ctx, w.id)
}

func (w rowWriter) InsertItem(ctx context.Context, item checkpay.UploadItem, payeeNormalized string) error {
	return w.tx.InsertItem(ctx, w.row.BatchID, w.row.FIID, w.row.PartyID, item, payeeNormalized)
}

func (w rowWriter) InsertRejection(ctx context.Context, rej Rejection) error {
	return w.tx.InsertRejection(ctx, w.id, rej)
}

func (w rowWriter) Advance(ctx context.Context, accepted, rejected, rowsProcessed int) error {
	return w.tx.Advance(ctx, w.id, accepted, rejected, rowsProcessed)
}
