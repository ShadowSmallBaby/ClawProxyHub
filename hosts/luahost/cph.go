// cph.go — 宿主能力 cph.*：脚本与外界的唯一通道。json / hash / time / random / log 在此，
// http 见 http.go。全部经 registerCPH 注入为 Go 闭包。
package main

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
)

// registerCPH 把 cph 命名空间注入全局环境。dir 预留给未来 require("lib.*")。
func registerCPH(L *lua.LState, host *sdk.Host, dir string) {
	cph := L.NewTable()
	L.SetField(cph, "http", newHTTPModule(L))
	L.SetField(cph, "json", tableOf(L, map[string]lua.LGFunction{
		"encode": cphJSONEncode,
		"decode": cphJSONDecode,
	}))
	L.SetField(cph, "hash", tableOf(L, map[string]lua.LGFunction{
		"md5":              cphMD5,
		"sha256":           cphSHA256,
		"hmac_sha256":      cphHMACSHA256,
		"base64url_encode": cphB64URLEncode,
		"base64url_decode": cphB64URLDecode,
	}))
	L.SetField(cph, "time", tableOf(L, map[string]lua.LGFunction{
		"now":   cphTimeNow,
		"sleep": cphTimeSleep,
	}))
	L.SetField(cph, "openai", newOpenAIModule(L))
	L.SetField(cph, "random", tableOf(L, map[string]lua.LGFunction{
		"uuid": cphUUID,
		"hex":  cphRandHex,
	}))
	L.SetField(cph, "log", newLogModule(L, host))
	L.SetGlobal("cph", cph)
}

// tableOf 建一个函数名 → Go 闭包的 Lua 子表。
func tableOf(L *lua.LState, fns map[string]lua.LGFunction) *lua.LTable {
	t := L.NewTable()
	for name, fn := range fns {
		L.SetField(t, name, L.NewFunction(fn))
	}
	return t
}

func cphJSONEncode(L *lua.LState) int {
	b, err := json.Marshal(luaToGo(L.CheckAny(1)))
	if err != nil {
		L.RaiseError("json.encode: %v", err)
	}
	L.Push(lua.LString(string(b)))
	return 1
}

func cphJSONDecode(L *lua.LState) int {
	var v interface{}
	if err := json.Unmarshal([]byte(L.CheckString(1)), &v); err != nil {
		L.RaiseError("json.decode: %v", err)
	}
	L.Push(goToLua(L, v))
	return 1
}

func cphMD5(L *lua.LState) int {
	sum := md5.Sum([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(sum[:])))
	return 1
}

func cphSHA256(L *lua.LState) int {
	sum := sha256.Sum256([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(sum[:])))
	return 1
}

func cphHMACSHA256(L *lua.LState) int {
	mac := hmac.New(sha256.New, []byte(L.CheckString(2))) // (data, key)
	mac.Write([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(mac.Sum(nil))))
	return 1
}

func cphB64URLEncode(L *lua.LState) int {
	L.Push(lua.LString(base64.RawURLEncoding.EncodeToString([]byte(L.CheckString(1)))))
	return 1
}

func cphB64URLDecode(L *lua.LState) int {
	// 兼容带 padding 与标准字母表：先按 RawURL 试，失败回退 StdEncoding。
	s := L.CheckString(1)
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		L.Push(lua.LString(string(b)))
		return 1
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		L.RaiseError("base64url_decode: %v", err)
	}
	L.Push(lua.LString(string(b)))
	return 1
}

func cphTimeNow(L *lua.LState) int {
	L.Push(lua.LNumber(nowMillis()))
	return 1
}

func cphUUID(L *lua.LState) int {
	L.Push(lua.LString(uuid.NewString()))
	return 1
}

func cphRandHex(L *lua.LState) int {
	n := L.CheckInt(1)
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	L.Push(lua.LString(hex.EncodeToString(buf)))
	return 1
}

// newLogModule 结构化日志：cph.log.<level>(msg, fields?) → 宿主 runlog。
func newLogModule(L *lua.LState, host *sdk.Host) *lua.LTable {
	mk := func(level string) lua.LGFunction {
		return func(L *lua.LState) int {
			msg := L.CheckString(1)
			var fields map[string]string
			if t, ok := L.Get(2).(*lua.LTable); ok {
				fields = map[string]string{}
				t.ForEach(func(k, v lua.LValue) { fields[k.String()] = v.String() })
			}
			if host != nil {
				host.LogFields(level, msg, fields)
			}
			return 0
		}
	}
	return tableOf(L, map[string]lua.LGFunction{
		"debug": mk("debug"), "info": mk("info"), "warn": mk("warn"), "error": mk("error"),
	})
}
