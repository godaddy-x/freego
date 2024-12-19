package utils

import (
	"time"
	_ "unsafe"
)

//go:linkname now time.now
func now() (sec int64, nsec int32, mono int64)

// UnixNano 直接调用底层方法比time.Now()性能提升1倍
func UnixNano() int64 {
	s, m, _ := now()
	return s*1e9 + int64(m)
}

// 获取当前时间/秒
func UnixSecond() int64 {
	s, _, _ := now()
	return s
}

// 获取当前时间/毫秒
func UnixMilli() int64 {
	return UnixNano() / 1e6
}

// 时间戳转time
func Int2Time(t int64) time.Time {
	return time.Unix(0, t*1e6)
}

// 时间戳转格式字符串/毫秒, 例: 2023-07-22 08:47:27.379
func Time2Str(t int64) string {
	return Int2Time(t).In(Cst_sh).Format(Time_fmt)
}

// 时间戳转格式字符串/毫秒
func Time2DateStr(t int64) string {
	return Int2Time(t).In(Cst_sh).Format(date_fmt)
}

// 时间戳转格式字符串/毫秒
func Time2FormatStr(t int64, local *time.Location, fmt string) string {
	if local == nil {
		return Int2Time(t).In(Cst_sh).Format(fmt)
	}
	return Int2Time(t).In(local).Format(fmt)
}

// 格式字符串转时间戳/毫秒
// 2023-07-22 08:47:27.379
// 2023-07-22 08:47:27
// 2023-07-22
func Str2Time(s string) (int64, error) {
	if len(s) == 10 {
		s += " 00:00:00.000"
	} else if len(s) == 19 {
		s += ".000"
	}
	t, err := time.ParseInLocation(Time_fmt, s, Cst_sh)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

// 格式字符串转时间戳/毫秒
func Str2Date(s string) (int64, error) {
	t, err := time.ParseInLocation(date_fmt, s, Cst_sh)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

// 格式字符串转时间戳/毫秒
func Str2FormatDate(s, fmt string, local *time.Location) (int64, error) {
	t, err := time.ParseInLocation(fmt, s, local)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

// 获取当前月份开始和结束时间
func GetMonthFirstAndLast() (int64, int64) {
	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, now.Location())
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	return firstOfMonth.UnixMilli(), lastOfMonth.UnixMilli() + OneDay
}

// 获取指定月份开始和结束时间
func GetAnyMonthFirstAndLast(month int) (int64, int64) {
	now := time.Now()
	currentYear, currentMonth, _ := now.Date()
	cmonth := int(currentMonth)
	offset := month - cmonth
	if month < 1 || month > 12 {
		offset = 0
	}
	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, now.Location()).AddDate(0, offset, 0)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	return firstOfMonth.UnixMilli(), lastOfMonth.UnixMilli() + OneDay
}

// 获取当前星期开始和结束时间
func GetWeekFirstAndLast() (int64, int64) {
	now := time.Now()
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, offset)
	first := start.UnixMilli()
	return first, first + OneWeek
}

// 获取当天开始和结束时间
func GetDayFirstAndLast() (int64, int64) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := start.UnixMilli()
	return first, first + OneDay
}

// 获取x天开始和结束时间,最多30天
func GetAnyDayFirstAndLast(x int64) (int64, int64) {
	if x < 0 {
		x = 0
	}
	if x > 30 {
		x = 30
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := start.UnixMilli()
	before := x * OneDay
	return first - before, first + OneDay - before
}

// 获取x天开始和当天结束时间,最多30天
func GetInDayFirstAndLast(x int64) (int64, int64) {
	if x < 0 {
		x = 0
	}
	if x > 30 {
		x = 30
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := start.UnixMilli()
	before := x * OneDay
	return first - before, first + OneDay
}

// 获取时间的0点
func GetFmtDate(t int64) int64 {
	now := Int2Time(t)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := start.UnixMilli()
	return first
}
