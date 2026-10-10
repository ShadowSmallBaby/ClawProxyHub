package github.shadowbaby.clawproxyhub.core;

import android.app.Activity;
import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;
import android.content.Intent;
import android.content.ComponentName;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.view.View;
import android.widget.*;
import android.webkit.*;
import androidx.webkit.*;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.*;

// 原生外壳掌管应用状态和工作台选择，WebView 只接收当前工作台的受信上下文。
public final class MainActivity extends ComponentActivity {
    private static final String ORIGIN = "https://appassets.androidplatform.net";
    private static final String HOME = ORIGIN + "/assets/web/index.html";
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final ExecutorService network = Executors.newSingleThreadExecutor();
    private final ExecutorService installations = Executors.newSingleThreadExecutor();
    private boolean installingPlugin;
    private final ExecutorService listings = Executors.newSingleThreadExecutor();
    private int listGeneration;
    private PluginMarket market;
    private boolean marketRequested;
    private NativePages pages;
    private WorkspaceStore workspaces;
    private FrontendUpdates updates;
    private WebView web;
    private int tab = 1;
    private volatile int generation;
    private boolean workspaceReady;
    private ValueCallback<Uri[]> chooser;
    private byte[] download;
    private long notificationToOpen;
    private String workspaceRoute = "";

    @Override public void onCreate(Bundle saved) {
        setTheme(AppPreferences.theme(this).equals("dark") ? android.R.style.Theme_Material_NoActionBar : android.R.style.Theme_Material_Light_NoActionBar);
        super.onCreate(saved);
        AppLog.write(this, "info", "application", "opened");
        workspaces = new WorkspaceStore(this); updates = new FrontendUpdates(this); pages = new NativePages(this);
        market = new PluginMarket(this);
        if (saved != null) tab = saved.getInt("tab", 1);
        notificationIntent(getIntent());
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) { @Override public void handleOnBackPressed() { navigateBack(); } });
        TaskScheduler.schedule(this);
        WindowLayout.setContent(this, pages.create(), pages::keyboard);
        showTab(tab);
    }
    void showTab(int value) {
        tab = value;
        pages.selectTab(tab);
        if (tab == 0) { loadPlugins(); if (!marketRequested) refreshMarket(); }
        else if (tab == 1) {
            if (web == null) createWeb();
            web.onResume(); pages.workspace(web, workspaceReady);
        }
        if (tab != 1 && web != null) web.onPause();
        pages.refreshPreferences();
        if (tab == 2) { if (pages.runtimesVisible()) loadRuntimes(); else if (pages.runtimeSettingsVisible()) loadRuntimeSettings(); else loadLocalSettings(); }
    }
    private void createWeb() {
        final int epoch = generation;
        workspaceReady = false;
        web = new WebView(this); web.setTag("native.workspace");
        web.setBackgroundColor(pages.background());
        web.getSettings().setJavaScriptEnabled(true); web.getSettings().setDomStorageEnabled(true);
        web.getSettings().setAllowFileAccess(false); web.getSettings().setAllowContentAccess(false);
        web.getSettings().setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        WebViewAssetLoader.AssetsPathHandler builtin = new WebViewAssetLoader.AssetsPathHandler(this);
        WebViewAssetLoader loader = new WebViewAssetLoader.Builder().addPathHandler("/assets/", path -> { WebResourceResponse selected = updates.open(path); return selected == null ? builtin.handle(path) : selected; }).build();
        web.setWebViewClient(new WebViewClient() {
            @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) { return loader.shouldInterceptRequest(request.getUrl()); }
            @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                if (!request.isForMainFrame()) return false;
                Uri uri = request.getUrl(); if (uri.toString().startsWith(ORIGIN + "/")) return false;
                if ("https".equals(uri.getScheme())) openURL(uri.toString()); return true;
            }
            @Override public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
                if (view == web) { generation++; if (view.getParent() instanceof android.view.ViewGroup) ((android.view.ViewGroup) view.getParent()).removeView(view); view.destroy(); web = null;
                    workspaceReady = true; pages.workspace(null, true); }
                return true;
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (chooser != null) chooser.onReceiveValue(null); chooser = callback; choose(10); return true;
            }
        });
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) {
            workspaceReady = true; web.loadData("<p>请更新 Android System WebView / Update Android System WebView</p>", "text/html", "UTF-8"); return;
        }
        WebViewCompat.addWebMessageListener(web, "cphPlatform", Collections.singleton(ORIGIN), (view, message, source, mainFrame, reply) -> {
            if (!mainFrame || !ORIGIN.equals(source.toString()) || epoch != generation) return;
            JSONObject request = null;
            try { request = new JSONObject(message.getData()); handle(request, reply, epoch); }
            catch (Exception error) { respond(reply, request == null ? "" : request.optString("id"), null, error, epoch); }
        });
        web.loadUrl(HOME + workspaceRoute);
        workspaceRoute = "";
        web.postDelayed(() -> { if (!isDestroyed() && epoch == generation && updates.pending()) { updates.rollback(); resetWorkspace(); showTab(tab); } }, 15000);
    }
    private void handle(JSONObject request, JavaScriptReplyProxy reply, int epoch) throws Exception {
        String type = request.getString("type"), id = request.optString("id");
        if ("frontend-ready".equals(type)) { updates.confirm(); return; }
        if ("notifications-sync".equals(type)) requestNotificationPermission();
        if ("clipboard-write".equals(type)) {
            String text = request.getString("text");
            if (text.length() > 1024 * 1024 || !hasWindowFocus()) throw new IllegalArgumentException("Clipboard request unavailable");
            android.content.ClipData clip = android.content.ClipData.newPlainText("ClawProxyHub", text);
            if (Build.VERSION.SDK_INT >= 33) { android.os.PersistableBundle extras = new android.os.PersistableBundle(); extras.putBoolean(android.content.ClipDescription.EXTRA_IS_SENSITIVE, true); clip.getDescription().setExtras(extras); }
            getSystemService(android.content.ClipboardManager.class).setPrimaryClip(clip);
            respond(reply, id, new JSONObject(), null, epoch); return;
        }
        if ("download".equals(type)) {
            if (download != null) throw new IllegalStateException("Another download is pending");
            String encoded = request.getString("data"); if (encoded.length() > 24 * 1024 * 1024) throw new IllegalArgumentException("Download exceeds 16 MiB");
            download = android.util.Base64.decode(encoded, android.util.Base64.DEFAULT);
            if (download.length > 16 * 1024 * 1024) { download = null; throw new IllegalArgumentException("Download exceeds 16 MiB"); }
            startActivityForResult(new Intent(Intent.ACTION_CREATE_DOCUMENT).setType("application/octet-stream").addCategory(Intent.CATEGORY_OPENABLE)
                    .putExtra(Intent.EXTRA_TITLE, request.optString("name", "backup.zip").replaceAll("[^a-zA-Z0-9._-]", "_")), 11); return;
        }
        worker.execute(() -> {
            try {
                if (epoch != generation) return;
                JSONObject result;
                if ("workspace-state".equals(type)) {
                    workspaces.migrate(request.optJSONObject("legacy"), this);
                    result = new JSONObject().put("version", BuildConfig.VERSION_NAME).put("package", getPackageName()).put("remoteOnly", AppPreferences.remoteOnly(this))
                            .put("locale", AppPreferences.locale(this)).put("theme", AppPreferences.theme(this));
                    result.put("notification", notificationToOpen); notificationToOpen = 0;
                    String selected = workspaces.selected(); JSONObject connection = workspaces.find(selected);
                    if (WorkspaceStore.LOCAL.equals(selected) && !AppPreferences.remoteOnly(this)) {
                        NativeCore.initialize(this); JSONObject session = NativeCore.session();
                        connection = new JSONObject().put("id", WorkspaceStore.LOCAL).put("name", "").put("baseURL", session.getString("address")).put("token", session.getString("token").isEmpty() ? "" : workspaces.token(WorkspaceStore.LOCAL));
                    } else if (connection != null) connection.put("token", workspaces.token(selected));
                    if (connection != null) result.put("connection", connection);
                    runOnUiThread(() -> { if (!isDestroyed() && epoch == generation) { workspaceReady = true; pages.workspace(web, true); pages.selectTab(tab); pages.refreshPreferences(); } });
                } else if ("workspace-token".equals(type)) {
                    workspaces.saveToken(request.getString("connection"), request.getString("token")); result = new JSONObject();
                } else if ("notifications-sync".equals(type)) {
                    String selected = workspaces.selected();
                    if (!selected.equals(request.getString("connection"))) throw new IllegalArgumentException("Workspace changed");
                    JSONObject workspace = workspaces.find(selected);
                    String name = workspace == null ? pages.text("内置工作台", "Built-in workspace") : workspace.getString("name");
                    result = new JSONObject().put("native", AppNotifications.deliver(this, selected, name, request.getJSONArray("notifications"), request.optInt("unread")));
                } else throw new IllegalArgumentException("Unsupported application operation");
                respond(reply, id, result, null, epoch);
            } catch (Exception error) { runOnUiThread(() -> { if (epoch == generation) { workspaceReady = true; pages.workspace(web, true); } }); respond(reply, id, null, error, epoch); }
        });
    }
    private void respond(JavaScriptReplyProxy reply, String id, JSONObject result, Exception error, int epoch) {
        if (id.isEmpty()) return;
        runOnUiThread(() -> { if (isDestroyed() || epoch != generation) return; try {
            JSONObject response = new JSONObject().put("id", id); if (error == null) response.put("result", result); else response.put("error", error.getMessage()); reply.postMessage(response.toString());
        } catch (Exception ignored) { } });
    }
    private void resetWorkspace() {
        generation++;
        workspaceReady = false; pages.workspace(null, false);
        if (chooser != null) { chooser.onReceiveValue(null); chooser = null; }
        if (web != null) { if (web.getParent() instanceof android.view.ViewGroup) ((android.view.ViewGroup) web.getParent()).removeView(web); web.stopLoading(); web.destroy(); web = null; }
    }
    void chooseWorkspace() {
        try { pages.chooseWorkspace(workspaces.list(), workspaces.selected(), AppPreferences.remoteOnly(this)); }
        catch (Exception error) { showError(error); }
    }
    String saveWorkspace(String id, String name, String address) {
        try {
            if (id.isEmpty()) switchWorkspace(workspaces.add(name, address));
            else { workspaces.update(id, name, workspaces.find(id).getString("baseURL")); pages.workspaceList(workspaces.list(), workspaces.selected()); }
            return null;
        }
        catch (Exception error) { return pages.text("请输入有效的 HTTPS 地址", "Enter a valid HTTPS address"); }
    }
    void switchWorkspace(String id) { change(() -> workspaces.select(id), true); }
    void removeWorkspace(String id) { change(() -> { workspaces.remove(id); JSONArray items = workspaces.list(); String selected = workspaces.selected(); runOnUiThread(() -> pages.workspaceList(items, selected)); }, true); }
    void floatingTabs(boolean enabled) { AppPreferences.floatingTabs(this, enabled); pages.refreshPreferences(); }
    void notificationMode(boolean system) {
        AppNotifications.system(this, system); requestNotificationPermission(); pages.refreshPreferences(); notifyNotificationMode();
    }
    private void requestNotificationPermission() {
        if (Build.VERSION.SDK_INT < 33 || !AppNotifications.system(this) || checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) == android.content.pm.PackageManager.PERMISSION_GRANTED) return;
        if (getSharedPreferences("application", 0).getBoolean("notification-permission-asked", false)) return;
        getSharedPreferences("application", 0).edit().putBoolean("notification-permission-asked", true).apply();
        requestPermissions(new String[]{android.Manifest.permission.POST_NOTIFICATIONS}, 16);
    }
    private void notifyNotificationMode() { if (web != null) web.evaluateJavascript("window.dispatchEvent(new Event('cph:notifications-changed'))", null); }
    private void notificationIntent(Intent intent) {
        if (intent == null || !"cph.notification".equals(intent.getAction())) return;
        String id = intent.getStringExtra("notification-workspace");
        if (id == null || WorkspaceStore.LOCAL.equals(id) && AppPreferences.remoteOnly(this)) return;
        try { workspaces.select(id); notificationToOpen = intent.getLongExtra("notification-id", 0); tab = 1; }
        catch (Exception ignored) { /* 已移除的工作台不再打开。 */ }
        intent.removeExtra("notification-id"); intent.setAction(null);
    }
    @Override protected void onNewIntent(Intent intent) { super.onNewIntent(intent); setIntent(intent); notificationIntent(intent); resetWorkspace(); showTab(tab); }
    @Override public void onRequestPermissionsResult(int code, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(code, permissions, results);
        if (code == 16) { pages.refreshPreferences(); notifyNotificationMode(); }
    }
    String githubProxy(String value) {
        try { AppPreferences.githubProxy(this, value); pages.refreshPreferences(); return null; }
        catch (Exception error) { return error.getMessage(); }
    }
    private void loadLocalSettings() {
        if (AppPreferences.remoteOnly(this) || !CoreState.started) return;
        listings.execute(() -> {
            try {
                JSONObject settings = CoreApi.localSettings();
                if (settings != null && !getSharedPreferences("application", 0).contains("github-proxy")) {
                    String proxy = settings.optString("github_proxy");
                    try { AppPreferences.githubProxy(this, proxy); } catch (IllegalArgumentException ignored) { }
                }
                runOnUiThread(() -> { if (!isDestroyed()) pages.refreshPreferences(); });
            } catch (Exception error) { AppLog.write(this, "error", "application", "Cannot load local settings"); }
        });
    }
    void logging(boolean enabled, String level, int days) { change(() -> AppLog.configure(this, enabled, level, days), false); }
    void showLogs() { pages.openLogs(); listings.execute(() -> { try { String content = AppLog.read(this); runOnUiThread(() -> { if (!isDestroyed()) pages.logs(content); }); } catch (Exception error) { showError(error); } }); }
    void clearLogs() { change(() -> { AppLog.clear(this); runOnUiThread(this::showLogs); }, false); }
    void checkUpdate() { pages.updateResult(null, null); network.execute(() -> {
        try { JSONObject result = AppUpdates.check(this); runOnUiThread(() -> { if (!isDestroyed()) pages.updateResult(result, null); }); }
        catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.updateResult(null, pages.text("检查失败，请稍后重试", "Could not check for updates. Try again later.")); }); }
    }); }
    void refreshMarket() {
        marketRequested = true; pages.loadingMarket();
        network.execute(() -> {
            try {
                JSONArray cached = market.load(false);
                runOnUiThread(() -> { if (!isDestroyed()) pages.market(cached, false, true); });
                JSONArray current = market.load(true);
                runOnUiThread(() -> { if (!isDestroyed()) pages.market(current, true, false); });
            } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.marketUnavailable(); }); }
        });
    }
    void installMarket(String name, boolean lua) {
        if (lua && AppPreferences.remoteOnly(this)) { showError(new IllegalStateException(pages.text("请先关闭仅远程模式，再安装本机脚本。", "Turn off Remote only before installing local scripts."))); return; }
        install(name, () -> {
            String installed = market.install(name, (phase, received, total) -> runOnUiThread(() -> { if (!isDestroyed()) pages.installProgress(phase, received, total); }));
            if (!lua) refreshPlugin(installed);
            return installed;
        });
    }
    interface Installation { String run() throws Exception; }
    private void install(String label, Installation action) {
        if (installingPlugin) return;
        installingPlugin = true;
        getWindow().getDecorView().performHapticFeedback(android.view.HapticFeedbackConstants.VIRTUAL_KEY);
        pages.installing(label);
        AppLog.write(this, "info", "plugins", "installation started");
        installations.execute(() -> {
            try {
                String installed = action.run();
                AppLog.write(this, "info", "plugins", "installation completed");
                runOnUiThread(() -> { if (!isDestroyed()) {
                    installingPlugin = false; pages.installComplete(installed); resetWorkspace(); showTab(tab); if (tab != 0) loadPlugins();
                } });
            } catch (Exception error) {
                AppLog.write(this, "error", "plugins", "installation failed: " + error.getClass().getSimpleName());
                runOnUiThread(() -> { if (!isDestroyed()) { installingPlugin = false; pages.installFailed(error.getMessage()); loadPlugins(); } });
            }
        });
    }
    interface Change { void run() throws Exception; }
    private void change(Change action, boolean recreateWeb) {
        worker.execute(() -> { try { action.run(); runOnUiThread(() -> { if (isDestroyed()) return; if (recreateWeb) resetWorkspace(); showTab(tab); }); }
            catch (Exception error) { showError(error); runOnUiThread(() -> { if (!isDestroyed()) showTab(tab); }); } });
    }
    void setRemoteMode(boolean enabled) { change(() -> { TaskScheduler.setRemoteOnly(this, enabled); workspaces.remoteOnly(enabled); }, true); }
    void appearance(String locale, String theme) {
        try { AppPreferences.appearance(this, locale, theme); recreate(); } catch (Exception error) { showError(error); }
    }
    void openURL(String url) { try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url))); } catch (Exception error) { showError(error); } }
    void downloadUpdate(String url) { openURL(AppPreferences.githubURL(this, url)); }
    void openSystemSettings(String section) {
        if ("notifications".equals(section)) {
            Intent intent = Build.VERSION.SDK_INT >= 26 ? new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, getPackageName())
                    : new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:" + getPackageName()));
            startActivity(intent); return;
        }
        if ("autostart".equals(section)) {
            String[][] components = {{"com.miui.securitycenter", "com.miui.permcenter.autostart.AutoStartManagementActivity"}, {"com.coloros.safecenter", "com.coloros.safecenter.permission.startup.StartupAppListActivity"}, {"com.huawei.systemmanager", "com.huawei.systemmanager.startupmgr.ui.StartupNormalAppListActivity"}, {"com.vivo.permissionmanager", "com.vivo.permissionmanager.activity.BgStartUpManagerActivity"}};
            for (String[] component : components) try { startActivity(new Intent().setComponent(new ComponentName(component[0], component[1]))); return; } catch (Exception ignored) { }
        } else if ("battery".equals(section)) {
            try { startActivity(new Intent().setComponent(new ComponentName("com.miui.powerkeeper", "com.miui.powerkeeper.ui.HiddenAppsConfigActivity")).putExtra("package_name", getPackageName()).putExtra("package_label", "ClawProxyHub")); return; } catch (Exception ignored) { }
            try { startActivity(new Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS)); return; } catch (Exception ignored) { }
        }
        try { startActivity(new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:" + getPackageName()))); } catch (Exception error) { showError(error); }
    }
    private void loadPlugins() {
        final int epoch = ++listGeneration;
        pages.loadingPlugins();
        pages.pluginExtensions(new JSONArray());
        listings.execute(() -> { try {
            JSONArray items = InstalledPlugins.read(this);
            String runtime;
            try { runtime = PluginState.luaRuntime(new HostStore(this).current(), AppPreferences.remoteOnly(this)); }
            catch (Exception error) { runtime = "runtime_unavailable"; }
            final String luaRuntime = runtime;
            runOnUiThread(() -> { if (!isDestroyed() && epoch == listGeneration) { pages.pluginRuntime(luaRuntime); pages.plugins(items); } });
            if (CoreState.started && !AppPreferences.remoteOnly(this)) {
                // 状态补充不影响本地列表；核心忙或未初始化时仍可查看和移除已安装包。
                try {
                    JSONArray updated = new JSONArray(items.toString());
                    JSONArray running = CoreApi.pluginStatus().getJSONArray("plugins");
                    for (int i = 0; i < updated.length(); i++) for (int j = 0; j < running.length(); j++) {
                        JSONObject item = updated.getJSONObject(i), state = running.getJSONObject(j);
                        if (item.optString("name").equals(state.optString("name")) && "lua".equals(item.optString("runtime")))
                            item.put("enabled", state.optBoolean("enabled", state.optBoolean("running"))).put("running", state.optBoolean("running")).put("editable", state.optBoolean("editable"));
                    }
                    runOnUiThread(() -> { if (!isDestroyed() && epoch == listGeneration) pages.plugins(updated); });
                } catch (Exception ignored) { /* 没有运行状态时显示安装类型，不把未知状态标成停用。 */ }
                try {
                    JSONArray extensions = CoreApi.extensionStatus().getJSONArray("extensions");
                    runOnUiThread(() -> { if (!isDestroyed() && epoch == listGeneration) pages.pluginExtensions(extensions); });
                } catch (Exception ignored) { /* 扩展查询失败不影响本地插件管理。 */ }
            }
        } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed() && epoch == listGeneration) pages.plugins(new JSONArray()); }); showError(error); } });
    }
    private void refreshPlugin(String name) throws Exception { if (CoreState.started) NativeCore.decode(NativeCore.refresh(name)); new NativePluginStore(this).prune(); }
    void togglePlugin(String name, boolean lua, boolean enabled) { change(() -> {
        if (lua) CoreApi.call(this, "POST", "/admin/plugins/" + name + (enabled ? "/start" : "/stop"), new JSONObject());
        else { new NativePluginStore(this).enabled(name, enabled); refreshPlugin(name); if (enabled && CoreState.started) CoreApi.call(this, "POST", "/admin/plugins/" + name + "/start", new JSONObject()); }
    }, true); }
    void removePlugin(String name, boolean lua) { change(() -> { if (lua) CoreApi.call(this, "DELETE", "/admin/plugins/" + name, null); else { new NativePluginStore(this).remove(name); refreshPlugin(name); } }, true); }
    void rollbackPlugin(String name) { change(() -> { new NativePluginStore(this).rollback(name); refreshPlugin(name); }, true); }
    private void choose(int code) { startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*").addCategory(Intent.CATEGORY_OPENABLE), code); }
    void choosePlugin() { choose(13); }
    void chooseLua() { if (AppPreferences.remoteOnly(this)) { showError(new IllegalStateException(pages.text("请先在设置中关闭仅远程模式，再导入本机脚本。", "Turn off Remote only in Settings before importing local scripts."))); return; } choose(14); }
    void chooseFrontend() { choose(12); }
    void chooseRuntime() { choose(15); }
    void showRuntimes() { if (tab != 2) showTab(2); pages.openRuntimes(); loadRuntimes(); }
    void showRuntimeSettings() { pages.openRuntimeSettings(); loadRuntimeSettings(); }
    private void loadRuntimeSettings() {
        listings.execute(() -> {
            try {
                JSONObject result = new RuntimePackages(this).request(new JSONObject().put("operation", "settings").put("id", "lua-runtime"));
                runOnUiThread(() -> { if (!isDestroyed()) pages.runtimeSettings(result, null); });
            } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.runtimeSettings(null, error.getMessage()); }); }
        });
    }
    void saveRuntimeSettings(String hash, JSONObject values) {
        pages.savingRuntimeSettings();
        worker.execute(() -> {
            try {
                JSONObject result = new RuntimePackages(this).request(new JSONObject().put("operation", "configure").put("id", "lua-runtime").put("sha256", hash).put("values", values));
                runOnUiThread(() -> { if (!isDestroyed()) { pages.runtimeSettings(result, null); android.widget.Toast.makeText(this, pages.text("已保存", "Saved"), android.widget.Toast.LENGTH_SHORT).show(); } });
            } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.runtimeSettings(null, error.getMessage()); }); }
        });
    }
    private void loadRuntimes() {
        pages.loadingRuntimes();
        listings.execute(() -> {
            try {
                JSONObject result = new RuntimePackages(this).request(new JSONObject().put("operation", "list"));
                runOnUiThread(() -> { if (!isDestroyed()) pages.runtimes(result, null); });
            } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.runtimes(null, error.getMessage()); }); }
        });
    }
    void runtimeChange(String operation) { change(() -> new RuntimePackages(this).request(new JSONObject().put("operation", operation).put("id", "lua-runtime")), true); }
    void installMountedRuntime(String hash, JSONArray grants) { install("Lua Host", () -> {
        pagesProgress("installing", 0, -1);
        new RuntimePackages(this).request(new JSONObject().put("operation", "mounted").put("id", "lua-runtime").put("sha256", hash).put("grants", grants));
        return "Lua Host";
    }); }
    void checkRuntimeUpdate() {
        pages.checkingRuntimeUpdate();
        network.execute(() -> {
            try {
                JSONObject item = new RuntimePackages(this).latest();
                runOnUiThread(() -> { if (!isDestroyed()) pages.runtimeUpdate(item, null); });
            } catch (Exception error) { runOnUiThread(() -> { if (!isDestroyed()) pages.runtimeUpdate(null, error.getMessage()); }); }
        });
    }
    void installRuntimeUpdate(JSONObject item) { install("Lua Host", () -> new RuntimePackages(this).installRelease(item, this::pagesProgress)); }
    private void pagesProgress(String phase, long received, long total) { runOnUiThread(() -> { if (!isDestroyed()) pages.installProgress(phase, received, total); }); }
    void showExtensions() {
        try {
            workspaces.select(WorkspaceStore.LOCAL);
            resetWorkspace(); workspaceRoute = "#/extensions"; showTab(1);
        } catch (Exception error) { showError(error); }
    }
    void openExtension(JSONObject contribution, String name) {
        if (AppPreferences.remoteOnly(this)) return;
        listings.execute(() -> {
            try {
                JSONArray entries = ExtensionContributions.list(CoreApi.extensionStatus().getJSONArray("extensions"), contribution.getString("location"), !name.isEmpty());
                JSONObject current = null;
                for (int i = 0; i < entries.length(); i++) {
                    JSONObject entry = entries.getJSONObject(i);
                    if (entry.getString("extension").equals(contribution.getString("extension")) && entry.getString("id").equals(contribution.getString("id"))) current = entry;
                }
                if (current == null) throw new IllegalStateException(pages.text("扩展已停用或不可用。", "The extension is disabled or unavailable."));
                String route = ExtensionContributions.route(current, name);
                runOnUiThread(() -> {
                    if (isDestroyed()) return;
                    try { workspaces.select(WorkspaceStore.LOCAL); resetWorkspace(); workspaceRoute = route; showTab(1); }
                    catch (Exception error) { showError(error); }
                });
            } catch (Exception error) { showError(error); }
        });
    }
    void resetFrontend() { change(updates::reset, true); }
    void importLua(Uri uri, String name) { install(name, () -> {
        String source;
        try (InputStream input = getContentResolver().openInputStream(uri)) { source = new String(NativePluginStore.read(input, 2 * 1024 * 1024), StandardCharsets.UTF_8); }
        runOnUiThread(() -> pages.installProgress("installing", 0, -1));
        CoreApi.call(this, "POST", "/admin/actions/core.workspace.create", new JSONObject().put("name", name).put("content", source));
        return name;
    }); }
    @Override protected void onActivityResult(int code, int result, Intent data) {
        super.onActivityResult(code, result, data);
        if (code == 10) { if (chooser != null) { chooser.onReceiveValue(result == RESULT_OK && data != null ? new Uri[]{data.getData()} : null); chooser = null; } return; }
        if (code == 11) { byte[] bytes = download; download = null; if (result == RESULT_OK && data != null && bytes != null) change(() -> { try (OutputStream output = getContentResolver().openOutputStream(data.getData(), "wt")) { output.write(bytes); } }, false); return; }
        if (result != RESULT_OK || data == null || data.getData() == null) return;
        Uri uri = data.getData();
        if (code == 13) install(pages.text("插件包", "Plugin package"), () -> importPackage(uri));
        else if (code == 12) change(() -> updates.install(uri), true);
        else if (code == 14) pages.askLua(uri);
        else if (code == 15) install("Lua Host", () -> new RuntimePackages(this).install(uri, this::pagesProgress));
    }
    private String importPackage(Uri uri) throws Exception {
        byte[] bytes;
        try (InputStream input = getContentResolver().openInputStream(uri)) { bytes = NativePluginStore.read(input, 128 * 1024 * 1024); }
        runOnUiThread(() -> pages.installProgress("verifying", bytes.length, bytes.length));
        File temporary = File.createTempFile("plugin-import-", ".cphplugin", getCacheDir());
        try {
            try (FileOutputStream output = new FileOutputStream(temporary)) { output.write(bytes); }
            try (java.util.zip.ZipFile zip = new java.util.zip.ZipFile(temporary)) {
                java.util.zip.ZipEntry entry = zip.getEntry("manifest.json");
                if (entry == null) throw new IOException("Plugin manifest missing");
                JSONObject manifest;
                try (InputStream input = zip.getInputStream(entry)) { manifest = new JSONObject(new String(NativePluginStore.read(input, 1024 * 1024), StandardCharsets.UTF_8)); }
                runOnUiThread(() -> pages.installProgress("installing", 0, -1));
                if ("lua".equals(manifest.optString("runtime"))) return CoreApi.upload(this, bytes);
            }
            String name = new NativePluginStore(this).install(Uri.fromFile(temporary)); refreshPlugin(name); return name;
        } finally { temporary.delete(); }
    }
    private void showError(Exception error) { AppLog.write(this, "error", "application", "operation failed: " + error.getClass().getSimpleName()); runOnUiThread(() -> { if (!isDestroyed()) pages.error(error.getMessage()); }); }
    private void navigateBack() { if (pages.back()) return; if (tab != 1) showTab(1); else if (web != null && web.canGoBack()) web.goBack(); else finish(); }
    @Override protected void onResume() { super.onResume(); if (pages != null) { pages.refreshPreferences(); if (tab == 0) loadPlugins(); } notifyNotificationMode(); }
    @Override protected void onSaveInstanceState(Bundle state) { state.putInt("tab", tab); super.onSaveInstanceState(state); }
    @Override protected void onDestroy() { resetWorkspace(); worker.shutdown(); network.shutdown(); listings.shutdown(); installations.shutdown(); super.onDestroy(); }
}
