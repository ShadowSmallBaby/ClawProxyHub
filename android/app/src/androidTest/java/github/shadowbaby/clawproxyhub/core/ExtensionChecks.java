package github.shadowbaby.clawproxyhub.core;

import android.content.ComponentName;
import android.content.Context;
import android.content.pm.ServiceInfo;
import android.os.Build;
import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;
import org.json.JSONArray;
import org.json.JSONObject;

// 完整签名包在模拟器中验证 JNI、动作、分块草稿和进程回收；不覆盖已有编辑器安装。
final class ExtensionChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        boolean remoteOnly = AppPreferences.remoteOnly(context), installed = false;
        String name = "ext-check-" + Long.toHexString(System.nanoTime());
        String base = "", token = "";
        try {
            AppPreferences.setRemoteOnly(context, false);
            NativeCore.initialize(context);
            JSONObject session = NativeCore.session();
            base = session.getString("address"); token = session.getString("token");
            if (token.isEmpty()) {
                if (!Build.HARDWARE.contains("ranchu") && !Build.HARDWARE.contains("goldfish"))
                    throw new IllegalStateException("Complete local workspace setup before testing on a physical device");
                test.request(base + "/admin/setup", "", "POST", new JSONObject().put("username", "extension-test").put("password", java.util.UUID.randomUUID().toString()));
                session = NativeCore.session(); token = session.getString("token");
            }
            JSONArray before = test.request(base + "/admin/extensions", token).getJSONArray("extensions");
            for (int i = 0; i < before.length(); i++) if ("lua-editor".equals(before.getJSONObject(i).getJSONObject("manifest").getString("id")))
                throw new IllegalStateException("Use an emulator without a Lua editor installation for this fixture");
            for (int i = 0; i < 8; i++) {
                ServiceInfo service = context.getPackageManager().getServiceInfo(new ComponentName(context,
                        "github.shadowbaby.clawproxyhub.extension.ExtensionService$Worker" + i), 0);
                if (service.exported || !service.processName.equals(context.getPackageName() + ":extension" + i)) throw new AssertionError("Extension service must be private");
            }
            byte[] archive;
            try (InputStream input = test.getContext().getAssets().open("lua-editor.cphext")) { archive = NativeCore.readBounded(input, 128 * 1024 * 1024); }
            JSONObject manifest = manifest(archive);
            JSONObject preview = test.multipart(base + "/admin/extensions/inspect", token, "package", archive);
            if (!"lua-editor".equals(preview.getJSONObject("manifest").getString("id"))) throw new AssertionError("Unexpected inspected extension");
            JSONObject state = test.multipart(base + "/admin/extensions/install", token, "package", archive, manifest.getJSONArray("permissions"));
            installed = true;
            if (!state.getBoolean("available")) throw new AssertionError("Android extension did not activate: " + state.optString("error"));
            String source = "local PLUGIN_NAME = '" + name + "'\nlocal PLUGIN_LABEL_ZH = 'Android 本地编辑器'\nreturn {}";
            JSONObject analyzed = action(test, base, token, "analyze", new JSONObject().put("content", source));
            if (!analyzed.getBoolean("valid") || !name.equals(analyzed.getString("name"))) throw new AssertionError("JNI source analysis failed");
            if (action(test, base, token, "analyze", new JSONObject().put("content", source + "\nfunction incomplete(")).getBoolean("valid")) throw new AssertionError("Invalid syntax was accepted");
            action(test, base, token, "create", new JSONObject().put("name", name).put("label", "Android editor test").put("content", source));
            String saved = source + "\n-- saved through shared workspace action";
            action(test, base, token, "save", new JSONObject().put("name", name).put("file", "main.lua").put("content", saved));
            if (!saved.equals(action(test, base, token, "read", new JSONObject().put("name", name).put("file", "main.lua")).getString("content"))) throw new AssertionError("Android source save/read failed");
            StringBuilder draft = new StringBuilder(source);
            for (int i = 0; i < 10000; i++) draft.append("\n-- 草稿");
            JSONObject draftResult = action(test, base, token, "draft-save", new JSONObject().put("name", name).put("revision", "")
                    .put("content", draft.toString()).put("base_sha256", analyzed.getString("sha256")));
            String revision = draftResult.getString("revision");
            boolean stale = false;
            try { action(test, base, token, "draft-delete", new JSONObject().put("name", name).put("revision", "")); }
            catch (IllegalStateException expected) { stale = true; }
            if (!stale) throw new AssertionError("Stale draft deletion succeeded");
            // 超过槽位总数，确保每次停用都真正释放原生进程。
            for (int i = 0; i < 9; i++) {
                enabled(test, base, token, false);
                boolean blocked = false;
                try { action(test, base, token, "analyze", new JSONObject().put("content", source)); }
                catch (IllegalStateException expected) { blocked = true; }
                if (!blocked) throw new AssertionError("Disabled extension still accepts actions");
                enabled(test, base, token, true);
            }
            enabled(test, base, token, false);
            String platform = Build.SUPPORTED_64_BIT_ABIS[0].equals("arm64-v8a") ? "android/arm64" : "android/amd64";
            String entry = manifest.getJSONObject("backend").getJSONObject("entries").getString(platform);
            File library = new File(context.getFilesDir(), "extensions/versions/lua-editor/" + state.getString("hash") + "/" + entry);
            byte[] original;
            try (InputStream input = new FileInputStream(library)) { original = NativeCore.readBounded(input, 128 * 1024 * 1024); }
            if (!library.setWritable(true, true)) throw new AssertionError("Cannot prepare tamper fixture");
            try {
                byte[] changed = original.clone(); changed[0] ^= 1;
                try (FileOutputStream output = new FileOutputStream(library)) { output.write(changed); }
                boolean rejected = false;
                try { enabled(test, base, token, true); } catch (IllegalStateException expected) { rejected = true; }
                if (!rejected) throw new AssertionError("Tampered native library loaded");
            } finally {
                try (FileOutputStream output = new FileOutputStream(library)) { output.write(original); }
                if (!library.setReadOnly()) throw new AssertionError("Cannot restore library permissions");
            }
            enabled(test, base, token, true);
            NativeCore.decode(NativeCore.stop()); CoreState.started = false;
            session = NativeCore.session(); base = session.getString("address"); token = session.getString("token");
            JSONObject restored = action(test, base, token, "draft-read", new JSONObject().put("name", name)).getJSONObject("draft");
            if (!draft.toString().equals(restored.getString("content")) || !revision.equals(restored.getString("revision"))) throw new AssertionError("Draft did not survive core restart");
            JSONArray states = test.request(base + "/admin/extensions", token).getJSONArray("extensions");
            JSONArray contributions = ExtensionContributions.list(states, "plugins.item.actions", true);
            if (contributions.length() == 0 || !ExtensionContributions.route(contributions.getJSONObject(0), name).contains("name=" + name)) throw new AssertionError("Native editor contribution is missing");
            workspaceUI(test, contributions.getJSONObject(0), name, token);
            long savedDeadline = android.os.SystemClock.elapsedRealtime() + 15000;
            boolean savedFromPage = false;
            while (android.os.SystemClock.elapsedRealtime() < savedDeadline) {
                if (draft.toString().equals(action(test, base, token, "read", new JSONObject().put("name", name).put("file", "main.lua")).getString("content"))
                        && !action(test, base, token, "draft-read", new JSONObject().put("name", name)).getBoolean("found")) { savedFromPage = true; break; }
                Thread.sleep(100);
            }
            if (!savedFromPage) throw new AssertionError("Android editor page did not save the recovered draft");
            return new JSONObject().put("ok", true).put("platform", platform).put("checks", new JSONArray()
                    .put("signed-install").put("private-Service-JNI").put("source-analysis").put("workspace-save-read")
                    .put("chunked-drafts").put("stale-write-rejection").put("nine-worker-restarts")
                    .put("tampered-library-rejection").put("core-restart").put("native-contributions").put("native-WebView-save"));
        } finally {
            if (installed && !base.isEmpty()) {
                try { test.request(base + "/admin/plugins/" + name, token, "DELETE", null); } catch (Exception ignored) { }
                try { test.request(base + "/admin/extensions/lua-editor", token, "DELETE", null); } catch (Exception ignored) { }
                try { test.request(base + "/admin/extensions/lua-editor/data", token, "DELETE", null); } catch (Exception ignored) { }
            }
            NativeCore.decode(NativeCore.stop()); CoreState.started = false;
            AppPreferences.setRemoteOnly(context, remoteOnly);
        }
    }
    // 从原生贡献入口打开真实 WebView，保存按钮必须由沙箱完成初始化后启用。
    private static void workspaceUI(CoreInstrumentation test, JSONObject contribution, String name, String token) throws Exception {
        Context context = test.getTargetContext();
        android.content.SharedPreferences workspaces = context.getSharedPreferences("workspaces", 0), application = context.getSharedPreferences("application", 0);
        java.util.Map<String, ?> savedWorkspaces = workspaces.getAll(), savedApplication = application.getAll();
        android.app.Instrumentation.ActivityMonitor monitor = test.addMonitor(MainActivity.class.getName(), null, false);
        android.app.Activity activity = null;
        try {
            AppPreferences.appearance(context, "en", "light");
            application.edit().putBoolean("system-notifications", false).commit();
            WorkspaceStore store = new WorkspaceStore(context);
            store.select(WorkspaceStore.LOCAL); store.saveToken(WorkspaceStore.LOCAL, token);
            try (android.os.ParcelFileDescriptor launch = test.getUiAutomation().executeShellCommand("am start -W -f 0x10008000 -n github.shadowbaby.clawproxyhub.core/.MainActivity");
                 InputStream input = new android.os.ParcelFileDescriptor.AutoCloseInputStream(launch)) { NativeCore.readBounded(input, 4096); }
            activity = test.waitForMonitorWithTimeout(monitor, 15000);
            if (activity == null) throw new AssertionError("Android editor window did not open");
            MainActivity target = (MainActivity) activity;
            test.runOnMainSync(() -> target.openExtension(contribution, name));
            java.lang.reflect.Field field = MainActivity.class.getDeclaredField("web"); field.setAccessible(true);
            String ready = "location.hash.includes('/extensions/lua-editor/editor?') && document.querySelector('iframe')?.getAttribute('sandbox') === 'allow-scripts' && [...document.querySelectorAll('.extension-topbar button')].some(button => button.textContent.trim() === 'Save' && !button.disabled)";
            long deadline = android.os.SystemClock.elapsedRealtime() + 20000;
            boolean initialized = false;
            while (android.os.SystemClock.elapsedRealtime() < deadline) {
                if (evaluate(test, target, field, ready)) { initialized = true; break; }
                Thread.sleep(100);
            }
            if (!initialized) throw new AssertionError("Android sandbox editor did not initialize");
            if (!evaluate(test, target, field, "(() => { const button = [...document.querySelectorAll('.extension-topbar button')].find(button => button.textContent.trim() === 'Save'); button.click(); return true })()"))
                throw new AssertionError("Android editor save button was unavailable");
            // 等待保存提示再关页面，确保动作和草稿清理不被页面销毁取消。
            deadline = android.os.SystemClock.elapsedRealtime() + 15000;
            while (android.os.SystemClock.elapsedRealtime() < deadline) {
                if (evaluate(test, target, field, "document.querySelector('.extension-status')?.textContent === 'Saved'")) return;
                Thread.sleep(100);
            }
            throw new AssertionError("Android editor did not confirm saving");
        } finally {
            test.removeMonitor(monitor);
            if (activity != null) { android.app.Activity target = activity; test.runOnMainSync(target::finish); }
            WorkspaceChecks.restore(workspaces, savedWorkspaces); WorkspaceChecks.restore(application, savedApplication);
        }
    }
    private static boolean evaluate(CoreInstrumentation test, MainActivity activity, java.lang.reflect.Field field, String script) throws Exception {
        java.util.concurrent.atomic.AtomicReference<String> value = new java.util.concurrent.atomic.AtomicReference<>();
        java.util.concurrent.CountDownLatch done = new java.util.concurrent.CountDownLatch(1);
        test.runOnMainSync(() -> {
            try {
                android.webkit.WebView web = (android.webkit.WebView) field.get(activity);
                if (web == null) { done.countDown(); return; }
                web.evaluateJavascript(script, result -> { value.set(result); done.countDown(); });
            } catch (IllegalAccessException error) { done.countDown(); }
        });
        return done.await(2, java.util.concurrent.TimeUnit.SECONDS) && "true".equals(value.get());
    }
    private static JSONObject manifest(byte[] archive) throws Exception {
        try (ZipInputStream zip = new ZipInputStream(new ByteArrayInputStream(archive))) {
            ZipEntry entry;
            while ((entry = zip.getNextEntry()) != null) if (entry.getName().equals("manifest.json"))
                return new JSONObject(new String(NativeCore.readBounded(zip, 1024 * 1024), StandardCharsets.UTF_8));
        }
        throw new IllegalStateException("Missing extension manifest");
    }
    private static JSONObject action(CoreInstrumentation test, String base, String token, String action, JSONObject input) throws Exception {
        return test.request(base + "/admin/actions/lua-editor." + action, token, "POST", input);
    }
    private static void enabled(CoreInstrumentation test, String base, String token, boolean enabled) throws Exception {
        test.request(base + "/admin/extensions/lua-editor/enabled", token, "PUT", new JSONObject().put("enabled", enabled));
    }
}
