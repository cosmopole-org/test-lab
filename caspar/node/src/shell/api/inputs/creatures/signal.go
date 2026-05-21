package inputs_creatures

type SignalInput struct {
	Type       string `json:"type" validate:"required"` // pvp | all
	Data       string `json:"data" validate:"required"`
	StoreId    string `json:"storeId"`
	CreatureId string `json:"creatureId"`
	ProgramId  string `json:"programId,omitempty"`
	EntityId   string `json:"entityId,omitempty"`
	Temp       bool   `json:"temp"`
}

func (d SignalInput) GetData() any       { return "dummy" }
func (d SignalInput) GetStoreId() string { return d.StoreId }
func (d SignalInput) Origin() string     { return "" }
