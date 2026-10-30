package checkregister

import (
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/dmmoody/poz-pay/internal/api/response"
	upload "github.com/dmmoody/poz-pay/internal/registerupload"
	"github.com/dmmoody/poz-pay/internal/store"
)

func (h Handler) uploadCheckRegisterCSV(w http.ResponseWriter, r *http.Request) {
	if h.Uploads == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "csv upload is not configured")
		return
	}
	fiID := r.PathValue("fi_id")
	partyID := r.PathValue("party_id")
	if !response.LooksLikeUUID(fiID) || !response.LooksLikeUUID(partyID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and party_id must be UUIDs")
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
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "multipart form is invalid")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "file field is required")
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
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "upload failed")
		return
	}
	response.WriteJSON(w, http.StatusAccepted, uploadAcceptedJSON(accepted))
}

func (h Handler) GetCheckRegisterUpload(w http.ResponseWriter, r *http.Request) {
	if h.Uploads == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "csv upload is not configured")
		return
	}
	fiID := r.PathValue("fi_id")
	partyID := r.PathValue("party_id")
	uploadID := r.PathValue("upload_id")
	if !response.LooksLikeUUID(fiID) || !response.LooksLikeUUID(partyID) || !response.LooksLikeUUID(uploadID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id, party_id, and upload_id must be UUIDs")
		return
	}
	found, err := h.Uploads.Get(r.Context(), fiID, partyID, uploadID)
	if writeUploadErr(w, err) {
		return
	}
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "upload lookup failed")
		return
	}
	response.WriteJSON(w, http.StatusOK, uploadStatusJSON(found))
}

func writeUploadErr(w http.ResponseWriter, err error) bool {
	var maxErr *http.MaxBytesError
	switch {
	case err == nil:
		return false
	case errors.As(err, &maxErr), errors.Is(err, upload.ErrTooLarge):
		response.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "file exceeds the upload limit")
	case errors.Is(err, upload.ErrInvalidCSV):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, store.ErrInstitutionNotFound):
		response.WriteError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, store.ErrPartyNotFound):
		response.WriteError(w, http.StatusNotFound, "PARTY_NOT_FOUND", err.Error())
	case errors.Is(err, upload.ErrFIInactive):
		response.WriteError(w, http.StatusConflict, "FI_INACTIVE", err.Error())
	case errors.Is(err, upload.ErrRoutingNumberRequired):
		response.WriteError(w, http.StatusUnprocessableEntity, "ROUTING_NUMBER_REQUIRED", err.Error())
	case errors.Is(err, upload.ErrUploadNotFound):
		response.WriteError(w, http.StatusNotFound, "UPLOAD_NOT_FOUND", err.Error())
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
