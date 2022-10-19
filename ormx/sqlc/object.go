package sqlc

type Index struct {
	Name   string         `bson:"name"`
	Key    map[string]int `bson:"key"`
	Unique bool           `bson:"unique"`
	V      int            `bson:"v"`
	NS     string         `bson:"ns"`
}

type Object interface {
	GetTable() string
	NewObject() Object
	NewIndex() []Index
}

type DefaultObject struct{}

func (o *DefaultObject) GetTable() string {
	return ""
}

func (o *DefaultObject) NewObject() Object {
	return nil
}

func (o *DefaultObject) NewIndex() []Index {
	return nil
}
