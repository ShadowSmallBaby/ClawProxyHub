package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.UUID;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import org.json.JSONArray;
import org.json.JSONObject;

// 工作台目录由原生持有；凭据用设备 Keystore 加密，网页只获得当前连接。
final class WorkspaceStore {
    static final String LOCAL = "android-local-core";
    private final SharedPreferences preferences;
    WorkspaceStore(Context context) { preferences = context.getSharedPreferences("workspaces", 0); }
    synchronized JSONArray list() throws Exception { return new JSONArray(preferences.getString("items", "[]")); }
    synchronized String selected() { return preferences.getString("selected", LOCAL); }
    synchronized void select(String id) throws Exception {
        if (!id.isEmpty() && !LOCAL.equals(id) && find(id) == null) throw new IllegalArgumentException("Workspace not found");
        commit(preferences.edit().putString("selected", id));
    }
    synchronized JSONObject find(String id) throws Exception {
        JSONArray items = list();
        for (int i = 0; i < items.length(); i++) if (id.equals(items.getJSONObject(i).getString("id"))) return items.getJSONObject(i);
        return null;
    }
    static String normalize(String input) throws Exception {
        URI uri = new URI(input.trim());
        String host = uri.getHost(), scheme = uri.getScheme();
        boolean loopback = "127.0.0.1".equals(host) || "localhost".equalsIgnoreCase(host) || "[::1]".equals(host) || "::1".equals(host);
        if (host == null || !("https".equalsIgnoreCase(scheme) || "http".equalsIgnoreCase(scheme) && loopback)
                || uri.getUserInfo() != null || uri.getQuery() != null || uri.getFragment() != null)
            throw new IllegalArgumentException("HTTPS required; HTTP is available only for loopback addresses");
        return uri.toASCIIString().replaceAll("/+$", "");
    }
    synchronized String add(String name, String address) throws Exception {
        String normalized = normalize(address);
        JSONArray items = list();
        for (int i = 0; i < items.length(); i++) if (normalized.equals(items.getJSONObject(i).getString("baseURL"))) return items.getJSONObject(i).getString("id");
        if (items.length() >= 64) throw new IllegalArgumentException("Too many workspaces");
        String id = UUID.randomUUID().toString();
        items.put(new JSONObject().put("id", id).put("name", name.trim().isEmpty() ? normalized : name.trim()).put("baseURL", normalized));
        commit(preferences.edit().putString("items", items.toString()));
        return id;
    }
    synchronized void remove(String id) throws Exception {
        if (LOCAL.equals(id)) throw new IllegalArgumentException("Cannot remove the local workspace");
        JSONArray old = list(), next = new JSONArray();
        for (int i = 0; i < old.length(); i++) if (!id.equals(old.getJSONObject(i).getString("id"))) next.put(old.getJSONObject(i));
        SharedPreferences.Editor edit = preferences.edit().putString("items", next.toString()).remove("token:" + id);
        if (id.equals(selected())) edit.putString("selected", "");
        commit(edit);
    }
    synchronized void update(String id, String name, String address) throws Exception {
        String normalized = normalize(address); JSONArray items = list(); boolean found = false;
        SharedPreferences.Editor edit = preferences.edit();
        for (int i = 0; i < items.length(); i++) {
            JSONObject item = items.getJSONObject(i);
            if (!id.equals(item.getString("id")) && normalized.equals(item.getString("baseURL"))) throw new IllegalArgumentException("Workspace address already saved");
            if (id.equals(item.getString("id"))) {
                if (!normalized.equals(item.getString("baseURL"))) edit.remove("token:" + id);
                item.put("name", name.trim().isEmpty() ? normalized : name.trim()).put("baseURL", normalized); found = true;
            }
        }
        if (!found) throw new IllegalArgumentException("Workspace not found");
        commit(edit.putString("items", items.toString()));
    }
    synchronized void remoteOnly(boolean enabled) throws Exception {
        if (enabled && LOCAL.equals(selected())) select("");
        if (!enabled && selected().isEmpty()) select(LOCAL);
    }
    synchronized String token(String id) throws Exception {
        String encrypted = preferences.getString("token:" + id, "");
        if (encrypted.isEmpty()) return "";
        byte[] raw = Base64.decode(encrypted, Base64.NO_WRAP);
        if (raw.length < 29) throw new IllegalStateException("Invalid stored credential");
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, raw, 0, 12));
        cipher.updateAAD(id.getBytes(StandardCharsets.UTF_8));
        return new String(cipher.doFinal(raw, 12, raw.length - 12), StandardCharsets.UTF_8);
    }
    synchronized void saveToken(String id, String token) throws Exception {
        if (!id.equals(selected()) || id.isEmpty()) throw new SecurityException("Workspace changed");
        storeToken(id, token);
    }
    private void storeToken(String id, String token) throws Exception {
        if ((!LOCAL.equals(id) && find(id) == null) || token.length() > 32768) throw new IllegalArgumentException("Invalid credential target");
        if (token.isEmpty()) { commit(preferences.edit().remove("token:" + id)); return; }
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, key());
        cipher.updateAAD(id.getBytes(StandardCharsets.UTF_8));
        byte[] encrypted = cipher.doFinal(token.getBytes(StandardCharsets.UTF_8)), iv = cipher.getIV();
        byte[] raw = new byte[iv.length + encrypted.length];
        System.arraycopy(iv, 0, raw, 0, iv.length); System.arraycopy(encrypted, 0, raw, iv.length, encrypted.length);
        commit(preferences.edit().putString("token:" + id, Base64.encodeToString(raw, Base64.NO_WRAP)));
    }
    private static synchronized SecretKey key() throws Exception {
        String alias = "cph.workspace.credentials.v1";
        KeyStore keys = KeyStore.getInstance("AndroidKeyStore"); keys.load(null);
        if (!keys.containsAlias(alias)) {
            KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
            generator.generateKey();
        }
        return (SecretKey) keys.getKey(alias, null);
    }
    synchronized void migrate(JSONObject legacy, Context context) throws Exception {
        if (preferences.getBoolean("migrated", false)) return;
        if (legacy != null) {
            JSONArray items = new JSONArray(legacy.optString("cph-connections", "[]"));
            String oldSelected = legacy.optString("cph-active-connection"), nextSelected = selected();
            for (int i = 0; i < Math.min(items.length(), 64); i++) {
                JSONObject item = items.getJSONObject(i); String oldId = item.optString("id");
                if (LOCAL.equals(oldId) || "same-origin".equals(oldId)) continue;
                try {
                    String id = add(item.optString("name"), item.getString("baseURL"));
                    storeToken(id, legacy.optString("cph-token:" + oldId));
                    if (oldId.equals(oldSelected)) nextSelected = id;
                } catch (IllegalArgumentException ignored) { /* 不迁移不符合地址约束的旧记录。 */ }
            }
            select(nextSelected);
            AppPreferences.appearance(context, legacy.optString("cph-locale", AppPreferences.locale(context)), legacy.optString("cph-theme", AppPreferences.theme(context)));
        }
        remoteOnly(AppPreferences.remoteOnly(context));
        commit(preferences.edit().putBoolean("migrated", true));
    }
    private static void commit(SharedPreferences.Editor editor) { if (!editor.commit()) throw new IllegalStateException("Cannot save workspace settings"); }
}
