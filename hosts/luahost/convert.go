// convert.go — 通用 Go/Lua JSON 值互转 + 取字段辅助 + 出向 message 转换。
package main

import (
	lua "github.com/yuin/gopher-lua"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// goToLua 把 JSON 解码出的 Go 值转成 Lua 值。
func goToLua(L *lua.LState, v interface{}) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []interface{}:
		t := L.NewTable()
		for _, e := range x {
			t.Append(goToLua(L, e))
		}
		return t
	case map[string]interface{}:
		t := L.NewTable()
		for k, e := range x {
			t.RawSetString(k, goToLua(L, e))
		}
		return t
	default:
		return lua.LNil
	}
}

// luaToGo 把 Lua 值转成可 JSON 编码的 Go 值：连续 1..n 整数键视为数组，否则对象。
func luaToGo(v lua.LValue) interface{} {
	switch x := v.(type) {
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		return float64(x)
	case lua.LString:
		return string(x)
	case *lua.LTable:
		if n := x.Len(); n > 0 {
			arr := make([]interface{}, 0, n)
			for i := 1; i <= n; i++ {
				arr = append(arr, luaToGo(x.RawGetInt(i)))
			}
			return arr
		}
		obj := map[string]interface{}{}
		x.ForEach(func(k, val lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				obj[string(ks)] = luaToGo(val)
			}
		})
		return obj
	default:
		return nil
	}
}

// strField / numField / boolField 从 table 取带类型字段（缺失返回零值）。
func strField(t *lua.LTable, k string) string {
	if s, ok := t.RawGetString(k).(lua.LString); ok {
		return string(s)
	}
	return ""
}

func numField(t *lua.LTable, k string) float64 {
	if n, ok := t.RawGetString(k).(lua.LNumber); ok {
		return float64(n)
	}
	return 0
}

func boolField(t *lua.LTable, k string) bool {
	return lua.LVAsBool(t.RawGetString(k))
}

// usageFromField 取 table 里名为 k 的 usage 子表转 Usage（Anthropic 语义五字段）。
func usageFromField(t *lua.LTable, k string) *pb.Usage {
	u, ok := t.RawGetString(k).(*lua.LTable)
	if !ok {
		return nil
	}
	return &pb.Usage{
		InputTokens:         int64(numField(u, "input_tokens")),
		OutputTokens:        int64(numField(u, "output_tokens")),
		CachedTokens:        int64(numField(u, "cached_tokens")),
		CacheCreationTokens: int64(numField(u, "cache_creation_tokens")),
		ReasoningTokens:     int64(numField(u, "reasoning_tokens")),
	}
}
