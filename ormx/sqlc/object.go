package sqlc

type Index struct {
	Name   string
	Key    []string
	Keys []KV
	Unique bool
}

type KV struct {
	K string
	V int
}

type Object interface {
	GetTable() string
	NewObject() Object
	AppendObject(data interface{}, target Object)
	NewIndex() []Index
}
