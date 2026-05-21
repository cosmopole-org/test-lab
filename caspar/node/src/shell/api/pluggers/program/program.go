package plugger_program

import (
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	actions "kasper/src/shell/api/actions/program"
	"kasper/src/shell/utils"
)

type Plugger struct {
	Id      *string
	Actions *actions.Actions
	Core    core.ICore
}

func (c *Plugger) CreateProgram() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.CreateProgram)
}

func (c *Plugger) DeleteProgram() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.DeleteProgram)
}

func (c *Plugger) UpdateProgram() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.UpdateProgram)
}

func (c *Plugger) RunProgramEntity() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.RunProgramEntity)
}

func (c *Plugger) StopProgramEntity() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.StopProgramEntity)
}

func (c *Plugger) OpenVmTerminal() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.OpenVmTerminal)
}

func (c *Plugger) CloseVmTerminal() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.CloseVmTerminal)
}

func (c *Plugger) ReadVmLogs() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.ReadVmLogs)
}

func (c *Plugger) ReadMachineBuilds() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.ReadMachineBuilds)
}

func (c *Plugger) Deploy() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.Deploy)
}

func (c *Plugger) ListPrograms() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.ListPrograms)
}

func (c *Plugger) ListMachines() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.ListMachines)
}

func (c *Plugger) ListProgramMachines() iaction.IAction {
	return utils.ExtractSecureAction(c.Core, c.Actions.ListProgramMachines)
}

func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
	err := actions.Install(a, extra...)
	if err != nil {
		panic(err)
	}
	return c
}

func New(actions *actions.Actions, core core.ICore) *Plugger {
	id := "program"
	return &Plugger{Id: &id, Actions: actions, Core: core}
}
