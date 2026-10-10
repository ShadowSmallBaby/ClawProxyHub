package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.ContextWrapper;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.zip.*;
import org.json.*;

// 用独立数据目录覆盖全新 setup 和真实 Lua 上传安装，不修改设备已有账号。
final class OnboardingChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context original = test.getTargetContext();
        File root = new File(original.getCacheDir(), "onboarding-" + System.nanoTime());
        if (!root.mkdir()) throw new IOException("Cannot create isolated test data");
        Context isolated = new ContextWrapper(original) {
            @Override public Context getApplicationContext() { return this; }
            @Override public File getFilesDir() { return root; }
        };
        boolean remote = AppPreferences.remoteOnly(original);
        try {
            AppPreferences.setRemoteOnly(original, false);
            NativeCore.decode(NativeCore.stop()); CoreState.started = false;
            NativeCore.context = isolated;
            JSONObject session = NativeCore.session(); String base = session.getString("address");
            if (!session.getString("token").isEmpty()) throw new AssertionError("Fresh installation auto logged in");
            if (test.request(base + "/admin/setup-status", "").getBoolean("initialized")) throw new AssertionError("Setup bypassed");
            test.request(base + "/admin/setup", "", "POST", new JSONObject().put("username", "setup-test").put("password", "test-only-password"));
            String token = test.request(base + "/admin/login", "", "POST", new JSONObject().put("username", "setup-test").put("password", "test-only-password")).getString("token");
            test.request(base + "/admin/me", token);
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            try (ZipOutputStream zip = new ZipOutputStream(bytes)) {
                zip.putNextEntry(new ZipEntry("manifest.json")); zip.write("{\"name\":\"lua-install-check\",\"version\":\"1.0.0\",\"runtime\":\"lua\",\"protocol_version\":2,\"icon\":\"icon.png\"}".getBytes(StandardCharsets.UTF_8)); zip.closeEntry();
                zip.putNextEntry(new ZipEntry("icon.png")); zip.write(android.util.Base64.decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jf2kAAAAASUVORK5CYII=", android.util.Base64.DEFAULT)); zip.closeEntry();
                zip.putNextEntry(new ZipEntry("main.lua")); zip.write("return {}".getBytes(StandardCharsets.UTF_8)); zip.closeEntry();
            }
            if (!"lua-install-check".equals(CoreApi.upload(isolated, bytes.toByteArray()))) throw new AssertionError("Upload identity missing");
            JSONArray local = InstalledPlugins.read(isolated);
            if (local.length() != 1 || !"lua-install-check".equals(local.getJSONObject(0).getString("name"))) throw new AssertionError("Installed Lua package missing from native list: " + local);
            File alias = new File(original.getCacheDir(), "onboarding-alias-" + System.nanoTime());
            android.system.Os.symlink(root.getAbsolutePath(), alias.getAbsolutePath());
            try {
                Context aliased = new ContextWrapper(isolated) { @Override public File getFilesDir() { return alias; } };
                if (InstalledPlugins.read(aliased).length() != 1) throw new AssertionError("System data directory alias hid Lua installation");
            } finally { alias.delete(); }
            JSONArray plugins = test.request(base + "/admin/plugins", token).getJSONArray("plugins");
            if (plugins.length() != 1 || !plugins.getJSONObject(0).getBoolean("running")) throw new AssertionError("Lua upload did not launch private service");
            java.net.HttpURLConnection icon = (java.net.HttpURLConnection) new java.net.URL(base + plugins.getJSONObject(0).getString("icon")).openConnection();
            try { if (icon.getResponseCode() != 200 || icon.getContentLength() <= 0) throw new AssertionError("Android workspace plugin icon unavailable"); }
            finally { icon.disconnect(); }
            if (!CoreApi.localSettings().getBoolean("plugin_lua_enabled")) throw new AssertionError("Native Lua settings unavailable after setup");
            RuntimePackages runtime = new RuntimePackages(isolated);
            JSONObject runtimeSettings = runtime.request(new JSONObject().put("operation", "settings").put("id", "lua-runtime"));
            if (!runtimeSettings.getJSONObject("values").getBoolean("isolation")) throw new AssertionError("Runtime isolation setting missing");
            runtime.request(new JSONObject().put("operation", "disable").put("id", "lua-runtime"));
            if (CoreApi.localSettings().getBoolean("plugin_lua_enabled") || CoreApi.pluginStatus().getJSONArray("plugins").getJSONObject(0).getBoolean("running")) throw new AssertionError("Native Lua switch did not stop script");
            runtime.request(new JSONObject().put("operation", "enable").put("id", "lua-runtime"));
            if (new File(root, "hosts/luahost-android-arm64").exists()) throw new AssertionError("Desktop luahost created");
            NativeCore.decode(NativeCore.stop()); CoreState.started = false;
            session = NativeCore.session();
            if (!test.request(session.getString("address") + "/admin/setup-status", "").getBoolean("initialized")) throw new AssertionError("Setup lost after restart");
            return new JSONObject().put("ok", true).put("checks", "fresh setup, chosen credentials, Lua package upload and service handshake, restart persistence");
        } finally {
            NativeCore.decode(NativeCore.stop()); CoreState.started = false; NativeCore.currentSession = null;
            NativeCore.context = original; AppPreferences.setRemoteOnly(original, remote);
            clean(root);
        }
    }
    private static void clean(File file) {
        File[] children = file.listFiles(); if (children != null) for (File child : children) clean(child);
        file.delete();
    }
}
