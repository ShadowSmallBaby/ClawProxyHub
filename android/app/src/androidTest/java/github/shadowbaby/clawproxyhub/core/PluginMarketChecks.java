package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import org.json.JSONArray;
import org.json.JSONObject;

// 只读取设备的目录与运行时状态，不初始化核心、不安装插件或修改设置。
final class PluginMarketChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        JSONObject host = new HostStore(context).current();
        String runtime = PluginState.luaRuntime(host, AppPreferences.remoteOnly(context));
        JSONArray catalog = new PluginMarket(context).load(false), installed = InstalledPlugins.read(context), scripts = new JSONArray();
        for (int i = 0; i < catalog.length(); i++) {
            JSONObject item = catalog.getJSONObject(i);
            if (!"lua".equals(item.optString("runtime"))) continue;
            if (!item.optBoolean("installable")) throw new AssertionError("Universal Lua package is unavailable: " + item.optString("unavailable_reason"));
            JSONObject local = null;
            for (int j = 0; j < installed.length(); j++) {
                JSONObject candidate = installed.getJSONObject(j);
                if (candidate.optString("name").equals(item.optString("name"))) local = candidate;
            }
            scripts.put(new JSONObject().put("name", item.getString("name")).put("state", PluginState.market(item, local, runtime)));
        }
        JSONObject manifest = host == null ? null : host.optJSONObject("manifest");
        return new JSONObject().put("ok", true).put("runtime", runtime.isEmpty() ? "ready" : runtime)
                .put("runtime_version", manifest == null ? "" : manifest.optString("version")).put("scripts", scripts);
    }
}
