package info

type IInfo interface {
	IsGod() bool
	UserId() string
	StoreId() string
	Identity() (string, string)
}
