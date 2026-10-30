package checkdecision

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dmmoody/poz-pay/internal/api/response"
	decisionsvc "github.com/dmmoody/poz-pay/internal/checkdecision"
)

func (h Handler) ListCheckDecisions(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	checkNumber := strings.TrimSpace(r.PathValue("check_number"))
	if !response.LooksLikeUUID(fiID) || !response.LooksLikeUUID(accountID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}
	if checkNumber == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}

	history, err := h.Decisions.ListDecisions(r.Context(), fiID, accountID, checkNumber)
	if writeDecisionErr(w, err) {
		return
	}
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "history failed")
		return
	}
	response.WriteJSON(w, http.StatusOK, history)
}

func (h Handler) ListCheckAudit(w http.ResponseWriter, r *http.Request) {
	fiID := r.PathValue("fi_id")
	accountID := r.PathValue("account_id")
	checkNumber := strings.TrimSpace(r.PathValue("check_number"))
	if !response.LooksLikeUUID(fiID) || !response.LooksLikeUUID(accountID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}
	if checkNumber == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check_number is required")
		return
	}

	audit, err := h.Decisions.ListAudit(r.Context(), fiID, accountID, checkNumber)
	if writeDecisionErr(w, err) {
		return
	}
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "audit failed")
		return
	}
	response.WriteJSON(w, http.StatusOK, audit)
}

func writeDecisionErr(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, decisionsvc.ErrFINotFound):
		response.WriteError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, decisionsvc.ErrAccountNotFound):
		response.WriteError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", err.Error())
	case errors.Is(err, decisionsvc.ErrCheckNumberRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		return false
	}
	return true
}
