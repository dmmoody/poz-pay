package checkpay

import (
	"time"

	"github.com/dmmoody/poz-pay/internal/decision"
)

// PresentedCheck is the inbound presented item.
type PresentedCheck struct {
	CheckNumber string
	Amount      int64
	Payee       string
	IssueDate   *time.Time // optional
}

func maxAgeDays(p Policy) int {
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
func EvaluateCheck(acct *Account, item *RegisterItem, presented PresentedCheck, policy Policy, asOf time.Time) decision.Result {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	if acct == nil || acct.FIID == "" || acct.ID == "" {
		return decision.Hard(decision.CodeAccountNotOurs, nil)
	}
	if !acct.Active || !acct.PositivePayEnabled {
		return decision.Hard(decision.CodeAccountInactive, nil)
	}

	if item == nil {
		return decision.Hard(decision.CodeCheckNotInRegister, decision.Facts{
			"check_number": presented.CheckNumber,
		})
	}
	if item.VoidedAt != nil {
		return decision.Hard(decision.CodeCheckVoided, decision.Facts{
			"check_number": presented.CheckNumber,
		})
	}
	if item.PaidAt != nil {
		return decision.Hard(decision.CodeCheckAlreadyPaid, decision.Facts{
			"check_number": presented.CheckNumber,
		})
	}

	if item.IssueDate != nil {
		maxAge := maxAgeDays(policy)
		age := calendarDaysBetween(*item.IssueDate, asOf)
		if age > maxAge {
			return decision.Hard(decision.CodeCheckExpired, decision.Facts{
				"check_number": presented.CheckNumber,
				"issue_date":   truncateDateUTC(*item.IssueDate).Format("2006-01-02"),
				"age_days":     age,
				"max_age_days": maxAge,
			})
		}
	}

	var excs []decision.Exception
	if presented.Amount != item.Amount {
		excs = append(excs, decision.Exception{
			Code: decision.CodeAmountMismatch,
			Facts: decision.Facts{
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
		excs = append(excs, decision.Exception{
			Code: decision.CodePayeeMismatch,
			Facts: decision.Facts{
				"presented_payee": presentedPayee,
				"issued_payee":    issuedPayee,
			},
		})
	}

	if presented.IssueDate != nil && item.IssueDate != nil {
		p := truncateDateUTC(*presented.IssueDate)
		i := truncateDateUTC(*item.IssueDate)
		if !p.Equal(i) {
			excs = append(excs, decision.Exception{
				Code: decision.CodeIssueDateMismatch,
				Facts: decision.Facts{
					"presented_issue_date": p.Format("2006-01-02"),
					"issued_issue_date":    i.Format("2006-01-02"),
				},
			})
		}
	}

	if len(excs) == 0 {
		result := decision.NewResult(decision.DecisionPay, nil)
		result.Match = &decision.Match{
			Code: decision.CodeRegisterMatch,
			Facts: decision.Facts{
				"register_item_id": item.ID,
			},
		}
		return result
	}

	outcome := decision.DecisionReview
	if policy.ReturnOnMismatch {
		outcome = decision.DecisionReturn
	}
	return decision.NewResult(outcome, excs)
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
