# CLI

连接已有 ClawProxyHub 核心的命令行客户端，无需安装到核心或扩展中心。
客户端随 full ZIP 提供。解压后进入 `ClawProxyHub` 目录运行 `./cli`，Windows 使用 `cli.exe`。

```sh
export CPH_BACKEND='http://127.0.0.1:8080'
./cli --token-file token.txt status
./cli --token-file token.txt actions
./cli --token-file token.txt describe core.extensions.list
```

`--url` 覆盖 `CPH_BACKEND`；认证可使用 `--token-file` 或 `CPH_TOKEN`。
`actions` 和 `describe` 列出当前核心允许调用的动作；`invoke ID [JSON|-]` 执行动作。
`install FILE [GRANTS_CSV]`、`enable ID`、`disable ID`、`uninstall ID` 管理功能扩展，
桌面核心也支持运行时包。Android 的 UI、Host 和业务插件由 APP 原生页管理。

`mcp` 启动 stdio MCP 客户端，标准输出仅用于协议通信。其他命令返回 JSON；
退出码为 0 成功、1 本地错误、3 认证失败、4 功能不可用、5 后端错误、130 取消。
