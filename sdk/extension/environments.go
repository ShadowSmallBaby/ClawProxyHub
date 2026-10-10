package extension

import "fmt"

// ValidateEnvironments 校验界面范围；Web 断点不改变核心上的安装和启用状态。
func ValidateEnvironments(values []string) error {
	if values != nil && len(values) == 0 {
		return fmt.Errorf("extension environments must not be empty")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if (value != "app" && value != "web-desktop" && value != "web-mobile") || seen[value] {
			return fmt.Errorf("invalid or duplicate extension environment %q", value)
		}
		seen[value] = true
	}
	return nil
}
