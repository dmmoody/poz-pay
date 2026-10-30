package checkdecision

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/api/response"
	decisionsvc "github.com/dmmoody/poz-pay/internal/checkdecision"
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
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	req.Rail = strings.TrimSpace(strings.ToLower(req.Rail))
	req.FIID = strings.TrimSpace(req.FIID)
	req.AccountID = strings.TrimSpace(req.AccountID)

	if req.FIID == "" || req.AccountID == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id are required")
		return
	}
	if !response.LooksLikeUUID(req.FIID) || !response.LooksLikeUUID(req.AccountID) {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "fi_id and account_id must be UUIDs")
		return
	}

	switch req.Rail {
	case decision.RailACH:
		response.WriteError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACH decide not implemented yet")
		return
	case decision.RailCheck:
	default:
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "rail must be check or ach")
		return
	}

	if req.Check == nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check payload required when rail=check")
		return
	}
	if strings.TrimSpace(req.Check.CheckNumber) == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.check_number is required")
		return
	}
	if req.Check.Amount < 0 {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.amount must be >= 0")
		return
	}

	presented := decisionsvc.PresentedCheck{
		CheckNumber: strings.TrimSpace(req.Check.CheckNumber),
		Amount:      req.Check.Amount,
		Payee:       req.Check.Payee,
	}
	if d := strings.TrimSpace(req.Check.IssueDate); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "check.issue_date must be YYYY-MM-DD")
			return
		}
		presented.IssueDate = &t
	}

	result, err := h.Decisions.Decide(r.Context(), decisionsvc.DecideInput{
		FIID:          req.FIID,
		AccountID:     req.AccountID,
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		Check:         presented,
		RawRequest:    req,
	})
	if err != nil {
		if errors.Is(err, decisionsvc.ErrFINotFound) {
			response.WriteError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "decide failed")
		return
	}
	response.WriteJSON(w, http.StatusOK, result)
}
