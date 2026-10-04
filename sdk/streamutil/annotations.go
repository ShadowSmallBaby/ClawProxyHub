package streamutil

import "encoding/json"

// AnnotationSet 按来源内容块去重增量与最终快照中的注解。
type AnnotationSet map[string]map[string]bool

func (s *AnnotationSet) Add(block string, values []json.RawMessage) string {
	if *s == nil {
		*s = AnnotationSet{}
	}
	if (*s)[block] == nil {
		(*s)[block] = map[string]bool{}
	}
	var added []json.RawMessage
	for _, raw := range values {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil || obj == nil {
			continue
		}
		canonical, _ := json.Marshal(obj)
		key := string(canonical)
		if (*s)[block][key] {
			continue
		}
		(*s)[block][key] = true
		added = append(added, canonical)
	}
	if len(added) == 0 {
		return ""
	}
	raw, _ := json.Marshal(added)
	return string(raw)
}
