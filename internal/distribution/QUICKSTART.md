# ClawProxyHub

解压后进入 `ClawProxyHub` 目录，运行 `./cph`；Windows 运行 `cph.exe`。默认访问地址为 `http://localhost:8080`，按页面提示完成管理员初始化。

Web 界面已嵌入核心。`data/packages/` 携带 Lua Host 和 Lua 编辑器签名包，首次启动时自动校验并安装；已安装、停用或卸载的状态会保留。设置 `CPH_INSTALL_PACKAGES=false`，或使用 `./cph --install-packages=false` 可关闭自动安装，随后从扩展中心手动安装。

运行时安装到 `data/hosts/`，功能扩展安装到 `data/extensions/`，业务插件安装到 `data/plugins/`。数据目录可用 `CPH_DATA_DIR` 配置；显式设置的数据库、插件和挂载包路径按各自配置解析。

`cli` 是连接已有核心的命令行客户端，Windows 文件名为 `cli.exe`。运行 `./cli help` 查看用法；使用 `CPH_BACKEND` 与 `CPH_TOKEN` 配置连接，也可用 `--url` 和 `--token-file` 指定地址与令牌文件。

`distribution.lock.json` 记录发行版本、平台、程序摘要、预配包和签名公钥。请保留该文件与程序一起部署。软件许可证见 `LICENSE`。

项目主页：https://github.com/ShadowSmallBaby/ClawProxyHub
