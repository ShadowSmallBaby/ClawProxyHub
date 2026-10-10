package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import org.json.JSONObject;

// 原生插件管理复用核心动作与校验，不直接修改业务数据库。
final class CoreApi {
    static String upload(Context context, byte[] archive) throws Exception {
        return uploadPackage(context, archive).getString("installed");
    }
    private static JSONObject uploadPackage(Context context, byte[] archive) throws Exception {
        NativeCore.initialize(context); JSONObject session = NativeCore.session();
        if (session.getString("token").isEmpty()) throw new IllegalStateException("请先在本地工作台完成账号设置 / Complete local workspace setup first");
        String boundary = "cph-" + java.util.UUID.randomUUID();
        HttpURLConnection connection = (HttpURLConnection) new URL(session.getString("address") + "/admin/plugins/install-upload").openConnection();
        connection.setRequestMethod("POST"); connection.setConnectTimeout(5000); connection.setReadTimeout(30000); connection.setDoOutput(true);
        connection.setRequestProperty("Authorization", "Bearer " + session.getString("token"));
        connection.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
        try {
            try (java.io.OutputStream output = connection.getOutputStream()) {
                output.write(("--" + boundary + "\r\nContent-Disposition: form-data; name=\"package\"; filename=\"plugin.cphplugin\"\r\nContent-Type: application/octet-stream\r\n\r\n").getBytes(StandardCharsets.UTF_8));
                output.write(archive); output.write(("\r\n--" + boundary + "--\r\n").getBytes(StandardCharsets.UTF_8));
            }
            int status = connection.getResponseCode();
            try (java.io.InputStream input = status >= 400 ? connection.getErrorStream() : connection.getInputStream()) {
                byte[] data = NativePluginStore.read(input, 2 * 1024 * 1024);
                if (status >= 400) throw new IllegalStateException(new String(data, StandardCharsets.UTF_8));
                return new JSONObject(new String(data, StandardCharsets.UTF_8));
            }
        } finally { connection.disconnect(); }
    }
    static JSONObject call(Context context, String method, String path, JSONObject body) throws Exception {
        NativeCore.initialize(context);
        JSONObject session = NativeCore.session();
        if (session.getString("token").isEmpty()) throw new IllegalStateException("请先在本地工作台完成账号设置 / Complete local workspace setup first");
        return call(session, method, path, body, 30000);
    }
    static JSONObject pluginStatus() throws Exception {
        JSONObject session = NativeCore.currentSession;
        if (!CoreState.started || session == null || session.optString("token").isEmpty()) throw new IllegalStateException("Core not ready");
        return call(session, "GET", "/admin/plugins", null, 3000);
    }
    static JSONObject extensionStatus() throws Exception {
        JSONObject session = NativeCore.currentSession;
        if (!CoreState.started || session == null || session.optString("token").isEmpty()) throw new IllegalStateException("Core not ready");
        return call(session, "GET", "/admin/extensions", null, 3000);
    }
    static JSONObject localSettings() throws Exception {
        JSONObject session = NativeCore.currentSession;
        if (!CoreState.started || session == null || session.optString("token").isEmpty()) return null;
        return call(session, "GET", "/admin/settings", null, 3000).getJSONObject("settings");
    }
    static JSONObject notifications() throws Exception {
        JSONObject session = NativeCore.currentSession;
        if (!CoreState.started || session == null || session.optString("token").isEmpty()) return null;
        return call(session, "GET", "/admin/notifications?limit=200", null, 3000);
    }
    private static JSONObject call(JSONObject session, String method, String path, JSONObject body, int timeout) throws Exception {
        HttpURLConnection connection = (HttpURLConnection) new URL(session.getString("address") + path).openConnection();
        connection.setRequestMethod(method); connection.setConnectTimeout(Math.min(5000, timeout)); connection.setReadTimeout(timeout);
        connection.setRequestProperty("Authorization", "Bearer " + session.getString("token"));
        if (body != null) {
            connection.setDoOutput(true); connection.setRequestProperty("Content-Type", "application/json");
            try (java.io.OutputStream output = connection.getOutputStream()) { output.write(body.toString().getBytes(StandardCharsets.UTF_8)); }
        }
        try {
            int code = connection.getResponseCode();
            java.io.InputStream stream = code >= 400 ? connection.getErrorStream() : connection.getInputStream();
            byte[] data; try (java.io.InputStream input = stream) { data = NativeCore.readBounded(input, 8 * 1024 * 1024); }
            String text = new String(data, StandardCharsets.UTF_8);
            if (code >= 400) throw new IllegalStateException(text);
            return text.isEmpty() ? new JSONObject() : new JSONObject(text);
        } finally { connection.disconnect(); }
    }
}
