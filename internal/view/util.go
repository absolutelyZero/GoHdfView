package view

import "strconv"

// itoa 把 int 转成十进制字符串。
func itoa(v int) string { return strconv.Itoa(v) }

// itoa64 把 uint64 转成十进制字符串。
func itoa64(v uint64) string { return strconv.FormatUint(v, 10) }

// itoa64Hex 把 uint64 转成小写十六进制字符串。
func itoa64Hex(v uint64) string { return strconv.FormatUint(v, 16) }
