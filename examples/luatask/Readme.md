# luatask — Lua 任务能力示范插件

最小的 Lua 插件任务骨架：`tasks()` 声明能力 + `task(req)` 执行，演示核心任务调度的接入方式。

## 使用

把 `main.lua` 与 `manifest.json` 拷到 `data/plugins/luatask/`，核心以共享 luahost
（`luahost --dir data/plugins/luatask`）加载；管理端 → 任务规则 → 选 `luatask` 插件的
`daily_report` 能力即可创建调度（或立即执行一次）。

## 要点

- 能力声明须与实现一致：声明 `tasks` 能力 ⇔ `tasks()` / `task()` 函数齐全。
- `task(req)` 入参：`capability_id` / `credential`（可空）/ `context`（规则附加参数）。
- 返回 `{summary, changed, blob, detail_json, notification, error}`；`error.code`
  401 → 凭据失效标记，502 → 可重试。
- 出站请求只走 `cph.*`（http/json/hash/time/log），无裸 socket；VM 复用池并发安全，
  长对话不阻塞任务调用。

完整 Lua 插件范式（凭据自持、token 刷新、登录流）见插件仓库 `plugins-lua/autoclaw/main.lua`。
