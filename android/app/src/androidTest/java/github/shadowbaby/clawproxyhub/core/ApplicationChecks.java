package github.shadowbaby.clawproxyhub.core;

import android.app.job.JobScheduler;
import android.content.ComponentName;
import android.content.Context;
import android.content.pm.ServiceInfo;
import java.net.HttpURLConnection;
import java.net.URL;
import org.json.JSONArray;
import org.json.JSONObject;

// 验证本机 Lua 服务与后台调度，切换模式后恢复原偏好。
final class ApplicationChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        boolean original = AppPreferences.remoteOnly(context);
        android.content.SharedPreferences nativePlugins = context.getSharedPreferences("native-plugins", 0);
        java.util.Map<String, ?> installed = nativePlugins.getAll();
        JobScheduler jobs = context.getSystemService(JobScheduler.class);
        try {
            android.content.SharedPreferences.Editor edit = nativePlugins.edit();
            for (String key : installed.keySet()) if (key.startsWith("current:")) edit.putBoolean("enabled:" + key.substring(8), false);
            if (!edit.commit()) throw new AssertionError("Cannot isolate application verification");
            TaskScheduler.setRemoteOnly(context, false);
            NativeCore.initialize(context);
            ServiceInfo lua = context.getPackageManager().getServiceInfo(
                    new ComponentName(context, "github.shadowbaby.clawproxyhub.lua.LuaService"), 0);
            if (lua.exported || !lua.processName.equals(context.getPackageName() + ":lua"))
                throw new AssertionError("Lua must be a private application process");
            ServiceInfo probe = context.getPackageManager().getServiceInfo(
                    new ComponentName(context, "github.shadowbaby.clawproxyhub.lua.LuaService$Probe"), 0);
            if (probe.exported || !probe.processName.equals(context.getPackageName() + ":lua_probe"))
                throw new AssertionError("Runtime validation must use a separate private process");
            JSONObject session = NativeCore.session();
            String address = session.getString("address"), token = session.getString("token");
            CoreInstrumentation.verifyCapabilities(test.request(address + "/admin/capabilities", token));
            test.verifyLua(address, token);
            TaskScheduler.schedule(context);
            if (jobs.getPendingJob(TaskScheduler.JOB_ID) == null) throw new AssertionError("scheduler missing");
            TaskScheduler.setRemoteOnly(context, true);
            if (jobs.getPendingJob(TaskScheduler.JOB_ID) != null) throw new AssertionError("remote-only job retained");
            boolean blocked = false;
            try { NativeCore.session(); } catch (IllegalStateException expected) { blocked = true; }
            if (!blocked) throw new AssertionError("remote-only started local core");
            HttpURLConnection connection = (HttpURLConnection) new URL(address + "/admin/capabilities").openConnection();
            connection.setConnectTimeout(2000);
            connection.setReadTimeout(2000);
            try {
                connection.getResponseCode();
                throw new AssertionError("local listener still active");
            } catch (java.io.IOException expected) { /* 本地监听应已关闭。 */ }
            finally { connection.disconnect(); }
            TaskScheduler.setRemoteOnly(context, false);
            if (jobs.getPendingJob(TaskScheduler.JOB_ID) == null) throw new AssertionError("scheduler not restored");
            session = NativeCore.session();
            test.request(session.getString("address") + "/admin/capabilities", session.getString("token"));
            return new JSONObject().put("ok", true).put("checks", new JSONArray(new String[]{
                    "bundled-private-lua", "lua-task-callback-cancel", "remote-only-stops-core-and-jobs", "local-mode-restored"}));
        } finally {
            if (CoreState.started) { NativeCore.decode(NativeCore.stop()); CoreState.started = false; }
            WorkspaceChecks.restore(nativePlugins, installed);
            TaskScheduler.setRemoteOnly(context, original);
        }
    }

    static JSONObject plugin(CoreInstrumentation test, String name) throws Exception {
        if (!name.matches("[a-z][a-z0-9_]*")) throw new IllegalArgumentException("invalid plugin name");
        NativeCore.initialize(test.getTargetContext());
        JSONObject session = NativeCore.session();
        NativeCore.decode(NativeCore.sync());
        JSONArray plugins = test.request(session.getString("address") + "/admin/plugins", session.getString("token")).getJSONArray("plugins");
        for (int i = 0; i < plugins.length(); i++) {
            JSONObject plugin = plugins.getJSONObject(i);
            if (name.equals(plugin.getString("name")) && plugin.getBoolean("running"))
                return new JSONObject().put("ok", true).put("plugin", name).put("version", plugin.getString("version")).put("protocol", plugin.getInt("protocol_version"));
        }
        throw new AssertionError("plugin Service unavailable: " + name);
    }
}
