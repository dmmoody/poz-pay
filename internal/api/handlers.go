package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dmmoody/poz-pay/internal/checkpay"
	"github.com/dmmoody/poz-pay/internal/decide"
)

// DBPinger is the health-check dependency (usually *sql.DB).
type DBPinger interface {
	PingContext(ctx context.Context) error
}

// Handlers holds HTTP dependencies and method handlers.
type Handlers struct {
	Checks *checkpay.Service
	DB     DBPinger
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.DB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"degraded","database":"unconfigured"}`))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.DB.PingContext(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy","database":"down"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","database":"up"}`))
}

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

func (h *Handlers) Decide(w http.ResponseWriter, r *http.Request) {
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
	case decide.RailACH:
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACH decide not implemented yet")
		return
	case decide.RailCheck:
		// ok
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

	presented := checkpay.PresentedCheck{
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

	result, err := h.Checks.Decide(r.Context(), checkpay.DecideInput{
		FIID:          req.FIID,
		AccountID:     req.AccountID,
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		Check:         presented,
		RawRequest:    req,
	})
	if err != nil {
		if errors.Is(err, checkpay.ErrFINotFound) {
			writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "decide failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type postRequest struct {
	CheckNumber   string `json:"check_number"`
	PaidAt        string `json:"paid_at"`
	CorrelationID string `json:"correlation_id"`
}

func (h *Handlers) PostCheck(w http.ResponseWriter, r *http.Request) {
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

	payment, err := h.Checks.Post(r.Context(), checkpay.PostInput{
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

func (h *Handlers) ListCheckDecisions(w http.ResponseWriter, r *http.Request) {
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

func (h *Handlers) ListCheckAudit(w http.ResponseWriter, r *http.Request) {
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
	case errors.Is(err, checkpay.ErrFINotFound):
		writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
	case errors.Is(err, checkpay.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", err.Error())
	case errors.Is(err, checkpay.ErrCheckNotInRegister):
		writeError(w, http.StatusNotFound, "CHECK_NOT_IN_REGISTER", err.Error())
	case errors.Is(err, checkpay.ErrCheckVoided):
		writeError(w, http.StatusConflict, "CHECK_VOIDED", err.Error())
	case errors.Is(err, checkpay.ErrCheckNumberRequired), errors.Is(err, checkpay.ErrPaidAtRequired):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		return false
	}
	return true
}

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
	BatchID  string                     `json:"batch_id"`
	Accepted int                        `json:"accepted"`
	Rejected int                        `json:"rejected"`
	Errors   []checkpay.UploadItemError `json:"errors"`
}

func (h *Handlers) UploadCheckRegister(w http.ResponseWriter, r *http.Request) {
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

	items := make([]checkpay.UploadItem, 0, len(req.Items))
	for _, in := range req.Items {
		it := checkpay.UploadItem{
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
		if errors.Is(err, checkpay.ErrFINotFound) {
			writeError(w, http.StatusNotFound, "FI_NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, checkpay.ErrPartyNotFound) {
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

func looksLikeUUID(s string) bool {
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
