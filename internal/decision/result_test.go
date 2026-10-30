package decision_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmmoody/poz-pay/internal/decision"
)

func TestResultJSON_matchNullWhenNothingAuthorized(t *testing.T) {
	t.Parallel()
	r := decision.NewResult(decision.DecisionReturn, []decision.Exception{{
		Code: decision.CodeCheckVoided,
	}})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"match":null`) {
		t.Fatalf("match=%s", b)
	}
}

func TestResultJSON_includesAuthorizingMatch(t *testing.T) {
	t.Parallel()
	r := decision.NewResult(decision.DecisionPay, nil)
	r.Match = &decision.Match{
		Code:  decision.CodeRegisterMatch,
		Facts: decision.Facts{"register_item_id": "reg-1"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Decision   string               `json:"decision"`
		Match      *decision.Match      `json:"match"`
		Exceptions []decision.Exception `json:"exceptions"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Decision != decision.DecisionPay || len(got.Exceptions) != 0 {
		t.Fatalf("body=%s", b)
	}
	if got.Match == nil || got.Match.Code != decision.CodeRegisterMatch || got.Match.Facts["register_item_id"] != "reg-1" {
		t.Fatalf("match=%+v", got.Match)
	}
}
