package decide

// Exception is one structured decide finding. Code/Severity are normative.
type Exception struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

// Result is the hot-path decide response body.
type Result struct {
	Decision      string      `json:"decision"`
	ReasonCode    string      `json:"reason_code"`
	Reason        string      `json:"reason"`
	Exceptions    []Exception `json:"exceptions"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	LatencyMs     int64       `json:"latency_ms"`
}

// NewResult builds a result with a non-nil exceptions slice and filled reason text.
func NewResult(decision, reasonCode string, excs []Exception) Result {
	if excs == nil {
		excs = []Exception{}
	}
	reason := ReasonLabels[reasonCode]
	if reason == "" {
		reason = reasonCode
	}
	return Result{
		Decision:   decision,
		ReasonCode: reasonCode,
		Reason:     reason,
		Exceptions: excs,
	}
}

// Hard returns a first-fail hard exception result (decision return).
func Hard(code, detail string) Result {
	return NewResult(DecisionReturn, code, []Exception{{
		Code:     code,
		Severity: SeverityHard,
		Detail:   detail,
	}})
}
