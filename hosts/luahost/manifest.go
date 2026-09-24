// manifest.go — 读 plugins-lua/<name>/manifest.json，生成握手用的 pb.Manifest。
// 脚本作者只写 manifest.json（含 runtime/entry/capabilities/auth_methods），无需在 Lua 里写 manifest 函数。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

type authField struct {
	Name        string            `json:"name"`
	Label       map[string]string `json:"label"`
	Type        string            `json:"type"`
	Required    bool              `json:"required"`
	Placeholder string            `json:"placeholder"`
}

type authMethod struct {
	Id           string            `json:"id"`
	Label        map[string]string `json:"label"`
	Fields       []authField       `json:"fields"`
	Capabilities []string          `json:"capabilities"`
	Callback     string            `json:"callback"`
}

// scriptManifest 脚本插件清单（字段与 cph.proto Manifest + runtime/entry 对齐）。
type scriptManifest struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Author          string            `json:"author"`
	Runtime         string            `json:"runtime"`
	Entry           string            `json:"entry"`
	Label           map[string]string `json:"label"`
	Icon            string            `json:"icon"`
	ProtocolVersion int32             `json:"protocol_version"`
	MinCoreVersion  string            `json:"min_core_version"`
	SettingsSchema  json.RawMessage   `json:"settings_schema"`
	InstanceSchema  json.RawMessage   `json:"instance_schema"`
	Capabilities    []string          `json:"capabilities"`
	AuthMethods     []authMethod      `json:"auth_methods"`
	Endpoints       []string          `json:"endpoints"`
}

func loadManifest(dir string) (*scriptManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var mf scriptManifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

// toPB 映射为握手 Manifest；settings_schema/instance_schema 以对象形式原样透传为字符串。
func (mf *scriptManifest) toPB() *pb.Manifest {
	m := &pb.Manifest{
		Name:            mf.Name,
		Version:         mf.Version,
		Author:          mf.Author,
		Label:           mf.Label,
		Icon:            mf.Icon,
		ProtocolVersion: mf.ProtocolVersion,
		MinCoreVersion:  mf.MinCoreVersion,
		SettingsSchema:  string(mf.SettingsSchema),
		InstanceSchema:  string(mf.InstanceSchema),
		Capabilities:    mf.Capabilities,
		Endpoints:       mf.Endpoints,
	}
	for _, am := range mf.AuthMethods {
		pam := &pb.AuthMethod{Id: am.Id, Label: am.Label, Capabilities: am.Capabilities, Callback: am.Callback}
		for _, f := range am.Fields {
			pam.Fields = append(pam.Fields, &pb.AuthField{
				Name: f.Name, Label: f.Label, Type: f.Type, Required: f.Required, Placeholder: f.Placeholder,
			})
		}
		m.AuthMethods = append(m.AuthMethods, pam)
	}
	return m
}
