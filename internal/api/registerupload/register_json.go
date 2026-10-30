package registerupload

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	paysvc "github.com/dmmoody/poz-pay/internal/checkpay"
)

type uploadRequest struct {
	Format string         `json:"format"`
	Strict bool           `json:"strict"`
	Items  []uploadItemIn `json:"items"`
}

type uploadItemIn struct {
	AccountID   string `json:"account_id"`
	CheckNumber string `json:"check_number"`
	Amount      int64  `json:"amount"`
	Payee       string `json:"payee"`
	IssueDate   string `json:"issue_date"`
	Void        bool   `json:"void"`
}

type uploadResponse struct {
	BatchID  string                   `json:"batch_id"`
	Accepted int                      `json:"accepted"`
	Rejected int                      `json:"rejected"`
	Errors   []paysvc.UploadItemError `json:"errors"`
}

func (h Handler) UploadCheckRegister(w http.ResponseWriter, r *http.Request) {
	if isMultipart(r) {
		h.uploadCheckRegisterCSV(w, r)
		return
	}
	fiID := r.PathValue("fi_id")
	partyID := r.PathValue("party_id")
	if !looksLikeUUID(fiID) || !looksLikeUUID(partyID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and party_id must be UUIDs")
		return
	}

	var req uploadRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if req.Format != "" && !strings.EqualFold(req.Format, "json") {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "format must be json")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "items required")
		return
	}

	items := make([]paysvc.UploadItem, 0, len(req.Items))
	for _, in := range req.Items {
		it := paysvc.UploadItem{
			AccountID:   strings.TrimSpace(in.AccountID),
			CheckNumber: strings.TrimSpace(in.CheckNumber),
			Amount:      in.Amount,
			Payee:       in.Payee,
			Void:        in.Void,
		}
		if d := strings.TrimSpace(in.IssueDate); d != "" {
			t, err := time.Parse("2006-01-02", d)
			if err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "issue_date must be YYYY-MM-DD")
				return
			}
			it.IssueDate = &t
		}
		items = append(items, it)
	}

	result, err := h.Checks.Upload(r.Context(), fiID, partyID, items)
	if err != nil {
		if errors.Is(err, paysvc.ErrFINotFound) {
			writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, paysvc.ErrPartyNotFound) {
			writeError(w, http.StatusNotFound, "PARTY_NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "upload failed")
		return
	}

	status := http.StatusCreated
	if req.Strict && result.Rejected > 0 {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, uploadResponse{
		BatchID:  result.BatchID,
		Accepted: result.Accepted,
		Rejected: result.Rejected,
		Errors:   result.Errors,
	})
}
