package task

import (
	"testing"
	"time"
)

func TestNextCronDaily(t *testing.T) {
	base := time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local)
	next := nextCron("0 9 * * *", base)
	if next.Hour() != 9 || next.Day() != 16 {
		t.Errorf("daily 9am next = %v, want next day 09:00", next)
	}
}

func TestNextCronIntervalStep(t *testing.T) {
	base := time.Date(2026, 9, 15, 10, 2, 0, 0, time.Local)
	next := nextCron("*/15 * * * *", base)
	if next.Minute() != 15 {
		t.Errorf("*/15 next = %v, want 10:15", next)
	}
}

func TestNextCronRangeAndList(t *testing.T) {
	base := time.Date(2026, 9, 15, 8, 0, 0, 0, time.Local) // 周二
	next := nextCron("30 9-11 * * 1,3,5", base)            // 周一三五 9:30-11:30
	// 9/15 周二不匹配 → 下一个是 9/16 周三 09:30
	if next.Weekday() != time.Wednesday || next.Hour() != 9 || next.Minute() != 30 {
		t.Errorf("range/list next = %v, want Wed 09:30", next)
	}
}

func TestNextCronAlias(t *testing.T) {
	base := time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local)
	next := nextCron("@hourly", base)
	if next.Hour() != 11 || next.Minute() != 0 {
		t.Errorf("@hourly next = %v, want 11:00", next)
	}
}

func TestNextCronInvalid(t *testing.T) {
	if !nextCron("bad expr", time.Now()).IsZero() {
		t.Errorf("invalid expr should be zero")
	}
	if !nextCron("99 * * * *", time.Now()).IsZero() {
		t.Errorf("out of range should be zero")
	}
}
