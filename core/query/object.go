package sqlc

type Index struct {
	Name   string
	Key    []string
	Keys   []KV
	Unique bool
	Sparse bool // MongoDB：仅索引存在该字段的文档（reqType=1 无 pendingTradeSign 时不参与唯一约束）
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
