package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.os.Build;
import java.io.InputStream;
import java.io.IOException;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import org.json.*;

// 应用更新按 applicationId、versionCode 与 ABI 判断，独立于连接核心的版本。
final class AppUpdates {
    private static JSONObject fetch(Context context, String url) throws Exception {
        HttpURLConnection request = (HttpURLConnection) new URL(AppPreferences.githubURL(context, url)).openConnection();
        request.setConnectTimeout(10000); request.setReadTimeout(15000);
        request.setRequestProperty("Accept", "application/json");
        request.setRequestProperty("User-Agent", "ClawProxyHub-Android/" + BuildConfig.VERSION_NAME);
        try {
            if (request.getResponseCode() != 200) throw new IOException("Update check HTTP " + request.getResponseCode());
            try (InputStream input = request.getInputStream()) { return new JSONObject(new String(NativePluginStore.read(input, 2 * 1024 * 1024), StandardCharsets.UTF_8)); }
        } finally { request.disconnect(); }
    }
    static JSONObject check(Context context) throws Exception {
        String repository = BuildConfig.UPDATE_REPOSITORY;
        return select(latest(context),
                BuildConfig.VERSION_CODE, Build.SUPPORTED_64_BIT_ABIS, Build.VERSION.SDK_INT, repository);
    }
    static JSONObject latest(Context context) throws Exception {
        JSONObject manifest = fetch(context, BuildConfig.UPDATE_REPOSITORY + "/releases/latest/download/update-manual.json");
        releaseTag(manifest, BuildConfig.UPDATE_REPOSITORY);
        return manifest;
    }
    static String releaseTag(JSONObject manifest, String repository) throws Exception {
        String tag = manifest.getString("tag");
        if (manifest.getInt("schema_version") != 1 || !tag.matches("v?[0-9]+\\.[0-9]+\\.[0-9]+")
                || !(repository + "/releases/tag/" + tag).equals(manifest.getString("release_url"))) throw new IOException("Invalid update release");
        return tag;
    }
    static JSONObject select(JSONObject manifest, long currentCode, String[] abis, int sdk, String repository) throws Exception {
        String tag = releaseTag(manifest, repository);
        JSONObject android = manifest.getJSONObject("platform").getJSONObject("android");
        long code = android.getLong("version_code");
        String version = android.getString("version");
        if (!"android-app".equals(android.getString("target"))
                || !BuildConfig.APPLICATION_ID.equals(android.getString("application_id")) || code < 1 || code > 2100000000
                || !version.matches("[0-9]+\\.[0-9]+\\.[0-9]+")) throw new IOException("Invalid application update manifest");
        JSONObject result = new JSONObject().put("target", "android-app").put("version", version).put("version_code", code)
                .put("url", "").put("newer", code > currentCode).put("changelog", android.optJSONObject("changelog"));
        if (android.getInt("min_sdk") > sdk) return result;
        JSONObject artifacts = android.getJSONObject("artifacts");
        for (String abi : abis) {
            if (!"arm64-v8a".equals(abi) && !"x86_64".equals(abi)) continue;
            JSONObject artifact = artifacts.optJSONObject(abi);
            if (artifact == null) continue;
            String filename = "ClawProxyHub-" + version + "-" + code + "-android-" + abi + ".apk";
            String expected = repository + "/releases/download/" + tag + "/" + filename;
            if (!filename.equals(artifact.getString("name")) || !expected.equals(artifact.getString("download_url"))
                    || !artifact.getString("sha256").matches("[0-9a-f]{64}") || artifact.getLong("size") < 1) throw new IOException("Invalid Android update artifact");
            return result.put("url", expected).put("sha256", artifact.getString("sha256"));
        }
        return result;
    }
}
