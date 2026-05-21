package inputs_users

type GetInput struct {
	UserId string `json:"userId" validate:"required"`
}

func (d GetInput) GetData() any {
	return "dummy"
}

func (d GetInput) GetStoreId() string {
	return ""
}

func (d GetInput) Origin() string {
	return ""
}
