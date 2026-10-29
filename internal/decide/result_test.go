package decide_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmmoody/poz-pay/internal/decide"
)

func TestResultJSON_matchNullWhenNothingAuthorized(t *testing.T) {
	t.Parallel()
	r := decide.NewResult(decide.DecisionReturn, []decide.Exception{{
		Code: decide.CodeCheckVoided,
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
	r := decide.NewResult(decide.DecisionPay, nil)
	r.Match = &decide.Match{
		Code:  decide.CodeRegisterMatch,
		Facts: decide.Facts{"register_item_id": "reg-1"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Decision   string             `json:"decision"`
		Match      *decide.Match      `json:"match"`
		Exceptions []decide.Exception `json:"exceptions"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Decision != decide.DecisionPay || len(got.Exceptions) != 0 {
		t.Fatalf("body=%s", b)
	}
	if got.Match == nil || got.Match.Code != decide.CodeRegisterMatch || got.Match.Facts["register_item_id"] != "reg-1" {
		t.Fatalf("match=%+v", got.Match)
	}
}
