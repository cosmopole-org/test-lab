package inputs_machiner

import (
	"testing"

	"kasper/src/abstract/models/input"
)

func TestMachineInputsImplementInterface(t *testing.T) {
	var _ input.IInput = ListAppMachsInput{}
	var _ input.IInput = ReadVmLogsInput{}
	var _ input.IInput = CreateAppInput{}
	var _ input.IInput = CreateMachineInput{}
	var _ input.IInput = DeleteAppInput{}
	var _ input.IInput = DeleteProgramInput{}
	var _ input.IInput = DeployInput{}
	var _ input.IInput = ListInput{}
	var _ input.IInput = MachineBuildsInput{}
	var _ input.IInput = RunProgramEntityInput{}
	var _ input.IInput = SignalInput{}
	var _ input.IInput = UpdateAppInput{}
	var _ input.IInput = UpdateProgramInput{}
	var _ input.IInput = VmTerminalInput{}
}

func TestMachineInputOriginsAndStoreIds(t *testing.T) {
	cases := []struct {
		in   input.IInput
		orig string
	}{
		{ListAppMachsInput{}, ""},
		{ListInput{}, ""},
		{SignalInput{}, ""},
		{ReadVmLogsInput{}, "global"},
		{CreateAppInput{}, "global"},
		{CreateMachineInput{}, "global"},
		{DeleteAppInput{}, "global"},
		{DeleteProgramInput{}, "global"},
		{DeployInput{}, "global"},
		{MachineBuildsInput{}, "global"},
		{RunProgramEntityInput{}, "global"},
		{UpdateAppInput{}, "global"},
		{UpdateProgramInput{}, "global"},
		{VmTerminalInput{}, "global"},
	}
	for i, c := range cases {
		if c.in.GetStoreId() != "" {
			t.Fatalf("case %d expected empty store id", i)
		}
		if c.in.Origin() != c.orig {
			t.Fatalf("case %d origin got=%q want=%q", i, c.in.Origin(), c.orig)
		}
	}
}
