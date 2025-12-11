package utils

import (
	"github.com/godaddy-x/freego/ormx/sqlc"
)

//easyjson:json
type Limit struct {
	// 分页条数
	Limit int64 `json:"limit"`
	// 总数据条数，查询条件prevID=0和lastID=0的时候触发查询count数量，一般在首页查询触发
	Total int64 `json:"total"`
	// 上一页的划分ID
	LastID int64 `json:"lastID"`
	// 下一页的划分ID
	PrevID int64 `json:"prevID"`
}

func LimitPager(sql *sqlc.Cnd, prevID, lastID int64) Limit {
	page := sql.GetPageResult()
	return Limit{Total: page.PageTotal, Limit: page.PageSize, PrevID: prevID, LastID: lastID}
}

func LoopPager(fast *sqlc.FastObject, size int, call func(index int)) {
	if fast.IsPrev {
		for i := size - 1; i >= 0; i-- {
			call(i)
		}
	} else {
		for i := 0; i < size; i++ {
			call(i)
		}
	}
}
