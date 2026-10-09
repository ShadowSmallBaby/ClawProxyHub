package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.Locale;
import java.util.TimeZone;
import java.util.UUID;

// 分离后台任务的准备、只读检查和清理；检查不启动核心或主动执行任务。
final class BackgroundChecks {
    static JSONObject run(CoreInstrumentation test, String phase) throws Exception {
        Context context = test.getTargetContext();
        File marker = new File(context.getFilesDir(), "p5-background-check.json");
        if ("inspect".equals(phase)) return inspect(context, load(marker));
        if (!"prepare".equals(phase) && !"cleanup".equals(phase))
            throw new IllegalArgumentException("background must be prepare, inspect or cleanup");
        if ("prepare".equals(phase) && marker.exists())
            throw new IllegalStateException("Inspect and clean up the previous background check first");

        NativeCore.initialize(context);
        JSONObject session = NativeCore.session();
        String base = session.getString("address"), token = session.getString("token");
        if ("cleanup".equals(phase)) {
            JSONObject state = load(marker);
            test.request(base + "/admin/task-rules/" + state.getLong("rule_id"), token, "DELETE", null);
            test.request(base + "/admin/plugins/" + state.getString("name"), token, "DELETE", null);
            if (!marker.delete()) throw new IllegalStateException("Cannot remove test marker");
            return new JSONObject().put("cleaned", true);
        }
        if (!NativeCore.hasLua()) throw new AssertionError("Lua Service is unavailable");
        String name = "p5-background-" + UUID.randomUUID().toString().replace("-", "").substring(0, 16);
        String summary = name + "-completed";
        String source = "local PLUGIN_NAME = \"" + name + "\"\nlocal M={}\n"
                + "function M.task(req) cph.log.info('" + name + "-started'); "
                + "cph.time.sleep(30000); return {summary='" + summary + "'} end\nreturn M";
        test.request(base + "/admin/actions/core.workspace.create", token, "POST",
                new JSONObject().put("name", name).put("content", source));
        test.request(base + "/admin/actions/core.workspace.reload", token, "POST", new JSONObject().put("name", name));
        long pluginID = 0;
        JSONArray plugins = test.request(base + "/admin/plugins", token).getJSONArray("plugins");
        for (int i = 0; i < plugins.length(); i++) {
            JSONObject plugin = plugins.getJSONObject(i);
            if (name.equals(plugin.getString("name")) && plugin.getBoolean("running")) pluginID = plugin.getLong("id");
        }
        if (pluginID == 0) throw new AssertionError("Background check Lua plugin did not start");
        SimpleDateFormat format = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.ROOT);
        format.setTimeZone(TimeZone.getTimeZone("UTC"));
        String due = format.format(new Date(System.currentTimeMillis() + 90000));
        long ruleID = test.request(base + "/admin/task-rules", token, "POST", new JSONObject()
                .put("plugin_id", pluginID).put("capability_id", "echo").put("trigger_type", "once")
                .put("trigger_value", due).put("target_scope", "global")).getLong("id");
        JSONObject state = new JSONObject().put("name", name).put("rule_id", ruleID)
                .put("plugin_id", pluginID).put("due", due).put("expected_summary", summary);
        try (FileOutputStream output = new FileOutputStream(marker)) {
            output.write(state.toString().getBytes(StandardCharsets.UTF_8));
            output.getFD().sync();
        }
        TaskScheduler.schedule(context);
        // 只初始化未来的触发时刻；该次调用不能提前执行测试任务。
        NativeCore.decode(NativeCore.due(NativeCore.newJob()));
        NativeCore.decode(NativeCore.stop());
        return state.put("prepared", true);
    }

    private static JSONObject load(File marker) throws Exception {
        byte[] bytes;
        try (FileInputStream input = new FileInputStream(marker)) { bytes = NativeCore.readBounded(input, 4096); }
        JSONObject state = new JSONObject(new String(bytes, StandardCharsets.UTF_8));
        if (!state.getString("name").matches("p5-background-[0-9a-f]{16}"))
            throw new SecurityException("Unexpected test marker identity");
        return state;
    }

    private static JSONObject inspect(Context context, JSONObject state) throws Exception {
        File database = new File(context.getFilesDir(), "cph.db");
        JSONArray runs = new JSONArray();
        try (SQLiteDatabase db = SQLiteDatabase.openDatabase(database.getAbsolutePath(), null, SQLiteDatabase.OPEN_READONLY);
             Cursor cursor = db.rawQuery("SELECT status, summary, error_message, started_at, finished_at FROM task_runs WHERE rule_id=? ORDER BY id",
                     new String[]{Long.toString(state.getLong("rule_id"))})) {
            while (cursor.moveToNext()) runs.put(new JSONObject().put("status", cursor.getString(0))
                    .put("summary", cursor.getString(1)).put("error", cursor.getString(2))
                    .put("started_at", cursor.getString(3)).put("finished_at", cursor.getString(4)));
        }
        state.put("runs", runs);
        boolean passed = runs.length() == 1 && "success".equals(runs.getJSONObject(0).getString("status"))
                && state.getString("expected_summary").equals(runs.getJSONObject(0).getString("summary"));
        if (!passed) throw new AssertionError(state.toString());
        return state.put("passed", true).put("read_only", true);
    }
}
