// Package textutil 提供按字节上限保留完整 UTF-8 字符的截断。
package textutil

import "unicode/utf8"

func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
