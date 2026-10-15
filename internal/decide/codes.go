package decide

// Closed vocabulary tokens. Must stay aligned with DB lookup tables.

const (
	DecisionPay    = "pay"
	DecisionReturn = "return"
	DecisionReview = "review"
)

const (
	SeverityHard = "hard"
	SeveritySoft = "soft"
)

const (
	RailCheck = "check"
	RailACH   = "ach"
)

const (
	ReasonOK = "OK"

	CodeAccountNotOurs  = "ACCOUNT_NOT_OURS"
	CodeAccountInactive = "ACCOUNT_INACTIVE"

	CodeCheckNotInRegister = "CHECK_NOT_IN_REGISTER"
	CodeCheckVoided        = "CHECK_VOIDED"
	CodeCheckAlreadyPaid   = "CHECK_ALREADY_PAID"
	CodeAmountMismatch     = "AMOUNT_MISMATCH"
	CodePayeeMismatch      = "PAYEE_MISMATCH"
	CodeIssueDateMismatch  = "ISSUE_DATE_MISMATCH"
)

// ReasonLabels are short display strings for reason_code (non-normative).
var ReasonLabels = map[string]string{
	ReasonOK:               "No exceptions; pay",
	CodeAccountNotOurs:     "Account not found or not owned by FI",
	CodeAccountInactive:    "Account inactive for positive pay",
	CodeCheckNotInRegister: "Check number not found in register",
	CodeCheckVoided:        "Check voided in register",
	CodeCheckAlreadyPaid:   "Check already paid/presented",
	CodeAmountMismatch:     "One or more check fields do not match register",
	CodePayeeMismatch:      "One or more check fields do not match register",
	CodeIssueDateMismatch:  "One or more check fields do not match register",
}
