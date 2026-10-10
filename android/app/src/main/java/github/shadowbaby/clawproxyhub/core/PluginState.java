package github.shadowbaby.clawproxyhub.core;

import org.json.JSONObject;

// 插件卡片统一展示运行时依赖、安装及运行状态，不把空闲的 Lua worker 视为停用。
final class PluginState {
    static String luaRuntime(JSONObject state, boolean remoteOnly) {
        if (state == null || state.optString("hash").isEmpty()) return "runtime_missing";
        if (remoteOnly || !state.optBoolean("enabled")) return "runtime_stopped";
        if (!state.optBoolean("available")) return "runtime_unavailable";
        return "";
    }
    static String dependency(JSONObject plugin, String luaRuntime) {
        return "lua".equals(plugin.optString("runtime")) ? luaRuntime : "";
    }
    static String installed(JSONObject plugin, String luaRuntime) {
        String dependency = dependency(plugin, luaRuntime);
        if (!dependency.isEmpty()) return dependency;
        if (!plugin.optBoolean("available", true)) return "unavailable";
        if (!plugin.has("enabled")) return "installed";
        if (!plugin.optBoolean("enabled")) return "disabled";
        if (plugin.has("running")) return plugin.optBoolean("running") ? "running" : "stopped";
        return "enabled";
    }
    static String market(JSONObject entry, JSONObject local, String luaRuntime) {
        String dependency = dependency(entry, luaRuntime);
        if (!dependency.isEmpty()) return dependency;
        if (local != null && local.optString("version").equals(entry.optString("version"))) return installed(local, luaRuntime);
        if (!entry.optBoolean("installable")) return entry.optString("unavailable_reason", "metadata");
        return local == null ? "not_installed" : "update_available";
    }
}
