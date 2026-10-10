package luahost

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type luaCallbackHost struct {
	pb.UnimplementedClawHostServer
	mu       sync.Mutex
	settings map[int64]string
	store    map[string][]byte
	failure  error
	block    string
	started  chan struct{}
	canceled chan struct{}
}

func (h *luaCallbackHost) before(ctx context.Context, operation string) error {
	if h.block == operation {
		close(h.started)
		<-ctx.Done()
		close(h.canceled)
		return status.FromContextError(ctx.Err()).Err()
	}
	return h.failure
}

func (h *luaCallbackHost) GetSettings(ctx context.Context, req *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
	if err := h.before(ctx, "settings"); err != nil {
		return nil, err
	}
	if req.Plugin != "callbacks" {
		return nil, status.Error(codes.PermissionDenied, "wrong plugin identity")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	raw, ok := h.settings[req.InstanceId]
	if !ok {
		return nil, status.Error(codes.NotFound, "unknown instance")
	}
	return &pb.GetSettingsResponse{Values: []byte(raw)}, nil
}

func (h *luaCallbackHost) StoreGet(ctx context.Context, req *pb.StoreGetRequest) (*pb.StoreGetResponse, error) {
	if err := h.before(ctx, "get"); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	value, found := h.store[req.Key]
	return &pb.StoreGetResponse{Value: append([]byte(nil), value...), Found: found}, nil
}

func (h *luaCallbackHost) StorePut(ctx context.Context, req *pb.StorePutRequest) (*pb.Empty, error) {
	if err := h.before(ctx, "put"); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.store == nil {
		h.store = make(map[string][]byte)
	}
	h.store[req.Key] = append([]byte(nil), req.Value...)
	return &pb.Empty{}, nil
}

func callbackFixture(t *testing.T, script string) string {
	t.Helper()
	return writeFixture(t, `{"name":"callbacks","version":"0.1.0","author":"test","runtime":"lua"}`, script)
}

// connectLuaCallbacks 经双向 gRPC 驱动 Lua 与宿主回调，覆盖真实的取消传播。
func connectLuaCallbacks(t *testing.T, dir string, callbacks pb.ClawHostServer) (pb.ClawPluginClient, func()) {
	t.Helper()
	a, b := net.Pipe()
	c, d := net.Pipe()
	host, err := transport.ConnectHost(c, a, callbacks)
	if err != nil {
		b.Close()
		d.Close()
		t.Fatal(err)
	}
	impl := New(dir)
	plugin, err := transport.ServePlugin(b, d, impl)
	if err != nil {
		host.Close()
		t.Fatal(err)
	}
	closeSession := func() {
		host.Close()
		plugin.Close()
		_ = impl.Close()
	}
	t.Cleanup(closeSession)
	return pb.NewClawPluginClient(host.Client), closeSession
}

func TestLuaSettingsAvailableAcrossRPCs(t *testing.T) {
	dir := callbackFixture(t, `local M = {}
function M.login(req)
  local settings = cph.settings.instance(req.instance_id)
  settings.global = cph.settings.get().global
  return {blob=cph.json.encode(settings)}
end
function M.refresh(cred)
  return {blob=cph.json.encode(cph.settings.instance(cred.instance_id))}
end
function M.profile(cred)
  return {display_name=cph.settings.instance(cred.instance_id).site}
end
function M.models(cred)
  return {models={{id=cph.settings.instance(cred.instance_id).site}}}
end
function M.tasks(req)
  local settings = cph.settings.instance(req.instance_id)
  if not settings.enabled then return {capabilities={}} end
  return {capabilities={{id="check",kind="recurring",per_account=true}}}
end
function M.task(req)
  return {summary=cph.settings.instance(req.credential.instance_id).site}
end
function M.chat(req, stream)
  stream.message_start({model=cph.settings.instance(req.credential.instance_id).site})
  stream.message_finish({finish_reason="stop"})
end
return M`)
	callbacks := &luaCallbackHost{settings: map[int64]string{
		0:  `{"global":"plugin-value","enabled":true}`,
		11: `{"site":"site-a","enabled":true,"empty":[],"nested":{"count":3}}`,
		12: `{"site":"site-b","enabled":false}`,
	}}
	client, _ := connectLuaCallbacks(t, dir, callbacks)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	login, err := client.Login(ctx, &pb.LoginRequest{InstanceId: 11})
	if err != nil || login.GetError() != nil {
		t.Fatalf("login: %v %v", login, err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(login.Blob, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["site"] != "site-a" || settings["global"] != "plugin-value" || len(settings["empty"].([]interface{})) != 0 || settings["nested"].(map[string]interface{})["count"] != float64(3) {
		t.Fatal(settings)
	}
	profile, err := client.GetProfile(ctx, &pb.CredentialBlob{InstanceId: 12})
	if err != nil || profile.GetDisplayName() != "site-b" {
		t.Fatalf("profile: %v %v", profile, err)
	}
	refresh, err := client.Refresh(ctx, &pb.CredentialBlob{InstanceId: 12})
	if err != nil || refresh.GetError() != nil || !strings.Contains(string(refresh.GetBlob()), "site-b") {
		t.Fatalf("refresh: %v %v", refresh, err)
	}
	models, err := client.ListModels(ctx, &pb.CredentialBlob{InstanceId: 11})
	if err != nil || len(models.GetModels()) != 1 || models.Models[0].Id != "site-a" {
		t.Fatalf("models: %v %v", models, err)
	}
	for _, id := range []int64{0, 11, 12} {
		caps, err := client.ListTaskCapabilities(ctx, &pb.TaskCapabilitiesRequest{InstanceId: id})
		if err != nil || (len(caps.GetCapabilities()) == 1) != (id != 12) {
			t.Fatalf("tasks for %d: %v %v", id, caps, err)
		}
	}
	task, err := client.RunTask(ctx, &pb.RunTaskRequest{Credential: &pb.CredentialBlob{InstanceId: 11}})
	if err != nil || task.GetSummary() != "site-a" {
		t.Fatalf("task: %v %v", task, err)
	}
	chat, err := client.Chat(ctx, &pb.ChatRequest{Credential: &pb.CredentialBlob{InstanceId: 12}})
	if err != nil {
		t.Fatal(err)
	}
	start, err := chat.Recv()
	if err != nil || start.GetMessageStart().GetModel() != "site-b" {
		t.Fatalf("chat start: %v %v", start, err)
	}
	finish, err := chat.Recv()
	if err != nil || finish.GetMessageFinish().GetFinishReason() != "stop" {
		t.Fatalf("chat finish: %v %v", finish, err)
	}
	if _, err := chat.Recv(); err != io.EOF {
		t.Fatalf("chat terminal: %v", err)
	}
	callbacks.mu.Lock()
	callbacks.settings[11] = `{"site":"updated"}`
	callbacks.mu.Unlock()
	login, err = client.Login(ctx, &pb.LoginRequest{InstanceId: 11})
	if err != nil || login.GetError() != nil || !strings.Contains(string(login.GetBlob()), "updated") {
		t.Fatalf("settings update: %v %v", login, err)
	}
}

func TestLuaStoreRoundTripAfterRuntimeRestart(t *testing.T) {
	dir := callbackFixture(t, `local M = {}
function M.login(req)
  local form = req.form or {}
  if form.write then
    local value = form.binary and ("binary" .. string.char(0, 255)) or form.value
    assert(cph.store.put(form.key, value))
  end
  local value, found = cph.store.get(form.key)
  if not found then
    assert(value == nil)
    return {profile={display_name="missing"}}
  end
  return {blob=value,profile={display_name="found"}}
end
return M`)
	callbacks := &luaCallbackHost{}
	client, stop := connectLuaCallbacks(t, dir, callbacks)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	values := map[string]string{"instance:11:account:a": "first", "instance:12:account:b": "binary\x00\xff", "empty": ""}
	for key, value := range values {
		form := map[string]string{"write": "yes", "key": key, "value": value}
		if key == "instance:12:account:b" {
			form["value"], form["binary"] = "", "yes"
		}
		result, err := client.Login(ctx, &pb.LoginRequest{Form: form})
		if err != nil || result.GetError() != nil || string(result.GetBlob()) != value {
			t.Fatalf("write %q: %v %v", key, result, err)
		}
	}
	stop()
	client, _ = connectLuaCallbacks(t, dir, callbacks)
	for _, key := range []string{"instance:11:account:a", "instance:12:account:b", "empty", "missing"} {
		value := values[key]
		result, err := client.Login(ctx, &pb.LoginRequest{Form: map[string]string{"key": key}})
		want := "found"
		if key == "missing" {
			want = "missing"
		}
		if err != nil || result.GetError() != nil || string(result.GetBlob()) != value || result.GetProfile().GetDisplayName() != want {
			t.Fatalf("read %q after restart: %v %v", key, result, err)
		}
	}
}

func TestLuaCallbacksKeepConcurrentAccountsSeparate(t *testing.T) {
	dir := callbackFixture(t, `local M = {}
function M.login(req)
  local site = cph.settings.instance(req.instance_id).site
  local key = tostring(req.instance_id) .. ":" .. req.form.account
  local value = site .. ":" .. req.form.account
  cph.store.put(key, value)
  local stored, found = cph.store.get(key)
  assert(found and stored == value)
  return {blob=stored}
end
return M`)
	client, _ := connectLuaCallbacks(t, dir, &luaCallbackHost{settings: map[int64]string{
		11: `{"site":"site-a"}`, 12: `{"site":"site-b"}`,
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failures := make(chan string, 16)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			id, site := int64(11), "site-a"
			if i%2 == 1 {
				id, site = 12, "site-b"
			}
			account := string(rune('a' + i/2))
			result, err := client.Login(ctx, &pb.LoginRequest{InstanceId: id, Form: map[string]string{"account": account}})
			if err != nil {
				failures <- err.Error()
			} else if result.GetError() != nil {
				failures <- result.Error.Message
			} else if string(result.Blob) != site+":"+account {
				failures <- "account state crossed instances"
			}
		})
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}

func TestLuaHostFailuresReachCaller(t *testing.T) {
	dir := callbackFixture(t, `local M = {}
function M.login(req)
  local op = req.form.op
  if op == "settings" then cph.settings.get()
  elseif op == "get" then cph.store.get("key")
  else cph.store.put("key", "value") end
  return {blob="unexpected success"}
end
function M.profile() return {display_name=cph.settings.get().name} end
return M`)
	client, _ := connectLuaCallbacks(t, dir, &luaCallbackHost{failure: status.Error(codes.Unavailable, "callback failure")})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, op := range []string{"settings", "get", "put"} {
		result, err := client.Login(ctx, &pb.LoginRequest{Form: map[string]string{"op": op}})
		if err != nil || result.GetError().GetCode() == 0 || !strings.Contains(result.GetError().GetMessage(), "callback failure") || len(result.GetBlob()) != 0 {
			t.Fatalf("%s hid callback failure: %v %v", op, result, err)
		}
	}
	if _, err := client.GetProfile(ctx, &pb.CredentialBlob{}); err == nil {
		t.Fatal("profile hid callback failure")
	}
}

func TestLuaCallbacksFollowRPCCancellation(t *testing.T) {
	for _, operation := range []string{"settings", "get", "put"} {
		t.Run(operation, func(t *testing.T) {
			dir := callbackFixture(t, `local M = {}
function M.login(req)
  if req.form.op == "settings" then cph.settings.get()
  elseif req.form.op == "get" then cph.store.get("key")
  else cph.store.put("key", "value") end
  return {}
end
return M`)
			callbacks := &luaCallbackHost{block: operation, started: make(chan struct{}), canceled: make(chan struct{})}
			client, _ := connectLuaCallbacks(t, dir, callbacks)
			deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			ctx, cancel := context.WithCancel(deadline)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := client.Login(ctx, &pb.LoginRequest{Form: map[string]string{"op": operation}})
				done <- err
			}()
			select {
			case <-callbacks.started:
			case <-deadline.Done():
				t.Fatal("callback did not start")
			}
			cancel()
			select {
			case <-callbacks.canceled:
			case <-deadline.Done():
				t.Fatal("host callback ignored RPC cancellation")
			}
			select {
			case err := <-done:
				if status.Code(err) != codes.Canceled {
					t.Fatal(err)
				}
			case <-deadline.Done():
				t.Fatal("Lua RPC did not finish")
			}
		})
	}
}

func TestLuaSettingsRejectInvalidArgumentsAndResponses(t *testing.T) {
	dir := callbackFixture(t, `local M = {}
function M.login(req)
  for _, id in ipairs({-1, 1.5, math.huge}) do
    local ok = pcall(cph.settings.instance, id)
    assert(not ok)
  end
  return {blob=cph.json.encode(cph.settings.get())}
end
return M`)
	for _, raw := range []string{"[]", "null", "broken JSON"} {
		client, _ := connectLuaCallbacks(t, dir, &luaCallbackHost{settings: map[int64]string{0: raw}})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err := client.Login(ctx, &pb.LoginRequest{})
		cancel()
		if err != nil || !strings.Contains(result.GetError().GetMessage(), "JSON object") {
			t.Fatalf("invalid settings %q: %v %v", raw, result, err)
		}
	}
	impl := New(dir)
	impl.SetHost(nil)
	defer impl.Close()
	result, err := impl.Login(context.Background(), &pb.LoginRequest{})
	if err != nil || !strings.Contains(result.GetError().GetMessage(), "unavailable") {
		t.Fatalf("missing callbacks: %v %v", result, err)
	}
}
