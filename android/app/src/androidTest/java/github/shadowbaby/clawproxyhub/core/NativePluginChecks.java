package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.net.Uri;
import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.util.zip.*;
import org.json.JSONObject;

// 验证宿主管理包的真实加载与生命周期，错误签名不能改变现有安装。
final class NativePluginChecks {
    static JSONObject run(CoreInstrumentation test, String phase, String expected) throws Exception {
        Context context = test.getTargetContext();
        android.content.SharedPreferences preferences = context.getSharedPreferences("native-plugins", 0);
        java.util.Map<String, ?> saved = preferences.getAll();
        boolean fixture = "fixture".equals(phase) || "binder".equals(phase);
        File source = fixture ? File.createTempFile("native-test-", ".cphplugin", context.getCacheDir()) : new File(context.getCacheDir(), "plugin.cphplugin");
        try {
            if (fixture) {
                try (java.io.InputStream input = test.getContext().getAssets().open("androidtest.cphplugin"); FileOutputStream output = new FileOutputStream(source)) {
                    output.write(NativePluginStore.read(input, 128 * 1024 * 1024));
                }
            }
            // 每次仅加载待测包，避免批量验证累积几十个常驻 Go 进程。
            android.content.SharedPreferences.Editor edit = preferences.edit();
            for (String key : saved.keySet()) if (key.startsWith("current:")) edit.putBoolean("enabled:" + key.substring(8), false);
            if (!edit.commit()) throw new AssertionError("Cannot isolate plugin verification");
            if ("binder".equals(phase)) {
                new NativePluginStore(context).install(Uri.fromFile(source));
                return NativeBinderChecks.run(context);
            }
            return verify(test, fixture ? "lifecycle" : phase, fixture ? "androidtest" : expected, source);
        } finally {
            NativeCore.decode(NativeCore.stop()); CoreState.started = false;
            WorkspaceChecks.restore(preferences, saved);
            new NativePluginStore(context).prune();
            if (fixture) source.delete();
        }
    }
    private static JSONObject verify(CoreInstrumentation test, String phase, String expected, File source) throws Exception {
        Context context = test.getTargetContext();
        NativePluginStore store = new NativePluginStore(context);
        String plugin = store.install(Uri.fromFile(source));
        if (expected != null && !plugin.equals(expected)) throw new AssertionError("Unexpected package identity");
        NativeCore.initialize(context); NativeCore.session(); NativeCore.decode(NativeCore.refresh(plugin));
        JSONObject report = ApplicationChecks.plugin(test, plugin);
        if ("lifecycle".equals(phase)) {
            File tampered = new File(context.getCacheDir(), "tampered.cphplugin");
            try {
                try (ZipFile original = new ZipFile(source); ZipOutputStream zip = new ZipOutputStream(new FileOutputStream(tampered))) {
                    java.util.Enumeration<? extends ZipEntry> entries = original.entries();
                    while (entries.hasMoreElements()) {
                        ZipEntry entry = entries.nextElement(); if (entry.isDirectory()) continue;
                        byte[] bytes; try (java.io.InputStream input = original.getInputStream(entry)) { bytes = NativePluginStore.read(input, 128 * 1024 * 1024); }
                        if (entry.getName().equals("manifest.json")) bytes = new String(bytes, StandardCharsets.UTF_8).replace(plugin, "tampered_plugin").getBytes(StandardCharsets.UTF_8);
                        zip.putNextEntry(new ZipEntry(entry.getName())); zip.write(bytes); zip.closeEntry();
                    }
                }
                boolean refused = false;
                try { store.install(Uri.fromFile(tampered)); } catch (SecurityException expectedError) { refused = true; }
                if (!refused) throw new AssertionError("Tampered signature accepted");
                store.enabled(plugin, false); NativeCore.decode(NativeCore.refresh(plugin));
                if (new org.json.JSONArray(store.discover()).toString().contains("\"name\":\"" + plugin + "\"")) throw new AssertionError("Disabled plugin discovered");
                store.enabled(plugin, true); NativeCore.decode(NativeCore.refresh(plugin)); ApplicationChecks.plugin(test, plugin);
                store.remove(plugin); NativeCore.decode(NativeCore.refresh(plugin));
                boolean removed = false; try { store.library(plugin); } catch (java.io.FileNotFoundException expectedError) { removed = true; }
                if (!removed) throw new AssertionError("Uninstalled native library available");
                store.install(Uri.fromFile(source)); NativeCore.decode(NativeCore.refresh(plugin)); ApplicationChecks.plugin(test, plugin);
                report.put("lifecycle", "tamper-rejection, disable, enable, remove, reinstall");
            } finally { tampered.delete(); store.install(Uri.fromFile(source)); NativeCore.decode(NativeCore.refresh(plugin)); }
        }
        return report.put("execution", "private-native-worker");
    }
}
