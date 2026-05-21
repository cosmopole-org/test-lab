package inputs_users

type DeleteInput struct {
	UserId string `json:"userId"`
}

func (d DeleteInput) GetData() any {
	return "dummy"
}

func (d DeleteInput) GetStoreId() string {
	return ""
}

func (d DeleteInput) Origin() string {
	return "global"
}
