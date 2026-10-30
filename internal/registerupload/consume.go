package registerupload

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/dmmoody/poz-pay/internal/broker"
)

// Consume blocks until ctx is cancelled, ingesting each accepted upload.
func Consume(ctx context.Context, lis broker.Listener, uploads *Service) error {
	return lis.Listen(ctx, TopicUploadAccepted, func(ctx context.Context, body []byte) error {
		var msg struct {
			UploadID string `json:"upload_id"`
		}
		if err := json.Unmarshal(body, &msg); err != nil || !validUploadID(msg.UploadID) {
			log.Printf("discard check register message: %v", err)
			return nil
		}
		err := uploads.Ingest(ctx, msg.UploadID)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, ErrAlreadyComplete), errors.Is(err, ErrAlreadyFailed), errors.Is(err, ErrUploadNotFound):
			return nil
		default:
			log.Printf("ingest %s: %v", msg.UploadID, err)
			return err
		}
	})
}

func validUploadID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}
