// cron.go — 5 字段 cron（分 时 日 月 周）的简化解析与下次触发计算。
// 支持：* 数字 */n a-b a-b/n（组合逗号分隔）。@daily/@hourly 别名。
package task

import "time"

// nextCron 计算 expr 的下次触发时刻（从 from 起一分钟粒度向后找，上限 366 天）。
func nextCron(expr string, from time.Time) time.Time {
	fields, ok := parseCronExpr(expr)
	if !ok {
		return time.Time{}
	}
	// 从下一分钟开始逐分钟前进（对自用调度足够，避免完整 DOW/DOM 语义争议：
	// 两者都受限时按"或"语义，与常见 cron 实现一致）
	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if cronMatch(fields, t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// cronField 一个字段允许的取值集合。
type cronField struct {
	min, max int
	set      map[int]bool
}

func parseCronExpr(expr string) ([5]cronField, bool) {
	switch expr {
	case "@hourly":
		expr = "0 * * * *"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@weekly":
		expr = "0 0 * * 0"
	}
	var parts [5]cronField
	fields := []struct {
		raw      string
		min, max int
	}{
		{"", 0, 59}, // minute
		{"", 0, 23}, // hour
		{"", 1, 31}, // day of month
		{"", 1, 12}, // month
		{"", 0, 6},  // day of week
	}
	toks := splitFields(expr)
	if len(toks) != 5 {
		return parts, false
	}
	for i := range toks {
		fields[i].raw = toks[i]
		set, ok := parseCronField(toks[i], fields[i].min, fields[i].max)
		if !ok {
			return parts, false
		}
		parts[i] = cronField{min: fields[i].min, max: fields[i].max, set: set}
	}
	return parts, true
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// parseCronField 单字段：*/n、*、a、a-b、a-b/n，逗号组合。
func parseCronField(s string, min, max int) (map[int]bool, bool) {
	set := map[int]bool{}
	for _, part := range splitComma(s) {
		step := 1
		if i := indexOfStr(part, "/"); i >= 0 {
			if !parseInt(part[i+1:], &step) || step <= 0 {
				return nil, false
			}
			part = part[:i]
		}
		lo, hi := min, max
		if part != "*" {
			if j := indexOfStr(part, "-"); j >= 0 {
				if !parseInt(part[:j], &lo) || !parseInt(part[j+1:], &hi) {
					return nil, false
				}
			} else if !parseInt(part, &lo) {
				return nil, false
			} else {
				hi = lo
			}
		}
		if lo < min || hi > max || lo > hi {
			return nil, false
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	return set, len(set) > 0
}

func cronMatch(fields [5]cronField, t time.Time) bool {
	if !fields[0].set[t.Minute()] || !fields[1].set[t.Hour()] {
		return false
	}
	dom, month, dow := fields[2].set[t.Day()], fields[3].set[int(t.Month())], fields[4].set[int(t.Weekday())]
	// 常见语义：dom 与 dow 均受限时取并集
	domRestricted := len(fields[2].set) < (fields[2].max - fields[2].min + 1)
	dowRestricted := len(fields[4].set) < (fields[4].max - fields[4].min + 1)
	if domRestricted && dowRestricted {
		return month && (dom || dow)
	}
	return month && dom && dow
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func parseInt(s string, out *int) bool {
	if s == "" {
		return false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
	}
	*out = n
	return true
}
