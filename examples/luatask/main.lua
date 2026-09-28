-- main.lua — lua 任务能力示范插件：tasks() 声明 + task() 执行的最小骨架。
-- 装入 data/plugins/<name>/（含本文件）即可被共享 luahost 加载；任务由核心调度触发。
-- 能力声明须与实际实现一致（tasks 能力 ⇔ tasks()/task() 函数齐全）。

local M = {}

-- 可选设置：从宿主读（settings_schema 见 manifest.json），此处演示默认阈值。
local DEFAULT_THRESHOLD = 1

-- plugin.tasks 声明任务能力：供核心装插件时预填调度、管理端展示。
-- kind: once / recurring / both；per_account: 按账号逐个执行；default_schedule: 推荐调度。
function M.tasks()
  return {
    capabilities = {
      {
        id = "daily_report",
        label = { ["zh-CN"] = "每日报告", ["en"] = "Daily report" },
        kind = "recurring",
        per_account = true,
        default_schedule = "daily 09:00",
      },
    },
  }
end

-- plugin.task 执行能力：req 表 {capability_id, credential, context}。
-- credential 可空（全局任务）；context 为任务规则附加参数（如 threshold）。
-- 返回 {summary, changed, blob, detail_json, notification, error}：
--   summary        运行摘要（任务记录展示）
--   detail_json    结构化明细快照（账号详情弹窗渲染）
--   changed+blob   凭据变更时回吐新凭据（如 token 刷新）
--   notification   站内通知 {title, content, level}
--   error          {code, message}；code 401 → 核心标记凭据失效，502 → 可重试
function M.task(req)
  local cap = req.capability_id or ""
  if cap ~= "daily_report" then
    return { error = { code = 400, message = "unknown capability: " .. cap } }
  end

  -- 凭据判空：全局任务无凭据，按账号任务缺失时回 400
  local cred = req.credential or {}
  local account_id = cred.account_id or ""

  -- 任务参数：规则 context 里带 threshold 则用之，否则默认
  local ctx = req.context or {}
  local threshold = tonumber(ctx.threshold) or DEFAULT_THRESHOLD

  -- 演示：真实插件在此调 cph.http.request 打上游（如签到接口），
  -- 用 cph.hash / cph.json 处理签名与响应；此处仅产出一份演示报告。
  local remaining = 42
  local result = {
    summary = "账号 " .. account_id .. " 每日报告完成（余额 " .. remaining .. "）",
    detail_json = cph.json.encode({
      account = account_id,
      items = {
        { name = "余额检查", value = remaining, threshold = threshold },
      },
    }),
  }
  cph.log.info("daily_report done", { account = account_id })

  -- 演示通知：余额低于阈值时提醒（真实签到插件在"需人工前往"时用）
  if remaining < threshold then
    result.notification = {
      title = "每日报告 · 余额不足",
      content = "当前余额 " .. remaining .. "（阈值 " .. threshold .. "），请及时处理",
      level = "warning",
    }
  end

  return result
end

return M
