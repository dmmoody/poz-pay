package checkpay

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	paysvc "github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/decision"
)

type decideRequest struct {
	Rail          string          `json:"rail"`
	FIID          string          `json:"fi_id"`
	AccountID     string          `json:"account_id"`
	CorrelationID string          `json:"correlation_id"`
	Check         *checkPayload   `json:"check"`
	ACH           json.RawMessage `json:"ach"`
}

type checkPayload struct {
	CheckNumber string `json:"check_number"`
	Amount      int64  `json:"amount"`
	Payee       string `json:"payee"`
	IssueDate   string `json:"issue_date"`
}

func (h Handler) Decide(w http.ResponseWriter, r *http.Request) {
	var req decideRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	req.Rail = strings.TrimSpace(strings.ToLower(req.Rail))
	req.FIID = strings.TrimSpace(req.FIID)
	req.AccountID = strings.TrimSpace(req.AccountID)

	if req.FIID == "" || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id are required")
		return
	}
	if !looksLikeUUID(req.FIID) || !looksLikeUUID(req.AccountID) {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}

	switch req.Rail {
	case decision.RailACH:
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACH decide not implemented yet")
		return
	case decision.RailCheck:
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "rail must be check or ach")
		return
	}

	if req.Check == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check payload required when rail=check")
		return
	}
	if strings.TrimSpace(req.Check.CheckNumber) == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.check_number is required")
		return
	}
	if req.Check.Amount < 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.amount must be >= 0")
		return
	}

	presented := paysvc.PresentedCheck{
		CheckNumber: strings.TrimSpace(req.Check.CheckNumber),
		Amount:      req.Check.Amount,
		Payee:       req.Check.Payee,
	}
	if d := strings.TrimSpace(req.Check.IssueDate); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.issue_date must be YYYY-MM-DD")
			return
		}
		presented.IssueDate = &t
	}

	result, err := h.Checks.Decide(r.Context(), paysvc.DecideInput{
		FIID:          req.FIID,
		AccountID:     req.AccountID,
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		Check:         presented,
		RawRequest:    req,
	})
	if err != nil {
		if errors.Is(err, paysvc.ErrFINotFound) {
			writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "decide failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
