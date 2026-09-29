package httpapi

import (
	"encoding/json"
	"math"
	"testing"
)

func TestPlanDefaultsAndValidation(t *testing.T) {
	name := "月卡"
	request := planRequest{Name: &name}
	input, err := request.withDefaults().validate()
	if err != nil {
		t.Fatalf("validate defaults: %v", err)
	}
	if input.Currency != "CNY" || input.ValidityUnit != "month" || input.ValidityValue != 1 || input.ResetQuota != 1 || input.Status != 1 {
		t.Fatalf("unexpected defaults: %+v", input)
	}
}

func TestPlanValidationRejectsInvalidResetMode(t *testing.T) {
	name := "bad"
	mode := 2
	_, err := (planRequest{Name: &name, ResetQuota: &mode}).withDefaults().validate()
	if err == nil || err.Error() != "流量重置模式参数错误" {
		t.Fatalf("expected reset mode validation error, got %v", err)
	}
}

func TestRequestInt64AcceptsDatabaseAndJSONNumberTypes(t *testing.T) {
	const want = int64(9007199254740993)
	for _, value := range []any{want, uint64(want), json.Number("9007199254740993"), "9007199254740993"} {
		got, err := requestInt64(value)
		if err != nil || got != want {
			t.Fatalf("requestInt64(%T(%v)) = %d, %v; want %d", value, value, got, err, want)
		}
	}
	if _, err := requestInt64(uint64(math.MaxInt64) + 1); err == nil {
		t.Fatal("expected overflowing uint64 to fail")
	}
}
