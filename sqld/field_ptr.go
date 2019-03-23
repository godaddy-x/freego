package sqld

import "unsafe"

// 通过指针获取对象字段位置
func GetPtr(v interface{}, offset uintptr) uintptr {
	structInter := (interface{})(v)
	structPtr := (*emptyInter)(unsafe.Pointer(&structInter)).w
	return uintptr(structPtr) + offset
}

// get string value
func GetString(ptr uintptr) string {
	return *((*string)(unsafe.Pointer(ptr)))
}

// set string value
func SetString(ptr uintptr, v string) {
	*((*string)(unsafe.Pointer(ptr))) = v
}

// get int value
func GetInt(ptr uintptr) int {
	return *((*int)(unsafe.Pointer(ptr)))
}

// set int value
func SetInt(ptr uintptr, v int) {
	*((*int)(unsafe.Pointer(ptr))) = v
}

// get int8 value
func GetInt8(ptr uintptr) int8 {
	return *((*int8)(unsafe.Pointer(ptr)))
}

// set int8 value
func SetInt8(ptr uintptr, v int8) {
	*((*int8)(unsafe.Pointer(ptr))) = v
}

// get int16 value
func GetInt16(ptr uintptr) int16 {
	return *((*int16)(unsafe.Pointer(ptr)))
}

// set int16 value
func SetInt16(ptr uintptr, v int16) {
	*((*int16)(unsafe.Pointer(ptr))) = v
}

// get int32 value
func GetInt32(ptr uintptr) int32 {
	return *((*int32)(unsafe.Pointer(ptr)))
}

// set int32 value
func SetInt32(ptr uintptr, v int32) {
	*((*int32)(unsafe.Pointer(ptr))) = v
}

// get int64 value
func GetInt64(ptr uintptr) int64 {
	return *((*int64)(unsafe.Pointer(ptr)))
}

// set int64 value
func SetInt64(ptr uintptr, v int64) {
	*((*int64)(unsafe.Pointer(ptr))) = v
}

// get float32 value
func GetFloat32(ptr uintptr) float32 {
	return *((*float32)(unsafe.Pointer(ptr)))
}

// set float32 value
func SetFloat32(ptr uintptr, v float32) {
	*((*float32)(unsafe.Pointer(ptr))) = v
}

// get float64 value
func GetFloat64(ptr uintptr) float64 {
	return *((*float64)(unsafe.Pointer(ptr)))
}

// set float32 value
func SetFloat64(ptr uintptr, v float64) {
	*((*float64)(unsafe.Pointer(ptr))) = v
}

// get bool value
func GetBool(ptr uintptr) bool {
	return *((*bool)(unsafe.Pointer(ptr)))
}

// set bool value
func SetBool(ptr uintptr, v bool) {
	*((*bool)(unsafe.Pointer(ptr))) = v
}

// get uint value
func GetUint(ptr uintptr) uint {
	return *((*uint)(unsafe.Pointer(ptr)))
}

// set uint value
func SetUint(ptr uintptr, v uint) {
	*((*uint)(unsafe.Pointer(ptr))) = v
}

// get uint8 value
func GetUint8(ptr uintptr) uint8 {
	return *((*uint8)(unsafe.Pointer(ptr)))
}

// set uint value
func SetUint8(ptr uintptr, v uint8) {
	*((*uint8)(unsafe.Pointer(ptr))) = v
}

// get uint16 value
func GetUint16(ptr uintptr) uint16 {
	return *((*uint16)(unsafe.Pointer(ptr)))
}

// set uint16 value
func SetUint16(ptr uintptr, v uint16) {
	*((*uint16)(unsafe.Pointer(ptr))) = v
}

// get uint32 value
func GetUint32(ptr uintptr) uint32 {
	return *((*uint32)(unsafe.Pointer(ptr)))
}

// set uint32 value
func SetUint32(ptr uintptr, v uint32) {
	*((*uint32)(unsafe.Pointer(ptr))) = v
}

// get uint64 value
func GetUint64(ptr uintptr) uint64 {
	return *((*uint64)(unsafe.Pointer(ptr)))
}

// set uint64 value
func SetUint64(ptr uintptr, v uint64) {
	*((*uint64)(unsafe.Pointer(ptr))) = v
}

// get []string value
func GetStringArr(ptr uintptr) []string {
	return *((*[]string)(unsafe.Pointer(ptr)))
}

// set []string value
func SetStringArr(ptr uintptr, v []string) {
	*((*[]string)(unsafe.Pointer(ptr))) = v
}

// get []int value
func GetIntArr(ptr uintptr) []int {
	return *((*[]int)(unsafe.Pointer(ptr)))
}

// set []int value
func SetIntArr(ptr uintptr, v []int) {
	*((*[]int)(unsafe.Pointer(ptr))) = v
}

// get []int8 value
func GetInt8Arr(ptr uintptr) []int8 {
	return *((*[]int8)(unsafe.Pointer(ptr)))
}

// set []int8 value
func SetInt8Arr(ptr uintptr, v []int8) {
	*((*[]int8)(unsafe.Pointer(ptr))) = v
}

// get []int16 value
func GetInt16Arr(ptr uintptr) []int16 {
	return *((*[]int16)(unsafe.Pointer(ptr)))
}

// set []int16 value
func SetInt16Arr(ptr uintptr, v []int16) {
	*((*[]int16)(unsafe.Pointer(ptr))) = v
}

// get []int32 value
func GetInt32Arr(ptr uintptr) []int32 {
	return *((*[]int32)(unsafe.Pointer(ptr)))
}

// set []int32 value
func SetInt32Arr(ptr uintptr, v []int32) {
	*((*[]int32)(unsafe.Pointer(ptr))) = v
}

// get []int64 value
func GetInt64Arr(ptr uintptr) []int64 {
	return *((*[]int64)(unsafe.Pointer(ptr)))
}

// set []int64 value
func SetInt64Arr(ptr uintptr, v []int64) {
	*((*[]int64)(unsafe.Pointer(ptr))) = v
}

// get []float32 value
func GetFloat32Arr(ptr uintptr) []float32 {
	return *((*[]float32)(unsafe.Pointer(ptr)))
}

// set []float32 value
func SetFloat32Arr(ptr uintptr, v []float32) {
	*((*[]float32)(unsafe.Pointer(ptr))) = v
}

// get []float64 value
func GetFloat64Arr(ptr uintptr) []float64 {
	return *((*[]float64)(unsafe.Pointer(ptr)))
}

// set []float64 value
func SetFloat64Arr(ptr uintptr, v []float64) {
	*((*[]float64)(unsafe.Pointer(ptr))) = v
}

// get []bool value
func GetBoolArr(ptr uintptr) []bool {
	return *((*[]bool)(unsafe.Pointer(ptr)))
}

// set []bool value
func SetBoolArr(ptr uintptr, v []bool) {
	*((*[]bool)(unsafe.Pointer(ptr))) = v
}

// get []uint value
func GetUintArr(ptr uintptr) []uint {
	return *((*[]uint)(unsafe.Pointer(ptr)))
}

// set []uint value
func SetUintArr(ptr uintptr, v []uint) {
	*((*[]uint)(unsafe.Pointer(ptr))) = v
}

// get []uint8 value
func GetUint8Arr(ptr uintptr) []uint8 {
	return *((*[]uint8)(unsafe.Pointer(ptr)))
}

// set []uint8 value
func SetUint8Arr(ptr uintptr, v []uint8) {
	*((*[]uint8)(unsafe.Pointer(ptr))) = v
}

// get []uint16 value
func GetUint16Arr(ptr uintptr) []uint16 {
	return *((*[]uint16)(unsafe.Pointer(ptr)))
}

// set []uint16 value
func SetUint16Arr(ptr uintptr, v []uint16) {
	*((*[]uint16)(unsafe.Pointer(ptr))) = v
}

// get []uint32 value
func GetUint32Arr(ptr uintptr) []uint32 {
	return *((*[]uint32)(unsafe.Pointer(ptr)))
}

// set []uint32 value
func SetUint32Arr(ptr uintptr, v []uint32) {
	*((*[]uint32)(unsafe.Pointer(ptr))) = v
}

// get []uint64 value
func GetUint64Arr(ptr uintptr) []uint64 {
	return *((*[]uint64)(unsafe.Pointer(ptr)))
}

// set []uint64 value
func SetUint64Arr(ptr uintptr, v []uint64) {
	*((*[]uint64)(unsafe.Pointer(ptr))) = v
}

// get map[string]string value
func GetMapString(ptr uintptr) map[string]string {
	return *((*map[string]string)(unsafe.Pointer(ptr)))
}

// set map[string]string value
func SetMapString(ptr uintptr, v map[string]string) {
	*((*map[string]string)(unsafe.Pointer(ptr))) = v
}
