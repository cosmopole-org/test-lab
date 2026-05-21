package inputs_machiner

type VmTerminalInput struct {
	CreatureId string `json:"creatureId" validate:"required"`
	VmId       string `json:"vmId" validate:"required"`
}

func (d VmTerminalInput) GetData() any {
	return "dummy"
}

func (d VmTerminalInput) GetStoreId() string {
	return ""
}

func (d VmTerminalInput) Origin() string {
	return "global"
}
