package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import java.io.File;
import org.json.JSONArray;
import org.json.JSONObject;

// 显式安装随 APP 挂载的 Lua Host，复用原生入口的验签与候选 worker 检查。
final class HostPackageChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        if (AppPreferences.remoteOnly(context)) throw new IllegalStateException("Local mode is disabled");
        HostStore hosts = new HostStore(context);
        JSONObject previous = hosts.current();
        if (previous != null && !previous.optBoolean("enabled")) throw new IllegalStateException("Lua Host is disabled; enable it before this check");
        RuntimePackages runtimes = new RuntimePackages(context);
        stage(test, "listing runtime packages");
        JSONArray catalog = runtimes.request(new JSONObject().put("operation", "list")).getJSONArray("packages");
        stage(test, "runtime packages listed");
        File bundled = new File(context.getFilesDir(), "packages/bundled").getCanonicalFile();
        JSONObject selected = null;
        for (int i = 0; i < catalog.length(); i++) {
            JSONObject item = catalog.getJSONObject(i), manifest = item.optJSONObject("manifest");
            if (!new File(item.getString("path")).getCanonicalFile().getParentFile().equals(bundled)) continue;
            if (manifest == null) throw new AssertionError("Bundled Lua Host verification failed: " + item.optString("error"));
            if ("lua-runtime".equals(manifest.optString("id"))) selected = item;
        }
        if (selected == null) throw new AssertionError("Bundled Lua Host is missing");
        stage(test, "installing bundled runtime");
        runtimes.request(new JSONObject().put("operation", "mounted").put("id", "lua-runtime")
                .put("sha256", selected.getString("sha256")).put("grants", selected.getJSONObject("manifest").getJSONArray("permissions")));
        stage(test, "runtime installed");
        JSONObject current = hosts.current();
        if (current == null || !current.optBoolean("enabled") || !current.optBoolean("available") || hosts.library() == null)
            throw new AssertionError("Installed Lua Host is not available");
        if (!selected.getString("sha256").equals(current.getString("hash"))) throw new AssertionError("Installed runtime differs from bundled package");
        return new JSONObject().put("ok", true).put("version", current.getJSONObject("manifest").getString("version"))
                .put("sha256", current.getString("hash")).put("runtime_probe", "passed").put("market", PluginMarketChecks.run(test));
    }

    private static void stage(CoreInstrumentation test, String message) {
        android.util.Log.i("CPH-host-check", message);
        android.os.Bundle status = new android.os.Bundle();
        status.putString("stage", message);
        test.sendStatus(0, status);
    }
}
