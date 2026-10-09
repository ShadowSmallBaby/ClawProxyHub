package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Comparator;
import org.json.*;

// 本地安装目录统一读取，允许系统数据目录别名，拒绝插件自身的目录链接。
final class InstalledPlugins {
    static JSONArray read(Context context) throws Exception {
        JSONArray items = new NativePluginStore(context).list();
        addLua(new File(context.getFilesDir(), "plugins").getCanonicalFile(), items, 2);
        return items;
    }
    static void attachIcon(JSONObject item, File directory) throws Exception {
        item.remove("local_icon"); String icon = item.optString("icon"); if (icon.isEmpty()) return;
        File file = new File(directory, icon).getCanonicalFile();
        if (file.getPath().startsWith(directory.getCanonicalPath() + File.separator) && file.isFile() && file.length() <= 2 * 1024 * 1024) item.put("local_icon", file.getAbsolutePath());
    }
    private static void addLua(File root, JSONArray items, int depth) throws Exception {
        File[] children = root.listFiles(); if (children == null) return;
        Arrays.sort(children, Comparator.comparing(File::getName));
        for (File dir : children) {
            if (dir.getName().startsWith(".") || !dir.isDirectory() || !dir.getCanonicalFile().equals(dir.getAbsoluteFile())) continue;
            File manifest = new File(dir, "manifest.json");
            if (!manifest.isFile()) { if (depth > 1) addLua(dir, items, depth - 1); continue; }
            try (InputStream input = new FileInputStream(manifest)) {
                JSONObject item = new JSONObject(new String(NativePluginStore.read(input, 1024 * 1024), StandardCharsets.UTF_8));
                if ("lua".equals(item.optString("runtime"))) { attachIcon(item, dir); items.put(item); }
            } catch (IOException | JSONException ignored) { /* 单个损坏的清单不应清空其他安装项。 */ }
        }
    }
}
