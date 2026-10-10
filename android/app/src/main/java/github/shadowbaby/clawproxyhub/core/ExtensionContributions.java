package github.shadowbaby.clawproxyhub.core;

import java.net.URLEncoder;
import java.util.ArrayList;
import java.util.Comparator;
import org.json.JSONArray;
import org.json.JSONObject;

// 原生插件页使用已激活扩展的同一份贡献声明，页面仍由工作台沙箱承载。
final class ExtensionContributions {
    static JSONArray list(JSONArray states, String location, boolean editable) {
        ArrayList<JSONObject> entries = new ArrayList<>();
        for (int i = 0; i < states.length(); i++) {
            JSONObject state = states.optJSONObject(i);
            if (state == null || !state.optBoolean("available")) continue;
            JSONObject manifest = state.optJSONObject("manifest");
            if (manifest == null || !supportsApp(manifest)) continue;
            JSONArray contributions = manifest.optJSONArray("contributions"), pages = manifest.optJSONArray("pages");
            if (contributions == null || pages == null) continue;
            for (int j = 0; j < contributions.length(); j++) {
                JSONObject entry = contributions.optJSONObject(j);
                if (entry == null || !location.equals(entry.optString("location")) ||
                        !(entry.optString("when").isEmpty() || editable && "editable".equals(entry.optString("when")))) continue;
                boolean pageExists = false;
                for (int k = 0; k < pages.length(); k++) {
                    JSONObject page = pages.optJSONObject(k);
                    if (page != null && entry.optString("page").equals(page.optString("id"))) pageExists = true;
                }
                if (!pageExists) continue;
                try {
                    JSONObject copy = new JSONObject(entry.toString()).put("extension", manifest.getString("id"));
                    entries.add(copy);
                } catch (org.json.JSONException ignored) { /* 不完整的条目不提供入口。 */ }
            }
        }
        entries.sort(Comparator.comparingInt(entry -> entry.optInt("order")));
        return new JSONArray(entries);
    }
    private static boolean supportsApp(JSONObject manifest) {
        JSONArray environments = manifest.optJSONArray("environments");
        if (environments == null) return true;
        for (int i = 0; i < environments.length(); i++) if ("app".equals(environments.optString(i))) return true;
        return false;
    }
    static String route(JSONObject entry, String name) throws Exception {
        String route = "#/extensions/" + encode(entry.getString("extension")) + "/" + encode(entry.getString("page"))
                + "?contribution=" + encode(entry.getString("id"));
        if (!name.isEmpty()) route += "&name=" + encode(name);
        return route;
    }
    private static String encode(String value) throws Exception { return URLEncoder.encode(value, "UTF-8").replace("+", "%20"); }
}
