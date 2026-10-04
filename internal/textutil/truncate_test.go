package textutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	for _, s := range []string{"你好世界", "a🙂b中文", "ASCII"} {
		for n := 0; n <= len(s)+1; n++ {
			got := Truncate(s, n)
			if !utf8.ValidString(got) || len(got) > n {
				t.Fatalf("%q, %d: %q", s, n, got)
			}
		}
	}
}
