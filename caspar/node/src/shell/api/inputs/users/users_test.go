package inputs_users

import (
	"testing"

	"kasper/src/abstract/models/input"
)

func TestUsersInputsImplementInterface(t *testing.T) {
	var _ input.IInput = AuthenticateInput{}
	var _ input.IInput = CheckSignInput{}
	var _ input.IInput = ConsumeLockInput{}
	var _ input.IInput = CreateInput{}
	var _ input.IInput = DeleteInput{}
	var _ input.IInput = FindInput{}
	var _ input.IInput = GetInput{}
	var _ input.IInput = GetByUsernameInput{}
	var _ input.IInput = ListInput{}
	var _ input.IInput = LoginInput{}
	var _ input.IInput = LockTokenInput{}
	var _ input.IInput = MetaInput{}
	var _ input.IInput = MintInput{}
	var _ input.IInput = TransferInput{}
	var _ input.IInput = UpdateInput{}
}

func TestUsersInputOriginsAndStoreIds(t *testing.T) {
	cases := []struct {
		name  string
		in    input.IInput
		store string
		orig  string
	}{
		{"authenticate", AuthenticateInput{}, "", ""},
		{"create", CreateInput{}, "", "global"},
		{"update", UpdateInput{}, "", "global"},
		{"delete", DeleteInput{}, "", "global"},
		{"login", LoginInput{}, "", ""},
		{"mint", MintInput{}, "", "global"},
		{"transfer", TransferInput{}, "", "global"},
		{"lock", LockTokenInput{}, "", "global"},
		{"consume", ConsumeLockInput{}, "", "global"},
		{"checksign", CheckSignInput{}, "", ""},
		{"find", FindInput{}, "", ""},
		{"get", GetInput{}, "", ""},
		{"get-by-uname", GetByUsernameInput{}, "", ""},
		{"meta", MetaInput{}, "", "global"},
		{"list", ListInput{}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.GetStoreId(); got != tc.store {
				t.Fatalf("store mismatch got=%q want=%q", got, tc.store)
			}
			if got := tc.in.Origin(); got != tc.orig {
				t.Fatalf("origin mismatch got=%q want=%q", got, tc.orig)
			}
		})
	}
}
