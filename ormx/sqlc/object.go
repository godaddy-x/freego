package sqlc

type Object interface {
	GetTable() string
	NewObject() Object
}

type DefaultObject struct{}

func (o *DefaultObject) GetTable() string {
	return ""
}

func (o *DefaultObject) NewObject() Object {
	return nil
}
