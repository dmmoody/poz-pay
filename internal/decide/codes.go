package decide

// Closed vocabulary tokens. Must stay aligned with DB lookup tables.

const (
	DecisionPay    = "pay"
	DecisionReturn = "return"
	DecisionReview = "review"
)

const (
	RailCheck = "check"
	RailACH   = "ach"
)

const (
	CodeAccountNotOurs  = "ACCOUNT_NOT_OURS"
	CodeAccountInactive = "ACCOUNT_INACTIVE"

	CodeCheckNotInRegister = "CHECK_NOT_IN_REGISTER"
	CodeCheckVoided        = "CHECK_VOIDED"
	CodeCheckAlreadyPaid   = "CHECK_ALREADY_PAID"
	CodeCheckExpired       = "CHECK_EXPIRED"
	CodeAmountMismatch     = "AMOUNT_MISMATCH"
	CodePayeeMismatch      = "PAYEE_MISMATCH"
	CodeIssueDateMismatch  = "ISSUE_DATE_MISMATCH"

	// CodeRegisterMatch is the single authorizing artifact on a clean check pay.
	CodeRegisterMatch = "REGISTER_MATCH"
)
