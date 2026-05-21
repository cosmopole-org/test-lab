package inputs_users

type GetByUsernameInput struct {
	Username string `json:"username" validate:"required"`
}

func (d GetByUsernameInput) GetData() any {
	return "dummy"
}

func (d GetByUsernameInput) GetStoreId() string {
	return ""
}

func (d GetByUsernameInput) Origin() string {
	return ""
}
