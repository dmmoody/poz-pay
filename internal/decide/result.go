package decide

// Exception is one structured decide finding. Code is normative.
type Exception struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// Result is the hot-path decide response body.
// Why is carried only by Exceptions (empty when decision is pay with no findings).
type Result struct {
	Decision      string      `json:"decision"`
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
func Hard(code, detail string) Result {
	return NewResult(DecisionReturn, []Exception{{
		Code:   code,
		Detail: detail,
	}})
}
