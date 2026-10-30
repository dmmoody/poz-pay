package checkposting

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/api/response"
	postingsvc "github.com/dmmoody/poz-pay/internal/checkposting"
)

type postRequest struct {
	CheckNumber   string `json:"check_number"`
	PaidAt        string `json:"paid_at"`
	CorrelationID string `json:"correlation_id"`
}

func (h Handler) PostCheck(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	if !response.LooksLikeUUID(fiID) || !response.LooksLikeUUID(accountID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}

	var req postRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.CheckNumber) == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}
	paidAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.PaidAt))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "paid_at is required and must be RFC3339")
		return
	}

	payment, err := h.Postings.Post(r.Context(), postingsvc.PostInput{
		FIID:          fiID,
		AccountID:     accountID,
		CheckNumber:   req.CheckNumber,
		CorrelationID: req.CorrelationID,
		PaidAt:        paidAt,
		RawRequest:    req,
	})
	if writePostErr(w, err) {
		return
	}
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "post failed")
		return
	}
	status := http.StatusCreated
	if payment.AlreadyPaid {
		status = http.StatusOK
	}
	response.WriteJSON(w, status, payment)
}

func writePostErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, postingsvc.ErrFINotFound):
		response.WriteError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, postingsvc.ErrAccountNotFound):
		response.WriteError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", err.Error())
	case errors.Is(err, postingsvc.ErrCheckNotInRegister):
		response.WriteError(w, http.StatusNotFound, "CHECK_NOT_IN_REGISTER", err.Error())
	case errors.Is(err, postingsvc.ErrCheckVoided):
		response.WriteError(w, http.StatusConflict, "CHECK_VOIDED", err.Error())
	case errors.Is(err, postingsvc.ErrCheckNumberRequired), errors.Is(err, postingsvc.ErrPaidAtRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		return false
	}
	return true
}
