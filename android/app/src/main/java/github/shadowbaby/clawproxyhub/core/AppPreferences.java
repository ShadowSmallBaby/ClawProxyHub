package github.shadowbaby.clawproxyhub.core;

import android.content.Context;

// 应用偏好独立于本机和远程后端，纯远程模式不加载 Go 核心。
final class AppPreferences {
    static String githubProxy(Context context) { return context.getSharedPreferences("application", 0).getString("github-proxy", ""); }
    static void githubProxy(Context context, String value) {
        String proxy = value.trim().replaceAll("/+$", "");
        if (!proxy.isEmpty()) {
            try {
                java.net.URI uri = new java.net.URI(proxy);
                if (!"https".equals(uri.getScheme()) || uri.getHost() == null || uri.getUserInfo() != null || uri.getQuery() != null || uri.getFragment() != null) throw new IllegalArgumentException();
            } catch (Exception error) { throw new IllegalArgumentException("请输入 HTTPS 代理地址 / Enter an HTTPS proxy URL"); }
        }
        if (!context.getSharedPreferences("application", 0).edit().putString("github-proxy", proxy).commit()) throw new IllegalStateException("Cannot save download proxy");
    }
    static String githubURL(Context context, String address) {
        String proxy = githubProxy(context), host = android.net.Uri.parse(address).getHost();
        return !proxy.isEmpty() && ("github.com".equals(host) || "raw.githubusercontent.com".equals(host) || "api.github.com".equals(host)) ? proxy + "/" + address : address;
    }
    static boolean floatingTabs(Context context) { return context.getSharedPreferences("application", 0).getBoolean("floating-tabs", false); }
    static void floatingTabs(Context context, boolean enabled) { context.getSharedPreferences("application", 0).edit().putBoolean("floating-tabs", enabled).apply(); }
    static String locale(Context context) { return context.getSharedPreferences("application", 0).getString("locale", java.util.Locale.getDefault().getLanguage().equals("zh") ? "zh" : "en"); }
    static String theme(Context context) { return context.getSharedPreferences("application", 0).getString("theme", "light"); }
    static void appearance(Context context, String locale, String theme) {
        if (!context.getSharedPreferences("application", 0).edit().putString("locale", "en".equals(locale) ? "en" : "zh")
                .putString("theme", "dark".equals(theme) ? "dark" : "light").commit()) throw new IllegalStateException("Cannot save appearance");
    }
    static boolean remoteOnly(Context context) {
        return context.getSharedPreferences("application", 0).getBoolean("remote-only", false);
    }
    static void setRemoteOnly(Context context, boolean value) {
        if (!context.getSharedPreferences("application", 0).edit().putBoolean("remote-only", value).commit())
            throw new IllegalStateException("Cannot persist application mode");
    }
}
