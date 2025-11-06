package sqlc

type Index struct {
	Name   string
	Key    []string
	Unique bool
}

type Object interface {
	GetTable() string
	NewObject() Object
	AppendObject(data interface{}, target Object)
	NewIndex() []Index
}
