# 公共契约

路径记号 `C`、`P` 见技能入口。字段以目标版本 `sdk/proto/cph.proto` 为准；Lua 还需确认宿主是否映射该字段。

## 清单与能力

新插件源清单示例（将示例身份换成作者信息）：

```json
{
  "name": "myplugin",
  "version": "0.1.0",
  "author": "your-github-name",
  "label": { "zh": "我的上游", "en": "My Upstream" },
  "protocol_version": 2
}
```

`name` 与目录名、握手身份一致，在 Go / Lua 两个源码根下全局唯一。图标可省略；设置 `icon` 时文件必须存在。`label.zh` 与 `label.en` 都要填写。版本唯一来源是源 `manifest.json`；Go 工厂接收注入版本，LuaHost 从包清单读取身份。

打包器按 SDK 写入 `protocol_version`，按 Lua 源目录补 `runtime: "lua"` 与默认 `entry: "main.lua"`。源清单中的能力、授权和 schema 不会随当前打包器保留；发布插件应通过 Handshake 声明它们。

| `capabilities` | Go RPC | Lua 函数 |
| --- | --- | --- |
| 无条件握手 | `Handshake` | `handshake(req)` |
| `login` | `Login` | `login(req)` |
| `refresh` | `Refresh` | `refresh(cred)` |
| `account` | `GetProfile` | `profile(cred)` |
| `models` | `ListModels` | `models(cred)` |
| `chat` | `Chat` | `chat(req, stream)` |
| `tasks` | `ListTaskCapabilities`、`RunTask` | `tasks(req)`、`task(req)` |
| `instances` | 各相关 RPC 按 `instance_id` 隔离 | `cph.settings.instance(instance_id)` 读取实例设置，并按实例隔离状态 |

`endpoints` 声明核心允许的客户端入口：`chat_completions`、`messages`、`responses`；空值默认前两者。它不决定上游协议，入口归一化后仍需按上游实际能力选适配器。

`settings_schema` 与 `instance_schema` 在握手中是 JSON Schema **字符串**。实例固定有名称和 `base_url`，实例 schema 只补站点特有字段。用户需要多站点时，声明 `instances`（Go 使用 `sdk.CapabilityInstances`），按请求中的实例 ID 读取合并设置；未声明时核心只提供默认实例。

## 登录、凭据与账号

- `AuthMethod` 声明表单与能力，`AuthField.type` 使用 `text/password/textarea/file/phone`。`refreshable/auto_relogin/profile` 按该登录方式实际行为声明。
- 浏览器授权的 `callback` 为 `auto/wait/auto_wait`，按真实回调接收方式选择。后续表单来自 `LoginNextStep.fields`；不要为插件另写一套核心登录页面。
- `Login` 完成时返回 `blob` 与可选 `profile`；未完成返回 `next`。多步状态用 `next.state` 签发、从请求 `state` 取回，避免仅存于进程全局变量。
- `CredentialBlob` 包含插件自定义 `blob`，以及独立的 `account_id/instance_id/proxy/updated_at`。Go 只反序列化 `blob.Blob`，另取信封上下文；如果放在同一结构里，用私有字段或 `json:"-"` 排除临时上下文。Lua 的 `cred.blob` 是原始字符串。
- `RefreshResult.blob` 空表示凭据不变。静态密钥不需要伪造刷新流程；换 token 时返回完整新 blob，兼容已有保存格式。Chat 中没有“更新 blob”结果字段，不把内存里刷新成功当成已持久化。
- 余额用数字字符串：`quota.credits/used_credits/total_credits`；提供积分快照时同步 `credits_json` 中的 `remaining/used/total`。自定义详情用 `sections`，展示名脱敏。

业务失败使用对应结果的 `Error`，Chat 使用 `TaskFailed`。保留认证失效、额度不足、限流和网络失败的区别；常见状态为 401、402、429、502，但映射须依据上游含义，不能把所有 403 当成 token 过期。是否重试结合核心处理与当前流状态判断。

## Chat 信封与事件

核心处理客户端 OpenAI / Anthropic / Responses 协议，插件处理上游方言。请求中的 `source` 是客户端入口；只有上游支持同一方言时才据它直接分流。

| 上游 | 请求构造 | SSE 解析 |
| --- | --- | --- |
| Chat Completions | `openaiup.ChatBody(req)` | `openaiup.NewParser(emit)` |
| Anthropic Messages | `anthropicup.ChatBody(req)` | `anthropicup.NewParser(emit)` |
| Responses | `responsesup.ChatBody(req)` | `responsesup.NewParser(emit)` |

修改转换逻辑时保留这些语义：

- `messages[].parts` 非空时是完整有序内容，已经包含文本；不能再把 `text` 拼一遍。保留图片、工具调用与结果关联、工具失败标记，以及可回放推理块的签名。
- 保留请求的工具定义、`tool_choice` 和支持的 `extra`。不要把 `extra` 全部当 HTTP 头或全部直接合入上游请求。UA 使用 `sdk.ExtraClientUserAgent`，其他头按上游实际要求选择。
- 对话 RPC 始终返回事件，即使客户端 `stream=false`；聚合由核心完成。通常使用上游流式接口，避免在插件里重新实现客户端聚合协议。
- 正常流为 `MessageStart` → 文本 / 推理 / 工具增量 → `MessageFinish`；失败为 `TaskFailed`。确认开始事件由插件还是解析器负责，避免遗漏或重复。工具 `id/name/arguments_delta` 必须能重组为稳定调用。
- 调用 SDK 扫描助手完成解析器收尾。上游空流、坏帧、缺终态、读取中断不能当正常 `stop`；已经发出失败或成功终态后，不再追加另一个终态。转发产生可见增量后，不能无条件重试并重复输出。
- `finish_reason` 保留 `stop/stop_sequence/tool_calls/length/content_filter` 等已定义值，`stop_sequence` 保留命中原值；拒绝、引用、推理签名等字段按目标适配器与宿主支持情况处理。

用量遵循 Anthropic 语义：

```text
总输入 = input_tokens（非缓存）+ cached_tokens（缓存读）+ cache_creation_tokens（缓存写）
reasoning_tokens 已包含在 output_tokens 中，不重复相加
```

OpenAI 的含缓存输入计数交给 SDK 解析器拆分。自写解析器时用样本验证缓存、工具碎片、跨帧数据和结束用量；不要按文本长度猜 token。

## 任务

调度由核心完成，插件声明能力并执行一次调用。`kind` 使用 `once/recurring/both`；`per_account` 为真时从 `req.credential` 取账号，处理凭据可能缺失的情况。

Go 的能力查询可按 `TaskCapabilitiesRequest.instance_id` 裁剪；Lua 对应 `tasks(req)` 中的 `req.instance_id`，0 表示插件级能力查询，旧 `tasks()` 继续兼容。执行返回摘要，结构化明细放 JSON 字符串 `detail_json`；只有凭据实际改变时返回 `changed=true` 和新 `blob`。业务性跳过写清摘要，失败返回 error，不能为了避免任务失败而吞掉错误。站内提醒使用 `notification`，无需自建调度器或通知页面。
