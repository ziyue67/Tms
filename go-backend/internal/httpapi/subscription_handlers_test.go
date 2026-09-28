package httpapi

import "testing"

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
