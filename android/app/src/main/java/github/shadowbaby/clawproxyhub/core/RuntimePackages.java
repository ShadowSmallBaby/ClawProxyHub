package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.net.Uri;
import android.os.Build;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import org.json.*;

// 原生运行时操作通过私有 JNI 入口复用核心安装状态、依赖检查与候选运行检测。
final class RuntimePackages {
    private final Context context;
    RuntimePackages(Context context) { this.context = context.getApplicationContext(); }

    JSONObject request(JSONObject input) throws Exception {
        NativeCore.initialize(context); NativeCore.session();
        return NativeCore.decode(NativeCore.runtime(input.toString()));
    }

    String install(Uri uri, PluginMarket.Progress progress) throws Exception {
        byte[] data;
        try (InputStream input = context.getContentResolver().openInputStream(uri)) { data = NativePluginStore.read(input, 128 * 1024 * 1024); }
        return install(data, null, progress);
    }

    private String install(byte[] data, JSONObject expected, PluginMarket.Progress progress) throws Exception {
        progress.update("verifying", data.length, data.length);
        File file = File.createTempFile("runtime-", ".cphhost", context.getCacheDir());
        try {
            try (FileOutputStream output = new FileOutputStream(file)) { output.write(data); }
            Map<String, byte[]> files = new NativePluginStore(context).verify(file, true);
            JSONObject manifest = new JSONObject(new String(files.get("manifest.json"), StandardCharsets.UTF_8));
            if (expected != null && (!expected.getString("version").equals(manifest.getString("version"))
                    || !expected.getString("id").equals(manifest.getString("id")))) throw new SecurityException("Runtime differs from the release index");
            JSONObject inspected = request(new JSONObject().put("operation", "inspect").put("file", file.getAbsolutePath()));
            progress.update("installing", 0, -1);
            request(new JSONObject().put("operation", "install").put("file", file.getAbsolutePath())
                    .put("sha256", inspected.getString("sha256")).put("grants", inspected.getJSONObject("manifest").getJSONArray("permissions")));
            return "Lua Host";
        } finally { file.delete(); }
    }

    static JSONObject select(JSONObject index, String[] abis, String coreVersion, String repository) throws Exception {
        String tag = AppUpdates.releaseTag(index, repository);
        JSONArray packages = index.getJSONArray("packages");
        for (String abi : abis) {
            String platform = "arm64-v8a".equals(abi) ? "android/arm64" : "x86_64".equals(abi) ? "android/amd64" : "";
            if (platform.isEmpty()) continue;
            for (int i = 0; i < packages.length(); i++) {
                JSONObject item = packages.getJSONObject(i);
                if (!"lua-runtime".equals(item.optString("id")) || !"runtime".equals(item.optString("kind")) || !"backend".equals(item.optString("target"))) continue;
                JSONArray platforms = item.optJSONArray("platforms");
                boolean supported = false;
                if (platforms != null) for (int j = 0; j < platforms.length(); j++) supported |= platform.equals(platforms.getString(j));
                if (!supported) continue;
                JSONObject core = item.getJSONObject("core");
                if (core.has("min") && FrontendPackage.compare(coreVersion, core.getString("min")) < 0
                        || core.has("max_exclusive") && FrontendPackage.compare(coreVersion, core.getString("max_exclusive")) >= 0) continue;
                String version = item.getString("version");
                FrontendPackage.compare(version, version);
                String name = "luahost-" + version + "-android-" + abi + ".cphhost";
                if (!name.equals(item.getString("name")) || !(repository + "/releases/download/" + tag + "/" + name).equals(item.getString("download_url"))
                        || !item.getString("sha256").matches("[0-9a-f]{64}") || item.getLong("size") < 1 || item.getLong("size") > 128 * 1024 * 1024)
                    throw new SecurityException("Invalid runtime release artifact");
                return new JSONObject(item.toString());
            }
        }
        return null;
    }

    JSONObject latest() throws Exception {
        return select(AppUpdates.latest(context), Build.SUPPORTED_64_BIT_ABIS, BuildConfig.CORE_VERSION, BuildConfig.UPDATE_REPOSITORY);
    }

    String installRelease(JSONObject item, PluginMarket.Progress progress) throws Exception {
        byte[] data = PluginMarket.download(context, item.getString("download_url"), (int) item.getLong("size"), progress);
        if (data.length != item.getLong("size") || !NativePluginStore.hash(data).equals(item.getString("sha256")))
            throw new SecurityException("Runtime package checksum or size mismatch");
        return install(data, item, progress);
    }
}
