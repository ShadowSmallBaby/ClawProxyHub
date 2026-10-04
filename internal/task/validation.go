package task

import (
	"encoding/json"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/model"
	"time"
)

func ValidateRule(rule *model.TaskRule) error {
	if rule.PluginID <= 0 || rule.CapabilityID == "" {
		return fmt.Errorf("plugin and capability required")
	}
	switch rule.TargetScope {
	case "all", "rotate", "global":
	case "account_ids":
		var ids []int64
		if json.Unmarshal([]byte(rule.TargetJSON), &ids) != nil || len(ids) == 0 {
			return fmt.Errorf("account_ids must be nonempty")
		}
		for _, id := range ids {
			if id <= 0 {
				return fmt.Errorf("invalid account id")
			}
		}
	default:
		return fmt.Errorf("invalid target_scope")
	}
	switch rule.TriggerType {
	case "interval":
		d, err := time.ParseDuration(rule.TriggerValue)
		if err != nil || d <= 0 {
			return fmt.Errorf("interval must be positive")
		}
	case "daily":
		if _, err := time.Parse("15:04", rule.TriggerValue); err != nil {
			return fmt.Errorf("daily time must be HH:MM")
		}
	case "once":
		if _, err := time.Parse(time.RFC3339, rule.TriggerValue); err != nil {
			return err
		}
	case "cron":
		if nextCron(rule.TriggerValue, time.Now().UTC()).IsZero() {
			return fmt.Errorf("invalid cron")
		}
	default:
		return fmt.Errorf("invalid trigger_type")
	}
	return nil
}
