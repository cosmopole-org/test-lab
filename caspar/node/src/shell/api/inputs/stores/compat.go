package inputs_stores

type SignalInput struct {
	Type    string `json:"type"`
	StoreId string `json:"storeId"`
	UserId  string `json:"userId"`
	Data    string `json:"data"`
	Temp    bool   `json:"temp,omitempty"`
}

func (d SignalInput) GetStoreId() string { return d.StoreId }
func (d SignalInput) Origin() string     { return "" }

type JoinInput struct {
	StoreId string `json:"storeId"`
}
