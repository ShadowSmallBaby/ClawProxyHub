package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.net.Uri;
import android.os.Build;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.*;
import java.util.zip.*;
import org.json.*;

// 市场目录与核心运行状态独立；下载内容必须再次通过平台、摘要及宿主签名校验。
final class PluginMarket {
    interface Progress { void update(String phase, long received, long total); }
    private static final Progress SILENT = (phase, received, total) -> {};
    private static final String ROOT = "https://raw.githubusercontent.com/ShadowSmallBaby/ClawProxyHubPlugins/main/";
    private final Context context;
    private JSONArray entries = new JSONArray();
    PluginMarket(Context context) { this.context = context.getApplicationContext(); }
    synchronized JSONArray load(boolean online) throws Exception {
        try (InputStream input = context.getAssets().open("plugin-market.json")) { entries = parse(NativePluginStore.read(input, 4 * 1024 * 1024)); }
        File cache = new File(context.getCacheDir(), "plugin-market.json");
        if (cache.isFile()) try (InputStream input = new FileInputStream(cache)) { entries = parse(NativePluginStore.read(input, 4 * 1024 * 1024)); } catch (Exception ignored) { }
        if (online) {
            JSONArray current = parse(download(ROOT + "index.json", 4 * 1024 * 1024));
            entries = current;
            try (FileOutputStream output = new FileOutputStream(cache)) { output.write(entries.toString().getBytes(StandardCharsets.UTF_8)); }
        }
        return decorate(entries);
    }
    private static JSONArray parse(byte[] bytes) throws Exception {
        JSONArray items = new JSONArray(new String(bytes, StandardCharsets.UTF_8));
        if (items.length() > 2048) throw new IOException("Plugin catalog too large");
        for (int i = 0; i < items.length(); i++) if (!items.getJSONObject(i).getString("name").matches("[a-z][a-z0-9_-]{0,63}")) throw new IOException("Invalid catalog entry");
        return items;
    }
    static String unavailableReason(JSONObject entry) {
        return unavailableReason(entry, Build.SUPPORTED_64_BIT_ABIS);
    }
    static String unavailableReason(JSONObject entry, String[] abis) {
        String runtime = entry.optString("runtime");
        boolean lua = "lua".equals(runtime);
        if (!runtime.isEmpty() && !"go".equals(runtime) && !lua) return "metadata";
        // Lua 只分发通用脚本包，设备兼容性由已安装的 Lua Host 决定。
        if (lua) return validReference(entry) && (!entry.has("size") || entry.optLong("size") > 0) ? "" : "metadata";
        if (!entry.has("platforms")) return "unpublished";
        JSONObject platforms = entry.optJSONObject("platforms");
        if (platforms == null) return "metadata";
        if (!platforms.has("android")) return "unpublished";
        JSONArray supported = platforms.optJSONArray("android");
        if (supported == null) return "metadata";
        if (supported.length() == 0) return "unpublished";
        boolean matches = false;
        for (int i = 0; i < supported.length(); i++) {
            Object value = supported.opt(i);
            if (!(value instanceof String) || ((String) value).isEmpty()) return "metadata";
            matches |= Arrays.asList(abis).contains(value);
        }
        if (!matches) return "abi";
        return validReference(entry.optJSONObject("release_manifest")) ? "" : "metadata";
    }
    private static boolean validReference(JSONObject reference) {
        if (reference == null || !reference.optString("sha256").matches("[0-9a-f]{64}")) return false;
        try {
            URI uri = new URI(reference.optString("download_url"));
            return "https".equals(uri.getScheme()) && uri.getHost() != null && uri.getUserInfo() == null;
        } catch (URISyntaxException error) { return false; }
    }
    private static JSONArray decorate(JSONArray input) throws Exception {
        JSONArray result = new JSONArray(input.toString());
        for (int i = 0; i < result.length(); i++) {
            JSONObject entry = result.getJSONObject(i);
            String reason = unavailableReason(entry);
            entry.put("installable", reason.isEmpty()).put("unavailable_reason", reason);
        }
        return result;
    }
    synchronized String install(String name, Progress progress) throws Exception {
        JSONObject entry = null;
        for (int i = 0; i < entries.length(); i++) if (name.equals(entries.getJSONObject(i).getString("name"))) entry = entries.getJSONObject(i);
        if (entry == null) throw new IOException("Plugin no longer in catalog");
        if (!unavailableReason(entry).isEmpty()) throw new IOException("No package available for this device");
        boolean lua = "lua".equals(entry.optString("runtime"));
        if (lua && !new HostStore(context).enabled()) throw new IOException("Lua Host 未安装或已停止 / Lua Host is not installed or enabled");
        progress.update("connecting", 0, -1);
        JSONObject artifact;
        if (lua) artifact = scriptArtifact(entry);
        else {
            JSONObject ref = entry.getJSONObject("release_manifest");
            artifact = releaseArtifact(entry, download(ref.getString("download_url"), 1024 * 1024));
        }
        byte[] data = download(artifact.getString("download_url"), 128 * 1024 * 1024, progress);
        progress.update("verifying", data.length, data.length);
        verifyDownload(artifact, data);
        File file = File.createTempFile("market-", ".cphplugin", context.getCacheDir());
        try {
            try (FileOutputStream output = new FileOutputStream(file)) { output.write(data); }
            try (ZipFile zip = new ZipFile(file)) {
                ZipEntry manifest = zip.getEntry("manifest.json");
                if (manifest == null) throw new SecurityException("Package has no manifest");
                JSONObject metadata;
                try (InputStream input = zip.getInputStream(manifest)) { metadata = new JSONObject(new String(NativePluginStore.read(input, 1024 * 1024), StandardCharsets.UTF_8)); }
                verifyPackage(entry, metadata);
            }
            progress.update("installing", 0, -1);
            if (lua) {
                String installed = CoreApi.upload(context, data);
                if (!name.equals(installed)) throw new IOException("Installed plugin differs from catalog");
            }
            else new NativePluginStore(context).install(Uri.fromFile(file));
            return name;
        } finally { file.delete(); }
    }
    static JSONObject scriptArtifact(JSONObject entry) throws Exception {
        if (!"lua".equals(entry.optString("runtime")) || !unavailableReason(entry, new String[0]).isEmpty())
            throw new IOException("Invalid Lua package reference");
        JSONObject artifact = new JSONObject().put("download_url", entry.getString("download_url")).put("sha256", entry.getString("sha256"));
        if (entry.has("size")) artifact.put("size", entry.getLong("size"));
        return artifact;
    }
    static void verifyDownload(JSONObject artifact, byte[] data) throws Exception {
        if (artifact.has("size") && data.length != artifact.getLong("size") || !NativePluginStore.hash(data).equals(artifact.getString("sha256")))
            throw new SecurityException("Downloaded package checksum or size mismatch");
    }
    static void verifyPackage(JSONObject entry, JSONObject metadata) throws Exception {
        String runtime = metadata.optString("runtime", "go");
        if (runtime.isEmpty()) runtime = "go";
        if (!entry.getString("name").equals(metadata.getString("name")) || !entry.getString("version").equals(metadata.getString("version"))
                || !("lua".equals(entry.optString("runtime")) ? "lua" : "go").equals(runtime) || metadata.optInt("protocol_version") != 2)
            throw new SecurityException("Downloaded package differs from catalog or uses an incompatible protocol");
    }
    private byte[] download(String address, int limit) throws Exception {
        return download(address, limit, SILENT);
    }
    static JSONObject releaseArtifact(JSONObject entry, byte[] raw) throws Exception {
        return releaseArtifact(entry, raw, Build.SUPPORTED_64_BIT_ABIS, Build.VERSION.SDK_INT);
    }
    static JSONObject releaseArtifact(JSONObject entry, byte[] raw, String[] abis, int sdk) throws Exception {
        if ("lua".equals(entry.optString("runtime")) || !unavailableReason(entry, abis).isEmpty()) throw new IOException("No native package available for this device");
        if (!NativePluginStore.hash(raw).equals(entry.getJSONObject("release_manifest").getString("sha256"))) throw new SecurityException("Release manifest checksum mismatch");
        JSONObject release = new JSONObject(new String(raw, StandardCharsets.UTF_8));
        if (release.getInt("schema_version") != 1 || !entry.getString("name").equals(release.getString("name"))
                || !entry.getString("version").equals(release.getString("version")) || !"go".equals(release.getString("runtime"))
                || release.getInt("protocol_version") != 2) throw new SecurityException("Incompatible release manifest identity or protocol");
        JSONObject artifacts = release.getJSONObject("artifacts"), selected = null;
        for (String abi : abis) {
            JSONArray supported = entry.getJSONObject("platforms").getJSONArray("android");
            boolean declared = false;
            for (int i = 0; i < supported.length(); i++) declared |= abi.equals(supported.getString(i));
            if (!declared) continue;
            String platform = "arm64-v8a".equals(abi) ? "android-arm64" : "x86_64".equals(abi) ? "android-amd64" : null;
            if (platform == null) continue;
            selected = artifacts.optJSONObject(platform);
            if (selected != null) break;
        }
        if (selected == null || !"cph-native-v1".equals(selected.optString("format"))
                || selected.optInt("min_sdk", 24) > sdk || selected.optLong("size") < 1
                || !validReference(selected)) throw new IOException("No compatible package in release manifest");
        return selected;
    }
    private byte[] download(String address, int limit, Progress progress) throws Exception {
        return download(context, address, limit, progress);
    }
    static byte[] download(Context context, String address, int limit, Progress progress) throws Exception {
        URL url = new URL(AppPreferences.githubURL(context, address));
        for (int redirect = 0; redirect < 6; redirect++) {
            if (!"https".equals(url.getProtocol()) || url.getUserInfo() != null) throw new SecurityException("HTTPS required for plugin downloads");
            HttpURLConnection request = (HttpURLConnection) url.openConnection();
            request.setInstanceFollowRedirects(false); request.setConnectTimeout(10000); request.setReadTimeout(20000);
            try {
                int status = request.getResponseCode();
                if (status >= 300 && status <= 399) { url = new URL(url, request.getHeaderField("Location")); continue; }
                if (status == 404) throw new FileNotFoundException("Catalog not published");
                if (status != 200) throw new IOException("Download failed (HTTP " + status + ")");
                try (InputStream input = request.getInputStream()) { return readDownload(input, request.getContentLengthLong(), limit, progress); }
            } finally { request.disconnect(); }
        }
        throw new IOException("Too many download redirects");
    }
    static byte[] readDownload(InputStream input, long total, int limit, Progress progress) throws Exception {
        if (total > limit) throw new IOException("Plugin package exceeds size limit");
        ByteArrayOutputStream output = new ByteArrayOutputStream(); byte[] buffer = new byte[32768]; int count;
        long last = 0;
        progress.update("downloading", 0, total);
        while ((count = input.read(buffer)) != -1) {
            if (output.size() + count > limit) throw new IOException("Plugin package exceeds size limit");
            output.write(buffer, 0, count);
            long now = android.os.SystemClock.elapsedRealtime();
            if (now - last >= 150) { progress.update("downloading", output.size(), total); last = now; }
        }
        if (total >= 0 && output.size() != total) throw new IOException("Download interrupted; please retry");
        progress.update("downloading", output.size(), total);
        return output.toByteArray();
    }
}
