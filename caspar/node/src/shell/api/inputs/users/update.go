package inputs_users

type UpdateInput struct {
	UserId    string  `json:"userId"`
	Metadata  any     `json:"metadata"`
	PublicKey *string `json:"publicKey"`
	Type      *string `json:"type"`
	Username  *string `json:"username"`
}

func (d UpdateInput) GetData() any {
	return "dummy"
}

func (d UpdateInput) GetStoreId() string {
	return ""
}

func (d UpdateInput) Origin() string {
	return "global"
}
