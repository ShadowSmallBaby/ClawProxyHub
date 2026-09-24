// account.go — 账号类 RPC 结果 table → proto：LoginResult / RefreshResult。
package main

import (
	lua "github.com/yuin/gopher-lua"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

// loginResultFromTable {blob, profile, next, error} → LoginResult。
func loginResultFromTable(t *lua.LTable) *pb.LoginResult {
	out := &pb.LoginResult{Error: errorFromField(t)}
	if b := strField(t, "blob"); b != "" {
		out.Blob = []byte(b)
	}
	out.Profile = profileFromTable(tblField(t, "profile"))
	if nx := tblField(t, "next"); nx != nil {
		out.Next = loginNextFromTable(nx)
	}
	return out
}

func loginNextFromTable(nx *lua.LTable) *pb.LoginNextStep {
	step := &pb.LoginNextStep{
		Action: strField(nx, "action"), Url: strField(nx, "url"),
		Prompt: strMapField(nx, "prompt"), Wait: boolField(nx, "wait"),
	}
	if s := strField(nx, "state"); s != "" {
		step.State = []byte(s)
	}
	if fs := tblField(nx, "fields"); fs != nil {
		fs.ForEach(func(_, v lua.LValue) {
			if f, ok := v.(*lua.LTable); ok {
				step.Fields = append(step.Fields, authFieldFromTable(f))
			}
		})
	}
	return step
}

// refreshResultFromTable {blob, profile, error} → RefreshResult。
func refreshResultFromTable(t *lua.LTable) *pb.RefreshResult {
	out := &pb.RefreshResult{Error: errorFromField(t)}
	if b := strField(t, "blob"); b != "" {
		out.Blob = []byte(b)
	}
	out.Profile = profileFromTable(tblField(t, "profile"))
	return out
}
