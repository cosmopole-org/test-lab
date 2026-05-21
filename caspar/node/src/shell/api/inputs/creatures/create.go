package inputs_creatures

type CreateInput struct {
	Type       string  `json:"type" validate:"required"`
	Username   string  `json:"username" validate:"required"`
	PublicKey  string  `json:"publicKey"`
	ChainId    *string `json:"chainId"`
	SubchainId *string `json:"subchainId"`
	OwnerId    *string `json:"ownerId"`
	Metadata   any     `json:"metadata"`
}

func (d CreateInput) GetData() any       { return "dummy" }
func (d CreateInput) GetStoreId() string { return "" }
func (d CreateInput) Origin() string     { return "global" }
