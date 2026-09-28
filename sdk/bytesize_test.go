package sdk

import "testing"

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"67108864", 67108864, true}, // 纯字节兼容
		{"64mb", 64 << 20, true},
		{"64MB", 64 << 20, true},
		{"64m", 64 << 20, true},
		{"4096kb", 4096 << 10, true},
		{"64 MB", 64 << 20, true}, // 带空格
		{"1gb", 1 << 30, true},
		{"512b", 512, true},
		{"", 0, false},
		{"abc", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
	}
	for _, c := range cases {
		got, ok := parseByteSize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseByteSize(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestGRPCMaxMsgSize(t *testing.T) {
	t.Setenv(EnvGRPCMaxMsgSize, "")
	if got := GRPCMaxMsgSize(); got != DefaultGRPCMaxMsgSize {
		t.Errorf("empty env = %d, want default %d", got, DefaultGRPCMaxMsgSize)
	}
	t.Setenv(EnvGRPCMaxMsgSize, "128mb")
	if got := GRPCMaxMsgSize(); got != 128<<20 {
		t.Errorf("128mb = %d, want %d", got, 128<<20)
	}
	t.Setenv(EnvGRPCMaxMsgSize, "1kb") // 低于 4MB 下限回退
	if got := GRPCMaxMsgSize(); got != DefaultGRPCMaxMsgSize {
		t.Errorf("below-min = %d, want default %d", got, DefaultGRPCMaxMsgSize)
	}
	t.Setenv(EnvGRPCMaxMsgSize, "garbage")
	if got := GRPCMaxMsgSize(); got != DefaultGRPCMaxMsgSize {
		t.Errorf("garbage = %d, want default %d", got, DefaultGRPCMaxMsgSize)
	}
}
