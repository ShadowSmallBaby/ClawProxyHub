# 原生 JS + Go 笔记扩展

一个 `.cphext` 打包公共 HTML/JS/CSS 前端和五个平台 Go 程序。页面通过公共桥请求宿主抽屉，后端通过 `Host.Batch` 将笔记与标签写入两张表；SQLite 连接及执行全部由宿主处理。

在 ClawProxyHub 仓库根目录运行：

```sh
python scripts/build-extension.py --project examples/extensions/notes/build.json --out build/extension-example --development-key
```

输出 `build/extension-example/sample-notes-0.1.0.cphext`、`trust.json` 和 `index.json`。开发签名仅用于本地测试；在测试核心配置生成的信任文件，然后通过扩展中心显式安装包，或者挂载该目录首次安装。已有安装不会自动更新。

包申请 `service.execute/storage.read/storage.write`，信任身份需有 `native: true`。安装后从“插件”页工具栏的“笔记”入口打开，点击“新建”填写内容与标签；刷新或重启后数据保留。示例没有 Vue、Node 或前端构建依赖；构建器将公共 `bridge.js` 复制为 `frontend/cph.js`。

`backend/go.mod` 的本地 `replace` 指向核心仓库。独立扩展仓库应改用匹配目标宿主的 SDK 模块版本。Go 程序是受信原生进程；前端 iframe 的沙箱不代表后端也具有 OS 沙箱。

更多契约见 [Go 后端](../../../sdk/extension/backend.md)、[业务存储](../../../sdk/extension/storage.md) 和 [UI 协议](../../../sdk/extension/README.md)。

浏览器回归见 [extension-service.spec.ts](../../../web/tests/extension-service.spec.ts)。先在 `web/` 运行 `pnpm run build:profiles`，再运行 `pnpm exec playwright test tests/extension-service.spec.ts`。测试构建真实签名包和核心，验证页面创建笔记、刷新、主题同步、沙箱隔离、系统设置导出备份、升级后清理废弃表及重启持久化。测试只执行当前平台的 Go 程序，其余入口通过交叉编译生成。
