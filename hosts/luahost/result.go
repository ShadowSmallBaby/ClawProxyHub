// result.go — 脚本返回 table → proto 结果：Manifest / AccountProfile 与通用辅助。
package main

import (
	lua "github.com/yuin/gopher-lua"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// tblField 取子表（缺失返回 nil）。
func tblField(t *lua.LTable, k string) *lua.LTable {
	if v, ok := t.RawGetString(k).(*lua.LTable); ok {
		return v
	}
	return nil
}

// strMapField 取字符串 map 子表（多语言/quota 等；值统一按 string 取）。
func strMapField(t *lua.LTable, k string) map[string]string {
	sub := tblField(t, k)
	if sub == nil {
		return nil
	}
	m := map[string]string{}
	sub.ForEach(func(key, v lua.LValue) { m[key.String()] = v.String() })
	return m
}

// strList 取字符串数组子表。
func strList(t *lua.LTable, k string) []string {
	sub := tblField(t, k)
	if sub == nil {
		return nil
	}
	var out []string
	sub.ForEach(func(_, v lua.LValue) { out = append(out, v.String()) })
	return out
}

// errorFromField 取 {error={code,message,retryable}} → pb.Error（无则 nil）。
func errorFromField(t *lua.LTable) *pb.Error {
	e := tblField(t, "error")
	if e == nil {
		return nil
	}
	return &pb.Error{Code: int32(numField(e, "code")), Message: strField(e, "message"), Retryable: boolField(e, "retryable")}
}

// manifestFromTable 把 plugin.handshake 返回的 manifest 子表转 pb.Manifest。
func manifestFromTable(mt *lua.LTable) *pb.Manifest {
	m := &pb.Manifest{
		Name: strField(mt, "name"), Version: strField(mt, "version"), Author: strField(mt, "author"),
		Icon: strField(mt, "icon"), MinCoreVersion: strField(mt, "min_core_version"),
		ProtocolVersion: int32(numField(mt, "protocol_version")),
		SettingsSchema:  strField(mt, "settings_schema"), InstanceSchema: strField(mt, "instance_schema"),
		Label: strMapField(mt, "label"), Capabilities: strList(mt, "capabilities"), Endpoints: strList(mt, "endpoints"),
	}
	if ams := tblField(mt, "auth_methods"); ams != nil {
		ams.ForEach(func(_, v lua.LValue) {
			if am, ok := v.(*lua.LTable); ok {
				m.AuthMethods = append(m.AuthMethods, authMethodFromTable(am))
			}
		})
	}
	return m
}

func authMethodFromTable(am *lua.LTable) *pb.AuthMethod {
	out := &pb.AuthMethod{
		Id: strField(am, "id"), Label: strMapField(am, "label"),
		Callback: strField(am, "callback"), Capabilities: strList(am, "capabilities"),
	}
	if fs := tblField(am, "fields"); fs != nil {
		fs.ForEach(func(_, v lua.LValue) {
			if f, ok := v.(*lua.LTable); ok {
				out.Fields = append(out.Fields, authFieldFromTable(f))
			}
		})
	}
	return out
}

func authFieldFromTable(f *lua.LTable) *pb.AuthField {
	return &pb.AuthField{
		Name: strField(f, "name"), Label: strMapField(f, "label"), Type: strField(f, "type"),
		Required: boolField(f, "required"), Placeholder: strField(f, "placeholder"),
	}
}

// profileFromTable {display_name,healthy,quota,credits_json} → AccountProfile。
func profileFromTable(t *lua.LTable) *pb.AccountProfile {
	if t == nil {
		return &pb.AccountProfile{}
	}
	return &pb.AccountProfile{
		DisplayName: strField(t, "display_name"),
		Healthy:     boolField(t, "healthy"),
		Quota:       strMapField(t, "quota"),
		CreditsJson: strField(t, "credits_json"),
	}
}
