package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.SharedPreferences;
import java.util.Map;
import org.json.*;

// 用实际 Keystore 验证连接隔离、迁移与地址变更，结束后恢复设备原有配置。
final class WorkspaceChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        SharedPreferences storage = context.getSharedPreferences("workspaces", 0), app = context.getSharedPreferences("application", 0);
        Map<String, ?> saved = storage.getAll(), settings = app.getAll();
        try {
            storage.edit().clear().commit();
            WorkspaceStore store = new WorkspaceStore(context);
            String first = store.add("First", "https://one.example/"), second = store.add("Second", "https://two.example");
            store.select(first); store.saveToken(first, "workspace-test-secret");
            String ciphertext = storage.getString("token:" + first, "");
            if (ciphertext.contains("workspace-test-secret") || !"workspace-test-secret".equals(store.token(first))) throw new AssertionError("credential encryption failed");
            storage.edit().putString("token:" + second, ciphertext).commit();
            boolean isolated = false; try { store.token(second); } catch (Exception expected) { isolated = true; }
            if (!isolated) throw new AssertionError("credential copied across workspaces");
            storage.edit().remove("token:" + second).commit();
            store.select(second);
            boolean stale = false; try { store.saveToken(first, "stale"); } catch (SecurityException expected) { stale = true; }
            if (!stale) throw new AssertionError("stale page overwrote credentials");
            store.update(first, "Renamed", "https://changed.example");
            if (!store.token(first).isEmpty()) throw new AssertionError("address change retained credentials");
            for (String address : new String[]{"http://example.com", "https://user:pass@example.com", "https://example.com?q=token", "file:///tmp"}) {
                boolean rejected = false; try { WorkspaceStore.normalize(address); } catch (Exception expected) { rejected = true; }
                if (!rejected) throw new AssertionError("unsafe address accepted");
            }
            store.select(WorkspaceStore.LOCAL); store.remoteOnly(true);
            if (!store.selected().isEmpty()) throw new AssertionError("remote-only selected local");
            store.remoteOnly(false);
            if (!WorkspaceStore.LOCAL.equals(store.selected())) throw new AssertionError("local mode did not restore selection");
            store.saveToken(WorkspaceStore.LOCAL, "local-login-token");
            if (!"local-login-token".equals(new WorkspaceStore(context).token(WorkspaceStore.LOCAL))) throw new AssertionError("local login not retained");
            store.saveToken(WorkspaceStore.LOCAL, "");
            if (!store.token(WorkspaceStore.LOCAL).isEmpty()) throw new AssertionError("local logout not retained");
            storage.edit().clear().commit(); AppPreferences.setRemoteOnly(context, false);
            JSONObject legacy = new JSONObject().put("cph-connections", new JSONArray().put(new JSONObject().put("id", "legacy").put("name", "Migrated").put("baseURL", "https://legacy.example")).toString())
                    .put("cph-active-connection", "legacy").put("cph-token:legacy", "migration-secret");
            store.migrate(legacy, context);
            if (!"migration-secret".equals(store.token(store.selected()))) throw new AssertionError("legacy credential migration failed");
            store.saveToken(store.selected(), "current"); store.migrate(legacy, context);
            if (!"current".equals(store.token(store.selected()))) throw new AssertionError("migration repeated");
            long before = android.os.SystemClock.elapsedRealtime();
            JSONArray installed = InstalledPlugins.read(context);
            long elapsed = android.os.SystemClock.elapsedRealtime() - before;
            if (elapsed > 3000) throw new AssertionError("Local plugin metadata blocked for " + elapsed + " ms");
            java.util.List<Long> progress = new java.util.ArrayList<>();
            byte[] payload = new byte[100000];
            PluginMarket.readDownload(new java.io.ByteArrayInputStream(payload), payload.length, payload.length, (phase, received, total) -> progress.add(received));
            if (progress.size() < 2 || progress.get(0) != 0 || progress.get(progress.size() - 1) != payload.length) throw new AssertionError("Download progress missing");
            boolean truncated = false;
            try { PluginMarket.readDownload(new java.io.ByteArrayInputStream(payload), payload.length + 1, payload.length + 1, (phase, received, total) -> {}); }
            catch (java.io.IOException expected) { truncated = true; }
            if (!truncated) throw new AssertionError("Truncated download reported success");
            ConfigurationChecks.run(test);
            return new JSONObject().put("ok", true).put("installed_count", installed.length()).put("list_ms", elapsed).put("checks", "Keystore encryption, local login/logout, connection isolation, stale writes, address change, remote-only, one-time migration, local plugin list, download progress and truncation");
        } finally { restore(storage, saved); restore(app, settings); }
    }
    static void restore(SharedPreferences storage, Map<String, ?> values) {
        SharedPreferences.Editor edit = storage.edit().clear();
        for (Map.Entry<String, ?> entry : values.entrySet()) {
            Object value = entry.getValue(); String key = entry.getKey();
            if (value instanceof String) edit.putString(key, (String) value);
            else if (value instanceof Boolean) edit.putBoolean(key, (Boolean) value);
            else if (value instanceof Integer) edit.putInt(key, (Integer) value);
            else if (value instanceof Long) edit.putLong(key, (Long) value);
        }
        if (!edit.commit()) throw new AssertionError("Cannot restore test preferences");
    }
}
