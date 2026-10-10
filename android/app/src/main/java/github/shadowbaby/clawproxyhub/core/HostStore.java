package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Map;
import org.json.JSONObject;

// 原生页复用核心的 Lua Host 安装状态，服务只加载已校验且可用的运行时。
public final class HostStore {
    private final Context context;
    private final File root;
    public HostStore(Context context) {
        this.context = context.getApplicationContext();
        root = new File(context.getFilesDir(), "extensions");
    }
    JSONObject current() throws Exception {
        File state = new File(root, "state.json");
        if (!state.isFile()) return null;
        try (InputStream input = new FileInputStream(state)) {
            return new JSONObject(new String(NativePluginStore.read(input, 4 * 1024 * 1024), StandardCharsets.UTF_8)).optJSONObject("lua-runtime");
        }
    }
    private File directory(String digest) {
        if (!digest.matches("[0-9a-f]{64}")) throw new SecurityException("Invalid host identity");
        return new File(context.getFilesDir(), "hosts/versions/lua-runtime/" + digest);
    }
    boolean enabled() {
        try { JSONObject state = current(); return state != null && state.optBoolean("enabled") && state.optBoolean("available"); }
        catch (Exception error) { return false; }
    }
    public String library() throws Exception {
        JSONObject state = current();
        if (state == null || !state.optBoolean("enabled") || !state.optBoolean("available")) return null;
        String digest = state.getString("hash");
        File directory = directory(digest), archive = new File(directory, "package.cphhost");
        return library(archive);
    }
    public String library(File archive) throws Exception {
        String digest;
        try (InputStream input = new FileInputStream(archive)) {
            digest = NativePluginStore.hash(NativePluginStore.read(input, 128 * 1024 * 1024));
        }
        File directory = directory(digest);
        if (!archive.getCanonicalFile().equals(new File(directory, "package.cphhost").getCanonicalFile())) throw new SecurityException("Host package identity changed");
        Map<String, byte[]> files = new NativePluginStore(context).verify(archive, true);
        JSONObject manifest = new JSONObject(new String(files.get("manifest.json"), StandardCharsets.UTF_8));
        String path = manifest.getJSONObject("android").getString("library");
        File library = new File(directory, path);
        try (InputStream input = new FileInputStream(library)) {
            if (!MessageDigest.isEqual(files.get(path), NativePluginStore.read(input, 128 * 1024 * 1024))) throw new SecurityException("Host library changed");
        }
        return library.getAbsolutePath();
    }
}
