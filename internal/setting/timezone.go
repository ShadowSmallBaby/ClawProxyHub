// timezone.go — 调度使用明确的 IANA 时区，内嵌数据保证精简镜像也可用。
package setting

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

const KeyTimezone = "system.timezone"
const DefaultTimezone = "Asia/Shanghai"

func ValidateTimezone(name string) error {
	if name == "" || name == "Local" || strings.TrimSpace(name) != name || len(name) > 128 {
		return fmt.Errorf("timezone must be an IANA name, such as Asia/Shanghai or UTC")
	}
	_, err := time.LoadLocation(name)
	return err
}

func (s *Store) Timezone() string {
	name := s.Get(KeyTimezone, DefaultTimezone)
	if ValidateTimezone(name) != nil {
		return DefaultTimezone
	}
	return name
}

func (s *Store) Location() *time.Location {
	loc, _ := time.LoadLocation(s.Timezone())
	return loc
}
