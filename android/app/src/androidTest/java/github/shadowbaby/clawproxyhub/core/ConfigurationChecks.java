package github.shadowbaby.clawproxyhub.core;

import android.content.*;
import java.io.*;
import java.nio.charset.StandardCharsets;
import org.json.*;

// 运行时、日志与更新选择使用隔离目录，不更改用户的 Host 和诊断记录。
final class ConfigurationChecks {
    static void run(CoreInstrumentation test) throws Exception {
        Context original = test.getTargetContext(); String prefix = "configuration-" + System.nanoTime();
        File root = new File(original.getCacheDir(), prefix); if (!root.mkdir()) throw new IOException("Cannot create fixture directory");
        Context isolated = new ContextWrapper(original) {
            public Context getApplicationContext() { return this; }
            public File getFilesDir() { return root; }
            public SharedPreferences getSharedPreferences(String name, int mode) { return original.getSharedPreferences(prefix + "-" + name, mode); }
        };
        try {
            if (!AppNotifications.system(isolated)) throw new AssertionError("System notifications are not the default");
            JSONObject unread = new JSONObject().put("id", 7).put("created_at", "2026-10-06T12:00:00Z").put("read", false);
            JSONObject read = new JSONObject().put("id", 8).put("created_at", "2026-10-06T12:01:00Z").put("read", true);
            JSONArray messages = new JSONArray().put(unread).put(read);
            if (AppNotifications.pending(messages, java.util.Collections.emptySet()).size() != 1) throw new AssertionError("Read notification was delivered");
            if (!AppNotifications.pending(messages, java.util.Set.of(AppNotifications.identity(unread))).isEmpty()) throw new AssertionError("Duplicate notification was delivered");
            unread.put("created_at", "2026-10-07T12:00:00Z");
            if (AppNotifications.pending(messages, java.util.Set.of("7:2026-10-06T12:00:00Z")).size() != 1) throw new AssertionError("Reused message id hid a new notification");
            unread.put("title", "CPH notification test").put("content", "Isolated test message");
            if (AppNotifications.available(original)) {
                if (!AppNotifications.deliver(isolated, prefix, "Test workspace", messages, 1)) throw new AssertionError("Allowed system notification was not delivered");
                android.app.NotificationManager manager = original.getSystemService(android.app.NotificationManager.class);
                android.service.notification.StatusBarNotification posted = null;
                for (int i = 0; i < 50 && posted == null; i++) {
                    for (android.service.notification.StatusBarNotification item : manager.getActiveNotifications()) if (("workspace-" + prefix).equals(item.getTag())) posted = item;
                    if (posted == null) Thread.sleep(50);
                }
                if (posted == null || !"CPH notification test".contentEquals(posted.getNotification().extras.getCharSequence(android.app.Notification.EXTRA_TITLE))) throw new AssertionError("System notification content missing");
                if (!isolated.getSharedPreferences("notification-history", 0).getStringSet(prefix, java.util.Collections.emptySet()).contains(AppNotifications.identity(unread))) throw new AssertionError("Notification receipt not persisted");
                AppNotifications.deliver(isolated, prefix, "Test workspace", messages, 1);
                unread.put("read", true); AppNotifications.deliver(isolated, prefix, "Test workspace", messages, 0);
            } else if (AppNotifications.deliver(isolated, prefix, "Test workspace", messages, 1)) throw new AssertionError("Denied notifications did not fall back to workspace");
            AppPreferences.githubProxy(isolated, " https://proxy.example/prefix/ ");
            for (String host : new String[]{"github.com", "raw.githubusercontent.com", "api.github.com"}) {
                if (!AppPreferences.githubURL(isolated, "https://" + host + "/file").equals("https://proxy.example/prefix/https://" + host + "/file")) throw new AssertionError("GitHub download proxy not applied");
            }
            if (!AppPreferences.githubURL(isolated, "https://example.com/file").equals("https://example.com/file")) throw new AssertionError("Proxy changed unrelated download");
            for (String invalid : new String[]{"http://proxy.example", "https://user:pass@proxy.example", "https://proxy.example?token=secret", "https://proxy.example/#bad"}) {
                boolean rejected = false; try { AppPreferences.githubProxy(isolated, invalid); } catch (IllegalArgumentException expected) { rejected = true; }
                if (!rejected) throw new AssertionError("Invalid proxy accepted");
            }
            AppPreferences.githubProxy(isolated, "");
            if (!AppPreferences.githubURL(isolated, "https://github.com/file").equals("https://github.com/file")) throw new AssertionError("Clearing proxy did not restore direct download");
            HostStore hosts = new HostStore(isolated);
            if (hosts.library() != null) throw new AssertionError("Uninstalled runtime became available");
            File archive = new File(root, "luahost.cphhost");
            try (InputStream input = test.getContext().getAssets().open("luahost.cphhost"); FileOutputStream output = new FileOutputStream(archive)) { byte[] buffer = new byte[32768]; int count; while ((count = input.read(buffer)) != -1) output.write(buffer, 0, count); }
            java.util.Map<String, byte[]> files = new NativePluginStore(isolated).verify(archive, true);
            byte[] raw; try (InputStream input = new FileInputStream(archive)) { raw = NativePluginStore.read(input, 128 * 1024 * 1024); }
            String hash = NativePluginStore.hash(raw);
            File installed = new File(root, "hosts/versions/lua-runtime/" + hash);
            for (java.util.Map.Entry<String, byte[]> file : files.entrySet()) {
                File output = new File(installed, file.getKey()); output.getParentFile().mkdirs();
                try (FileOutputStream stream = new FileOutputStream(output)) { stream.write(file.getValue()); }
            }
            try (FileOutputStream stream = new FileOutputStream(new File(installed, "package.cphhost"))) { stream.write(raw); }
            JSONObject hostState = new JSONObject().put("hash", hash).put("enabled", true).put("available", true).put("manifest", new JSONObject(new String(files.get("manifest.json"), StandardCharsets.UTF_8)));
            File stateFile = new File(root, "extensions/state.json");
            stateFile.getParentFile().mkdirs();
            try (FileOutputStream stream = new FileOutputStream(stateFile)) { stream.write(new JSONObject().put("lua-runtime", hostState).toString().getBytes(StandardCharsets.UTF_8)); }
            File library = new File(hosts.library());
            if (!library.isFile() || !library.getCanonicalPath().startsWith(installed.getCanonicalPath())) throw new AssertionError("Core-managed Host was not selected");
            if (!library.setWritable(true)) throw new AssertionError("Cannot corrupt test host");
            try (FileOutputStream output = new FileOutputStream(library, true)) { output.write(0); }
            boolean rejected = false; try { hosts.library(); } catch (SecurityException expected) { rejected = true; }
            if (!rejected) throw new AssertionError("Modified Host library was accepted");
            hostState.put("enabled", false).put("available", false);
            try (FileOutputStream stream = new FileOutputStream(stateFile)) { stream.write(new JSONObject().put("lua-runtime", hostState).toString().getBytes(StandardCharsets.UTF_8)); }
            if (hosts.library() != null) throw new AssertionError("Disabled runtime fell back to another library");
            AppLog.write(isolated, "error", "test", "disabled-event");
            if (!AppLog.read(isolated).isEmpty()) throw new AssertionError("Android logs enabled by default");
            AppLog.configure(isolated, true, "warn", 3);
            AppLog.write(isolated, "debug", "test", "hidden-event"); AppLog.write(isolated, "error", "test", "visible-event");
            String log = AppLog.read(isolated); if (!log.contains("visible-event") || log.contains("hidden-event")) throw new AssertionError("Log level filter failed");
            File expired = new File(root, "android-logs/expired.log"); try (FileOutputStream output = new FileOutputStream(expired)) { output.write(1); } expired.setLastModified(System.currentTimeMillis() - 5 * 86400000L);
            AppLog.read(isolated); if (expired.exists()) throw new AssertionError("Expired log retained");
            AppLog.clear(isolated); if (!AppLog.read(isolated).isEmpty()) throw new AssertionError("Log clear failed");
        } finally {
            original.getSystemService(android.app.NotificationManager.class).cancel("workspace-" + prefix, 1);
            original.deleteSharedPreferences(prefix + "-notification-history"); original.deleteSharedPreferences(prefix + "-application"); original.deleteSharedPreferences(prefix + "-android-logs"); original.deleteSharedPreferences(prefix + "-native-plugins"); clean(root);
        }
    }
    private static void clean(File file) { File[] children = file.listFiles(); if (children != null) for (File child : children) clean(child); file.delete(); }
}
