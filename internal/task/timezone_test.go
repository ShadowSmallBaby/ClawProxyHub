package task

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/database"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/setting"
)

func timezoneEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	store := setting.New(db)
	if err := store.Set(setting.KeyTaskDailyJitter, "0"); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(db, dir, nil, nil, store)
	t.Cleanup(e.Stop)
	return e
}

func TestScheduleTimezone(t *testing.T) {
	e := timezoneEngine(t)
	if e.settings.Timezone() != "Asia/Shanghai" {
		t.Fatal("default timezone is not Beijing time")
	}
	cases := []struct{ name, zone, kind, value, from, want string }{
		{"beijing_daily", "Asia/Shanghai", "daily", "09:00", "2026-10-05T00:30:00Z", "2026-10-05T01:00:00Z"},
		{"beijing_next_day", "Asia/Shanghai", "daily", "09:00", "2026-10-05T01:00:00Z", "2026-10-06T01:00:00Z"},
		{"beijing_cron", "Asia/Shanghai", "cron", "0 9 * * *", "2026-10-05T00:30:00Z", "2026-10-05T01:00:00Z"},
		{"utc", "UTC", "daily", "09:00", "2026-10-05T00:30:00Z", "2026-10-05T09:00:00Z"},
		{"half_hour", "Asia/Kolkata", "daily", "09:00", "2026-10-05T00:30:00Z", "2026-10-05T03:30:00Z"},
		{"new_york_summer", "America/New_York", "daily", "09:00", "2026-07-01T00:00:00Z", "2026-07-01T13:00:00Z"},
		{"new_york_winter", "America/New_York", "cron", "0 9 * * *", "2026-12-01T00:00:00Z", "2026-12-01T14:00:00Z"},
		{"dst_missing_hour", "America/New_York", "daily", "02:30", "2026-03-08T06:59:00Z", "2026-03-09T06:30:00Z"},
		{"dst_repeated_hour", "America/New_York", "daily", "01:30", "2026-11-01T05:45:00Z", "2026-11-01T06:30:00Z"},
		{"interval_utc", "UTC", "interval", "1h", "2026-10-05T08:30:00+08:00", "2026-10-05T01:30:00Z"},
		{"interval_shanghai", "Asia/Shanghai", "interval", "1h", "2026-10-05T08:30:00+08:00", "2026-10-05T01:30:00Z"},
		{"once_offset", "America/New_York", "once", "2026-10-05T09:00:00+08:00", "2026-10-05T00:30:00Z", "2026-10-05T01:00:00Z"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := e.SaveSettings(map[string]string{setting.KeyTimezone: tt.zone}); err != nil {
				t.Fatal(err)
			}
			from, _ := time.Parse(time.RFC3339, tt.from)
			next := e.computeNext(&model.TaskRule{TriggerType: tt.kind, TriggerValue: tt.value}, from)
			if next == nil || next.Format(time.RFC3339) != tt.want || next.Location() != time.UTC {
				t.Fatalf("next=%v, want UTC %s", next, tt.want)
			}
		})
	}
}

func TestTimezoneChangeReschedulesCalendarOnly(t *testing.T) {
	e := timezoneEngine(t)
	p := model.Plugin{Name: "timezone-test", ManifestJSON: "{}"}
	if err := e.db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	rules := []model.TaskRule{
		{TriggerType: "daily", TriggerValue: "09:00"},
		{TriggerType: "cron", TriggerValue: "0 9 * * *"},
		{TriggerType: "interval", TriggerValue: "1h"},
		{TriggerType: "once", TriggerValue: future.Format(time.RFC3339)},
	}
	for i := range rules {
		rules[i].PluginID, rules[i].CapabilityID, rules[i].Enabled = p.ID, "test", true
		rules[i].NextRunAt = &future
		if err := e.db.Create(&rules[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := e.SaveSettings(map[string]string{setting.KeyTimezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	for i := range rules {
		var got model.TaskRule
		if err := e.db.First(&got, rules[i].ID).Error; err != nil {
			t.Fatal(err)
		}
		if i < 2 && got.NextRunAt != nil {
			t.Fatalf("calendar rule kept old deadline: %v", got.NextRunAt)
		}
		if i >= 2 && (got.NextRunAt == nil || !got.NextRunAt.Equal(future)) {
			t.Fatalf("fixed instant changed: %+v", got)
		}
	}
	e.doTick(context.Background())
	for _, rule := range rules[:2] {
		var got model.TaskRule
		if err := e.db.First(&got, rule.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.NextRunAt == nil || got.NextRunAt.UTC().Hour() != 9 || !got.NextRunAt.After(time.Now()) {
			t.Fatalf("rule not rescheduled: %+v", got)
		}
	}
	if err := e.SaveSettings(map[string]string{setting.KeyTimezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	var got model.TaskRule
	e.db.First(&got, rules[0].ID)
	if got.NextRunAt == nil {
		t.Fatal("saving unchanged timezone reset schedule")
	}
	if err := e.db.Exec(`CREATE TRIGGER fail_timezone BEFORE UPDATE ON settings WHEN NEW.key = 'system.timezone' BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.SaveSettings(map[string]string{setting.KeyTimezone: "Asia/Tokyo"}); err == nil {
		t.Fatal("expected persistence failure")
	}
	e.db.First(&got, rules[0].ID)
	if e.settings.Timezone() != "UTC" || got.NextRunAt == nil {
		t.Fatal("failed save changed timezone or deadlines")
	}
}
