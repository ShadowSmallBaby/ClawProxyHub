// host.go — 将当前插件的设置与持久化状态回调开放给 Lua。
package luahost

import (
	"encoding/json"
	"math"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	lua "github.com/yuin/gopher-lua"
)

// newSettingsModule 从清单绑定插件名，脚本只能选择该插件下的实例。
func newSettingsModule(L *lua.LState, host *sdk.Host, dir string) *lua.LTable {
	manifest, manifestErr := loadManifest(dir)
	read := func(instance bool) lua.LGFunction {
		return func(L *lua.LState) int {
			var instanceID int64
			if instance {
				value := float64(L.CheckNumber(1))
				if value < 0 || value > 1<<53-1 || math.Trunc(value) != value {
					L.ArgError(1, "instance_id must be a non-negative safe integer")
					return 0
				}
				instanceID = int64(value)
			}
			if manifestErr != nil || manifest.Name == "" {
				L.RaiseError("settings: plugin identity unavailable in manifest.json")
				return 0
			}
			raw, err := host.InstanceSettingsContext(luaContext(L), manifest.Name, instanceID)
			if err != nil {
				L.RaiseError("settings: %v", err)
				return 0
			}
			var values map[string]interface{}
			if err := json.Unmarshal(raw, &values); err != nil || values == nil {
				L.RaiseError("settings: host must return a JSON object")
				return 0
			}
			L.Push(goToLua(L, values))
			return 1
		}
	}
	return tableOf(L, map[string]lua.LGFunction{"get": read(false), "instance": read(true)})
}

// newStoreModule 原样存取字符串，缺失与回调失败分别用 found 和 Lua 异常表示。
func newStoreModule(L *lua.LState, host *sdk.Host) *lua.LTable {
	return tableOf(L, map[string]lua.LGFunction{
		"get": func(L *lua.LState) int {
			value, found, err := host.StoreGetContext(luaContext(L), L.CheckString(1))
			if err != nil {
				L.RaiseError("store.get: %v", err)
				return 0
			}
			if found {
				L.Push(lua.LString(string(value)))
			} else {
				L.Push(lua.LNil)
			}
			L.Push(lua.LBool(found))
			return 2
		},
		"put": func(L *lua.LState) int {
			key, value := L.CheckString(1), L.CheckString(2)
			if err := host.StorePutContext(luaContext(L), key, []byte(value)); err != nil {
				L.RaiseError("store.put: %v", err)
				return 0
			}
			L.Push(lua.LTrue)
			return 1
		},
	})
}
