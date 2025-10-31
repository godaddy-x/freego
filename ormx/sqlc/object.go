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

type DefaultObject struct{}

func (o *DefaultObject) GetTable() string {
	return ""
}

func (o *DefaultObject) NewObject() Object {
	return nil
}

func (o *DefaultObject) AppendObject(data interface{}, target Object) {
}

func (o *DefaultObject) NewIndex() []Index {
	return nil
}
