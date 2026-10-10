# Lua 插件实现

Lua 插件通过已安装并启用的 LuaHost 执行。脚本作者无需编译或随包携带 LuaHost；核心可按与 Go 相同的 RPC 调用它，但 Lua 可用 API 与字段以目标宿主实现为准。

## 模块、握手与返回值

源码放 `P/plugins-lua/<name>/manifest.json`、`main.lua`，需要拆分时使用 `lib/*.lua`。当前打包器只收集 `main.lua` 和 `lib/` 直接子级的 `.lua` 文件，不会递归携带任意资源。使用 `require("lib.util")`，不要依赖本机 Lua 模块或 C 扩展。

`main.lua` 创建模块表，函数挂到该表，最后 `return M`。使用 `function M.login(req)` 这样的点号定义；宿主不传隐式 `self`。流方法同样使用 `stream.content_delta({...})`，不能改成冒号调用。

| 函数 | 入参与返回形状 |
| --- | --- |
| `handshake(req)` | 收 `protocol_version/core_version`；回 `{manifest={...}}` 或 `{error={code,message}}` |
| `login(req)` | 收 `method_id/form/state/instance_id`；回 `{blob,profile}` 或 `{next={...}}` 或 error |
| `refresh(cred)` | 回 `{blob,profile}`，无凭据变更可回 `{}` |
| `profile(cred)` | 直接回账号档案 table，无外层 `profile` |
| `models(cred)` | 回 `{models={{id,label,context_window,supports_tools,supports_stream}}}` |
| `chat(req, stream)` | 收统一信封，用 stream 方法发事件，返回值不作响应 |
| `tasks(req)` | 收 `instance_id`（0 表示插件级查询），回 `{capabilities={{id,label,kind,per_account,default_schedule}}}`；兼容原无参函数 |
| `task(req)` | 收 `capability_id/credential/context`；回 `{summary,changed,blob,detail_json,notification,error}` |

以上表中字段名表示结构，具体值按业务填充。凭据可能缺失：普通凭据 RPC 收空 table，Chat / Task 的 `req.credential` 可能为 nil。`blob`、登录 `state`、`detail_json` 均为字符串；JSON 凭据用 `cph.json.encode/decode`，不按 protojson 的 bytes 规则额外做 base64。

新发布插件显式实现 `handshake` 并检查支持的协议；返回 label、capabilities、endpoints、auth_methods 等。`name/version/author` 由宿主用包清单覆盖，协议由握手上下文管理，不在脚本里维护第二份发行版本。虽然 LuaHost 支持缺少 handshake 时读清单，打包器不会保留清单内业务能力，不能依赖这一回退发布插件。

## `cph.*` 实际接口

查 `C/hosts/luahost/cph.go`、`host.go` 与 `http.go` 确认目标宿主支持情况。HTTP 使用一个参数表，不是位置参数：

```lua
-- url、headers 来自当前调用已校验的配置与凭据。
local ok, resp = pcall(cph.http.request, {
  method = "GET",
  url = url,
  headers = headers,
  timeout = 30000,
})
```

`ok=false` 表示请求未完成（异常文本在 `resp`）；`ok=true` 时读 `resp.status/body/headers`，4xx/5xx 也正常返回表，必须检查 HTTP 状态。`body` 可为原始字符串或自动 JSON 编码的 table；`timeout` 单位为毫秒。

| API | 实际行为 |
| --- | --- |
| `cph.settings.get()` | 读取当前插件设置，返回 table；插件身份自动绑定 |
| `cph.settings.instance(instance_id)` | 读取当前插件下的实例合并设置，返回含站点地址与名称的 table；0 等价于 `get()` |
| `cph.store.get(key)` | 返回原始字符串与 found；键不存在为 `nil, false`，空值为 `"", true` |
| `cph.store.put(key, value)` | 持久化字符串，成功返回 true；表数据先 JSON 编码 |
| `cph.http.stream(opts)` | 参数表含 `method/url/headers/body/format/stream`；OpenAI SSE 用 `format="openai"` 与 `stream=stream` |
| `cph.openai.chat_body(req)` | 将统一信封转换为 OpenAI 请求 table，之后只修改上游确实需要的字段 |
| `cph.json.encode(value)` / `decode(text)` | JSON 编解码，失败抛异常 |
| `cph.log.info(msg, fields)` | 结构化日志；同组还有 `debug/warn/error`，不是 `cph.log(level, msg)` |
| `cph.hash.md5/sha256(text)` | 返回十六进制摘要 |
| `cph.hash.hmac_sha256(data, key)` | 参数顺序为数据、密钥 |
| `cph.hash.base64url_encode/decode(text)` | Base64 URL 编解码 |
| `cph.time.now()` / `sleep(ms)` | 当前时间与等待均以毫秒计；凭据信封 `updated_at` 仍是 Unix 秒 |
| `cph.random.uuid()` / `hex(n)` | UUID 或 n 个随机字节编码成的 2n 个十六进制字符 |

`http.stream` 建连或 HTTP 失败可回 `false, err`；解析过程中也可能直接调用 `stream.failed` 或抛异常。即使返回 true，也不能据此额外补发成功终态。OpenAI parser 驱动结束事件，脚本负责按该 parser 行为补齐开始事件。不要在流失败后无条件重放已输出的内容。

手工发事件使用 `stream.message_start/content_delta/reasoning_delta/tool_call_delta/message_finish/failed`，每个方法收一个事件 table，例如 `stream.failed({code=401, message="凭据无效"})`。用量及终态规则见[公共契约](contract.md)。

## 多实例设置与存储

握手声明 `instances` 和 JSON 字符串 `instance_schema`；登录用 `req.instance_id`，账号类函数用 `cred.instance_id`，Chat / Task 用 `req.credential.instance_id` 读取设置。`tasks(req)` 也能按实例设置裁剪能力。配置读取示例：

```lua
local site = cph.settings.instance(req.instance_id)
local base_url = site.base_url
```

宿主按插件设置、实例设置、实例地址与名称依次覆盖；每次读取查询核心，插件自行缓存时要按实例分键并设置失效策略。当前插件以外的实例会被核心拒绝，不能通过参数指定其他插件名。

Store 通过核心数据库持久化，可跨 VM 和运行时重启读取；同一插件的所有实例共享命名空间。按用途加入实例和账号 ID，例如 `instance:11:account:42:cursor`。JSON 内容显式 encode/decode，读后再写不具备原子更新语义；凭据仍走 RPC 的 blob/state。

上述回调跟随当前 RPC 取消。宿主不可用、数据库失败或非法实例会抛异常，可用 `pcall` 处理；不要把失败当成默认设置、键不存在或写入成功。旧环境需要同时更新核心与 LuaHost 后使用这些接口。

## 运行时限制会影响选型

- 只开放 base / table / string / math 和沙箱 require；无 `os/io/debug/package`，也没有 `print/dofile/load/loadfile/loadstring`。日志使用 `cph.log.*`，网络只走 `cph.http.*`。
- HTTP 自动使用本次凭据信封绑定的代理；Login 没有凭据信封。当前流解析仅实现 `format="openai"`，没有 `format="anthropic"/"responses"` 或通用 WebSocket API，不能从 Go SDK 能力推断 Lua 可直接使用。
- 字段映射不是完整 proto 镜像。需要原始消息、缓存断点、引用或刷新通知等字段时检查 `proto.go/account.go/result.go/stream.go`；例如当前 `refresh` 未映射 notification，`stream.failed` 未映射 retryable。
- VM 池会复用脚本状态，同一插件不同调用也可能落到不同 VM。模块级变量不适合保存“当前账号”或跨 RPC 登录状态；凭据与登录状态返回 blob/state，其他持久化小状态使用按实例、账号分键的 Store。
- 空 table 默认编码为 JSON 对象；需要空数组可用 `cph.json.decode("[]")`。需保留 JSON null 时通过解码/编码保留表示，不把它当普通 Lua nil。

这些是当前宿主边界；目标版本实现变化时以源码核对结果为准。参考 `P/plugins-lua/autoclaw/main.lua` 的真实调用形式、`C/examples/luatask/main.lua` 的任务结果结构，不复制其上游常量。

## 本地编辑器场景

自建插件位于配置的插件安装根下 `local/<name>/`，默认 `C/data/plugins/local/<name>/`。此模式由核心建档生成清单，编辑入口只开放 `main.lua`。保留 `PLUGIN_NAME` 和中英文 label 声明，身份建档后不改名；具体行为查 `C/internal/plugin/authoring.go`。

编辑本地插件时使用既有编辑、保存和重载流程；保存文件与重载运行实例是不同动作。准备市场分发时再整理到 `P/plugins-lua/<name>/`，补作者与源版本清单，显式实现 handshake 并用打包器输出。

脚本包成功生成并不证明能加载或正确运行。验证方式见[验证与打包](validation.md)。
