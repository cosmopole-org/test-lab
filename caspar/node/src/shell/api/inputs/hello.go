package inputs

type HelloInput struct {
	Name string `json:"name"`
}

func (d HelloInput) GetData() any {
	return "dummy"
}

func (d HelloInput) GetStoreId() string {
	return ""
}

func (d HelloInput) Origin() string {
	return ""
}