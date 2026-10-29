package checkpay

import (
	"time"

	"github.com/dmmoody/poz-pay/internal/decide"
)

// DefaultMaxAgeDays is the UCC-aligned stale window (~6 months) when policy omits max age.
const DefaultMaxAgeDays = 180

// Account is the resolved account for decide (tenant slice).
type Account struct {
	ID                 string
	FIID               string
	PartyID            string
	Active             bool
	PositivePayEnabled bool
}

// RegisterItem is an issued check row used for matching.
type RegisterItem struct {
	ID                string
	CheckNumber       string
	Amount            int64
	Payee             string
	PayeeNormalized   string
	IssueDate         *time.Time // date-only; wall date in UTC noon or date truncated
	VoidedAt          *time.Time
	PaidAt            *time.Time
	PaidCorrelationID string
}

// PresentedCheck is the inbound presented item.
type PresentedCheck struct {
	CheckNumber string
	Amount      int64
	Payee       string
	IssueDate   *time.Time // optional
}

// Policy controls check decide outcomes (FI/party check settings).
type Policy struct {
	// ReturnOnMismatch maps field mismatches to decision return (wash theater / strict).
	ReturnOnMismatch bool
	// MaxAgeDays is days after register issue_date before CHECK_EXPIRED.
	// <=0 means DefaultMaxAgeDays (180).
	MaxAgeDays int
}

func (p Policy) effectiveMaxAgeDays() int {
	if p.MaxAgeDays <= 0 {
		return DefaultMaxAgeDays
	}
	return p.MaxAgeDays
}

// EvaluateCheck runs the contractual check pipeline against an already-resolved
// account and optional register item (nil = not found / not active).
//
// asOf is the decide instant (UTC); zero means time.Now().UTC().
//
// Voided/paid items should be passed in when found including voided rows so
// CHECK_VOIDED / CHECK_ALREADY_PAID can fire; callers that only query
// active rows pass nil for missing → CHECK_NOT_IN_REGISTER.
func EvaluateCheck(acct *Account, item *RegisterItem, presented PresentedCheck, policy Policy, asOf time.Time) decide.Result {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	if acct == nil || acct.FIID == "" || acct.ID == "" {
		return decide.Hard(decide.CodeAccountNotOurs, nil)
	}
	if !acct.Active || !acct.PositivePayEnabled {
		return decide.Hard(decide.CodeAccountInactive, nil)
	}

	if item == nil {
		return decide.Hard(decide.CodeCheckNotInRegister, decide.Facts{
			"check_number": presented.CheckNumber,
		})
	}
	if item.VoidedAt != nil {
		return decide.Hard(decide.CodeCheckVoided, decide.Facts{
			"check_number": presented.CheckNumber,
		})
	}
	if item.PaidAt != nil {
		return decide.Hard(decide.CodeCheckAlreadyPaid, decide.Facts{
			"check_number": presented.CheckNumber,
		})
	}

	if item.IssueDate != nil {
		maxAge := policy.effectiveMaxAgeDays()
		age := calendarDaysBetween(*item.IssueDate, asOf)
		if age > maxAge {
			return decide.Hard(decide.CodeCheckExpired, decide.Facts{
				"check_number": presented.CheckNumber,
				"issue_date":   truncateDateUTC(*item.IssueDate).Format("2006-01-02"),
				"age_days":     age,
				"max_age_days": maxAge,
			})
		}
	}

	var excs []decide.Exception
	if presented.Amount != item.Amount {
		excs = append(excs, decide.Exception{
			Code: decide.CodeAmountMismatch,
			Facts: decide.Facts{
				"presented_amount": presented.Amount,
				"issued_amount":    item.Amount,
			},
		})
	}

	presentedPayee := NormalizePayee(presented.Payee)
	issuedPayee := item.PayeeNormalized
	if issuedPayee == "" {
		issuedPayee = NormalizePayee(item.Payee)
	}
	if presentedPayee != issuedPayee {
		excs = append(excs, decide.Exception{
			Code: decide.CodePayeeMismatch,
			Facts: decide.Facts{
				"presented_payee": presentedPayee,
				"issued_payee":    issuedPayee,
			},
		})
	}

	if presented.IssueDate != nil && item.IssueDate != nil {
		p := truncateDateUTC(*presented.IssueDate)
		i := truncateDateUTC(*item.IssueDate)
		if !p.Equal(i) {
			excs = append(excs, decide.Exception{
				Code: decide.CodeIssueDateMismatch,
				Facts: decide.Facts{
					"presented_issue_date": p.Format("2006-01-02"),
					"issued_issue_date":    i.Format("2006-01-02"),
				},
			})
		}
	}

	if len(excs) == 0 {
		result := decide.NewResult(decide.DecisionPay, nil)
		result.Match = &decide.Match{
			Code: decide.CodeRegisterMatch,
			Facts: decide.Facts{
				"register_item_id": item.ID,
			},
		}
		return result
	}

	decision := decide.DecisionReview
	if policy.ReturnOnMismatch {
		decision = decide.DecisionReturn
	}
	return decide.NewResult(decision, excs)
}

func truncateDateUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// calendarDaysBetween is whole UTC calendar days from start date to asOf date (can be negative).
func calendarDaysBetween(start, asOf time.Time) int {
	s := truncateDateUTC(start)
	a := truncateDateUTC(asOf)
	return int(a.Sub(s) / (24 * time.Hour))
}
