package decide

// Facts are closed, code-specific machine fields on an exception.
// Keys and value types are normative per exception code (see docs/exceptions.md).
type Facts map[string]any

// Exception is one structured decide finding. Code and Facts are normative.
type Exception struct {
	Code  string `json:"code"`
	Facts Facts  `json:"facts,omitempty"`
}

// Match is the single artifact that authorized a pay.
// Same closed shape as an Exception. The response field is null when nothing authorized the item.
type Match struct {
	Code  string `json:"code"`
	Facts Facts  `json:"facts,omitempty"`
}

// Result is the hot-path decide response body.
// Exceptions carry failures. Match names the one artifact that authorized a pay.
type Result struct {
	Decision      string      `json:"decision"`
	Match         *Match      `json:"match"`
	Exceptions    []Exception `json:"exceptions"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	LatencyMs     int64       `json:"latency_ms"`
}

// NewResult builds a result with a non-nil exceptions slice.
func NewResult(decision string, excs []Exception) Result {
	if excs == nil {
		excs = []Exception{}
	}
	return Result{
		Decision:   decision,
		Exceptions: excs,
	}
}

// Hard returns a first-fail gate result (decision return, single exception).
func Hard(code string, facts Facts) Result {
	return NewResult(DecisionReturn, []Exception{{
		Code:  code,
		Facts: facts,
	}})
}
