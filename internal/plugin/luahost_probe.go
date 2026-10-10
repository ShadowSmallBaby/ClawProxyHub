package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ShadowSmallBaby/ClawProxyHub/sdk"
	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func ProbeLuaExecutable(ctx context.Context, executable string) error {
	m := &Manager{luaProvider: func() (string, error) { return executable, nil }}
	return ProbeLuaRuntime(ctx, defaultRuntime(m))
}

// ProbeLuaRuntime 使用临时脚本检查协议握手与 Lua VM，不注册插件或开放宿主能力。
func ProbeLuaRuntime(ctx context.Context, candidate Runtime) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "cph-runtime-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	name := filepath.Base(dir)
	manifest, _ := json.Marshal(map[string]any{"name": name, "version": "0.0.0", "runtime": "lua", "protocol_version": sdk.ProtocolVersion})
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "main.lua"), []byte(`return {models=function() return {models={{id="cph-runtime-probe"}}} end}`), 0600); err != nil {
		return err
	}
	session, err := candidate.Start(ctx, dir, &pb.UnimplementedClawHostServer{})
	if err != nil {
		return fmt.Errorf("Lua Host startup check: %w", err)
	}
	if session == nil {
		return fmt.Errorf("Lua Host startup returned no session")
	}
	defer session.Close()
	client, protocol := session.Client(), session.ProtocolVersion()
	if client == nil || protocol < sdk.MinProtocolVersion || protocol > sdk.ProtocolVersion {
		return fmt.Errorf("Lua Host negotiated an unsupported protocol")
	}
	response, err := client.Handshake(ctx, &pb.HandshakeRequest{CoreVersion: CoreVersion, ProtocolVersion: protocol})
	if err != nil {
		return fmt.Errorf("Lua Host handshake: %w", err)
	}
	if response.GetError().GetCode() != 0 || response.GetManifest().GetName() != name || response.GetManifest().GetProtocolVersion() != protocol {
		return fmt.Errorf("Lua Host handshake identity or protocol mismatch")
	}
	models, err := client.ListModels(ctx, &pb.CredentialBlob{})
	if err != nil {
		return fmt.Errorf("Lua Host VM check: %w", err)
	}
	if models.GetError().GetCode() != 0 || len(models.GetModels()) != 1 || models.GetModels()[0].GetId() != "cph-runtime-probe" {
		return fmt.Errorf("Lua Host failed to execute the validation script")
	}
	return nil
}
