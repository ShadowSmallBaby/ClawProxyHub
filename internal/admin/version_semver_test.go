package admin

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.3", "1.0.5", -1}, // 远端旧于本机 → 不提示更新
		{"1.0.5", "1.0.3", 1},  // 远端新于本机 → 提示更新
		{"1.0.5", "1.0.5", 0},  // 相等 → 不提示
		{"v1.2.0", "1.1.9", 1}, // v 前缀 + 段间进位
		{"1.0.10", "1.0.9", 1}, // 数值比较而非字典序
		{"1.1", "1.1.0", 0},    // 缺省段补零
		{"2.0.0-beta", "2.0.0", 0},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
