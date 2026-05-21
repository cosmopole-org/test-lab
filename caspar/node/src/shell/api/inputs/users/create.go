package inputs_users

type CreateInput struct {
	Username  string `json:"username" validate:"required"`
	PublicKey string `json:"publicKey" validate:"required"`
	Metadata  any    `json:"metadata"`
}

func (d CreateInput) GetData() any {
	return "dummy"
}

func (d CreateInput) GetStoreId() string {
	return ""
}

func (d CreateInput) Origin() string {
	return "global"
}
