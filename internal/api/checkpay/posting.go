package checkpay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	paysvc "github.com/dmmoody/poz-pay/internal/checkpay"
)

type postRequest struct {
	CheckNumber   string `json:"check_number"`
	PaidAt        string `json:"paid_at"`
	CorrelationID string `json:"correlation_id"`
}

func (h Handler) PostCheck(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	if !looksLikeUUID(fiID) || !looksLikeUUID(accountID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}

	var req postRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.CheckNumber) == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}
	paidAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.PaidAt))
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "paid_at is required and must be RFC3339")
		return
	}

	payment, err := h.Checks.Post(r.Context(), paysvc.PostInput{
		FIID:          fiID,
		AccountID:     accountID,
		CheckNumber:   req.CheckNumber,
		CorrelationID: req.CorrelationID,
		PaidAt:        paidAt,
		RawRequest:    req,
	})
	if writeCheckErr(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "post failed")
		return
	}
	status := http.StatusCreated
	if payment.AlreadyPaid {
		status = http.StatusOK
	}
	writeJSON(w, status, payment)
}

func (h Handler) ListCheckDecisions(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	checkNumber := strings.TrimSpace(r.PathValue("check_number"))
	if !looksLikeUUID(fiID) || !looksLikeUUID(accountID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}
	if checkNumber == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}

	history, err := h.Checks.ListDecisions(r.Context(), fiID, accountID, checkNumber)
	if writeCheckErr(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "history failed")
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (h Handler) ListCheckAudit(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	checkNumber := strings.TrimSpace(r.PathValue("check_number"))
	if !looksLikeUUID(fiID) || !looksLikeUUID(accountID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}
	if checkNumber == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}

	audit, err := h.Checks.ListAudit(r.Context(), fiID, accountID, checkNumber)
	if writeCheckErr(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "audit failed")
		return
	}
	writeJSON(w, http.StatusOK, audit)
}

func writeCheckErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, paysvc.ErrFINotFound):
		writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, paysvc.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", err.Error())
	case errors.Is(err, paysvc.ErrCheckNotInRegister):
		writeError(w, http.StatusNotFound, "CHECK_NOT_IN_REGISTER", err.Error())
	case errors.Is(err, paysvc.ErrCheckVoided):
		writeError(w, http.StatusConflict, "CHECK_VOIDED", err.Error())
	case errors.Is(err, paysvc.ErrCheckNumberRequired), errors.Is(err, paysvc.ErrPaidAtRequired):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		return false
	}
	return true
}
