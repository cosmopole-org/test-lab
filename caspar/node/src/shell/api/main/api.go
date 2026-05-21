package plugger_api

import (
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"reflect"

	action_auth "kasper/src/shell/api/actions/auth"
	plugger_auth "kasper/src/shell/api/pluggers/auth"

	action_creature "kasper/src/shell/api/actions/creature"
	plugger_creature "kasper/src/shell/api/pluggers/creature"

	action_dummy "kasper/src/shell/api/actions/dummy"
	plugger_dummy "kasper/src/shell/api/pluggers/dummy"

	action_program "kasper/src/shell/api/actions/program"
	plugger_program "kasper/src/shell/api/pluggers/program"
)

func PlugThePlugger(core core.ICore, plugger interface{}) {
	s := reflect.TypeOf(plugger)
	for i := 0; i < s.NumMethod(); i++ {
		f := s.Method(i)
		if f.Name != "Install" {
			result := f.Func.Call([]reflect.Value{reflect.ValueOf(plugger)})
			action := result[0].Interface().(iaction.IAction)
			core.Actor().InjectAction(action)
		}
	}
}

func PlugAll(core core.ICore, modelExtender map[string]map[string]iaction.ExtendedField) {

	a_auth := &action_auth.Actions{App: core}
	p_auth := plugger_auth.New(a_auth, core)
	PlugThePlugger(core, p_auth)
	p_auth.Install(a_auth, modelExtender)

	a_creature := &action_creature.Actions{App: core}
	p_creature := plugger_creature.New(a_creature, core)
	PlugThePlugger(core, p_creature)
	p_creature.Install(a_creature, modelExtender)

	a_dummy := &action_dummy.Actions{App: core}
	p_dummy := plugger_dummy.New(a_dummy, core)
	PlugThePlugger(core, p_dummy)
	p_dummy.Install(a_dummy, modelExtender)

	a_program := &action_program.Actions{App: core}
	p_program := plugger_program.New(a_program, core)
	PlugThePlugger(core, p_program)
	p_program.Install(a_program, modelExtender)

}
