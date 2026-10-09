package github.shadowbaby.clawproxyhub.core;

// 原生管理页面无需为了查询生命周期状态而加载 Go 库。
final class CoreState { static volatile boolean started; }
