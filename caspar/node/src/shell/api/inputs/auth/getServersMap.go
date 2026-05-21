package inputs_auth

type GetServersMapInput struct{}

func (d GetServersMapInput) GetData() any {
	return "dummy"
}

func (d GetServersMapInput) GetStoreId() string {
	return ""
}

func (d GetServersMapInput) Origin() string {
	return ""
}