package vaidate

import "testing"

type sampleInput struct {
	Name string `validate:"notblank"`
}

func TestLoadValidationSystemAndNotBlankRule(t *testing.T) {
	LoadValidationSystem()
	if Validate == nil {
		t.Fatal("expected validation system to be initialized")
	}

	if err := Validate.Struct(sampleInput{Name: "ok"}); err != nil {
		t.Fatalf("expected valid input, got error: %v", err)
	}

	if err := Validate.Struct(sampleInput{Name: "   "}); err == nil {
		t.Fatal("expected whitespace-only string to fail notblank validation")
	}
}
