package registerupload

import (
	"errors"
	"mime"
	"net/http"
	"time"

	paysvc "github.com/dmmoody/poz-pay/internal/checkpay"
	upload "github.com/dmmoody/poz-pay/internal/registerupload"
)

func (h Handler) uploadCheckRegisterCSV(w http.ResponseWriter, r *http.Request) {
	if h.Uploads == nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "csv upload is not configured")
		return
	}
	fiID := r.PathValue("fi_id")
	partyID := r.PathValue("party_id")
	if !looksLikeUUID(fiID) || !looksLikeUUID(partyID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and party_id must be UUIDs")
		return
	}
	max := h.MaxUploadBytes
	if max <= 0 {
		max = upload.DefaultMaxUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if writeUploadErr(w, err) {
			return
		}
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "multipart form is invalid")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "file field is required")
		return
	}
	defer file.Close()

	accepted, err := h.Uploads.Accept(r.Context(), upload.AcceptInput{
		FIID:     fiID,
		PartyID:  partyID,
		Filename: hdr.Filename,
		Body:     file,
	})
	if writeUploadErr(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "upload failed")
		return
	}
	writeJSON(w, http.StatusAccepted, uploadAcceptedJSON(accepted))
}

func (h Handler) GetCheckRegisterUpload(w http.ResponseWriter, r *http.Request) {
	if h.Uploads == nil {
		writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "csv upload is not configured")
		return
	}
	fiID := r.PathValue("fi_id")
	partyID := r.PathValue("party_id")
	uploadID := r.PathValue("upload_id")
	if !looksLikeUUID(fiID) || !looksLikeUUID(partyID) || !looksLikeUUID(uploadID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id, party_id, and upload_id must be UUIDs")
		return
	}
	found, err := h.Uploads.Get(r.Context(), fiID, partyID, uploadID)
	if writeUploadErr(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "upload lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, uploadStatusJSON(found))
}

func writeUploadErr(w http.ResponseWriter, err error) bool {
	var maxErr *http.MaxBytesError
	switch {
	case err == nil:
		return false
	case errors.As(err, &maxErr), errors.Is(err, upload.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "file exceeds the upload limit")
	case errors.Is(err, upload.ErrInvalidCSV):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, paysvc.ErrFINotFound):
		writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, paysvc.ErrPartyNotFound):
		writeError(w, http.StatusNotFound, "PARTY_NOT_FOUND", err.Error())
	case errors.Is(err, upload.ErrFIInactive):
		writeError(w, http.StatusConflict, "FI_INACTIVE", err.Error())
	case errors.Is(err, upload.ErrRoutingNumberRequired):
		writeError(w, http.StatusUnprocessableEntity, "ROUTING_NUMBER_REQUIRED", err.Error())
	case errors.Is(err, upload.ErrUploadNotFound):
		writeError(w, http.StatusNotFound, "UPLOAD_NOT_FOUND", err.Error())
	default:
		return false
	}
	return true
}

type uploadAcceptedBody struct {
	UploadID      string    `json:"upload_id"`
	FIID          string    `json:"fi_id"`
	PartyID       string    `json:"party_id"`
	RoutingNumber string    `json:"routing_number"`
	Filename      string    `json:"filename"`
	ByteSize      int64     `json:"byte_size"`
	SHA256        string    `json:"sha256"`
	Status        string    `json:"status"`
	ReceivedAt    time.Time `json:"received_at"`
}

type uploadStatusBody struct {
	uploadAcceptedBody
	BatchID             string            `json:"batch_id,omitempty"`
	Accepted            int               `json:"accepted"`
	Rejected            int               `json:"rejected"`
	RowsProcessed       int               `json:"rows_processed"`
	Failure             string            `json:"failure,omitempty"`
	RejectionsTruncated bool              `json:"rejections_truncated"`
	Rejections          []uploadRejection `json:"rejections"`
}

type uploadRejection struct {
	Row     int    `json:"row"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func uploadAcceptedJSON(u upload.Upload) uploadAcceptedBody {
	return uploadAcceptedBody{
		UploadID:      u.ID,
		FIID:          u.FIID,
		PartyID:       u.PartyID,
		RoutingNumber: u.RoutingNumber,
		Filename:      u.Filename,
		ByteSize:      u.ByteSize,
		SHA256:        u.SHA256,
		Status:        u.Status,
		ReceivedAt:    u.ReceivedAt.UTC(),
	}
}

func uploadStatusJSON(u upload.Upload) uploadStatusBody {
	rejections := make([]uploadRejection, 0, len(u.Rejections))
	for _, rej := range u.Rejections {
		rejections = append(rejections, uploadRejection{Row: rej.Row, Code: rej.Code, Message: rej.Message})
	}
	return uploadStatusBody{
		uploadAcceptedBody:  uploadAcceptedJSON(u),
		BatchID:             u.BatchID,
		Accepted:            u.Accepted,
		Rejected:            u.Rejected,
		RowsProcessed:       u.RowsProcessed,
		Failure:             u.Failure,
		RejectionsTruncated: u.RejectionsTruncated(),
		Rejections:          rejections,
	}
}

func isMultipart(r *http.Request) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "multipart/form-data"
}
