package inputs_auth

import (
	"testing"

	"kasper/src/abstract/models/input"
)

func TestAuthInputsImplementInterfaceAndScope(t *testing.T) {
	var _ input.IInput = GetServersMapInput{}
	var _ input.IInput = GetServerKeyInput{}

	all := []input.IInput{GetServersMapInput{}, GetServerKeyInput{}}
	for _, in := range all {
		if in.GetStoreId() != "" || in.Origin() != "" {
			t.Fatalf("unexpected auth input values store=%q origin=%q", in.GetStoreId(), in.Origin())
		}
	}
}
