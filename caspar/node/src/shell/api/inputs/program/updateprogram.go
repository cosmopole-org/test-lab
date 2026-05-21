package inputs_machiner

type UpdateProgramInput struct {
	ProgramId string         `json:"programId" validate:"required"`
	Path      string         `json:"path" validate:"required"`
	Metadata  map[string]any `json:"metadata"`
}

func (d UpdateProgramInput) GetData() any {
	return "dummy"
}

func (d UpdateProgramInput) GetStoreId() string {
	return ""
}

func (d UpdateProgramInput) Origin() string {
	return "global"
}
