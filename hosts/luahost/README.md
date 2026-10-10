# Lua Host 共享运行时

根包提供 `New(workspace)`，共用脚本契约、VM 和 RPC 实现；桌面入口为 `cmd/main.go`，Android JNI 入口为 `android/`。组件元数据与版本在 [manifest.json](manifest.json)。

```sh
go test ./...
go build -o luahost ./cmd
```

桌面运行时通过 `.cphhost` 安装，在主库根目录执行：

```sh
python scripts/build-packages.py --platform windows/amd64 --only lua-runtime --out build/packages --development-key
```

Android 使用主库 `android/gradlew :app:packageLuaHost` 生成与 APP 同证书签名的包，见 [Android 构建说明](../../android/README.md)。两种平台均通过核心扩展管理器挂载发现、验签、首次安装和启停，核心不再携带原始运行时二进制。

桌面插件使用已验证的可执行文件，Android 私有 Lua 服务加载已选中的动态库。Service 会话结束关闭空闲 VM，进行中的 RPC 随连接取消；脚本副本位于临时目录，清理不修改核心工作区。停用或卸载运行时前必须停止依赖它的 Lua 插件。

## 设置与持久化状态

Lua 通过以下接口复用核心 `ClawHost` 回调。插件身份从工作区清单绑定，核心校验调用方和实例归属；脚本不传其他插件名。

| 接口 | 返回值与作用 |
| --- | --- |
| `cph.settings.get()` | 当前插件的设置 table，含核心保留的浏览器 UA 设置 |
| `cph.settings.instance(instance_id)` | 插件设置、实例设置、实例地址和名称依次合并后的 table；0 等价于 `get()` |
| `cph.store.get(key)` | 原始字符串和 `found`；不存在返回 `nil, false`，空字符串返回 `"", true` |
| `cph.store.put(key, value)` | 持久化原始字符串，成功返回 true；JSON 对象需先 `cph.json.encode` |

读取设置每次查询核心，因此管理端保存后下次调用即可读取。回调失败、非法实例或非 JSON 对象设置会抛 Lua 异常，可用 `pcall` 处理；失败不会变成空设置、键不存在或写入成功。调用跟随当前 RPC 的取消与截止时间。

```lua
function M.login(req)
  local site = cph.settings.instance(req.instance_id)
  -- 使用 site.base_url 与站点字段完成登录，再返回凭据 blob。
end

function M.tasks(req)
  local site = cph.settings.instance(req.instance_id)
  if not site.enable_checkin then return { capabilities = {} } end
  return { capabilities = {
    { id = "checkin", kind = "recurring", per_account = true },
  } }
end
```

`tasks(req)` 现在收到 `{instance_id=...}`；0 表示插件级能力查询，已有无参数 `tasks()` 会忽略新增入参。账号类 RPC 从 `cred.instance_id` 读取实例，Chat 和 Task 从 `req.credential.instance_id` 读取。

Store 按插件隔离并持久化到核心数据库，可跨 VM 与运行时重启读取。需要账号或实例隔离时，自行使用如 `instance:11:account:42:cursor` 的键；一次读后再写不是原子更新。账号凭据仍经 RPC 的 blob/state 交回核心，不放入 Store。

使用新增接口时同步更新核心和 Lua Host，确保设置归属校验、数据库失败回传和取消传播都生效。插件作者的完整流程见[插件开发技能](../../skills/clawproxyhub-plugin/SKILL.md)。
