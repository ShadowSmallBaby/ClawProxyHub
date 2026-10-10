package github.shadowbaby.clawproxyhub.core;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.view.ViewGroup;
import android.view.accessibility.AccessibilityNodeInfo;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsCompat;
import java.util.concurrent.atomic.AtomicReference;
import org.json.JSONObject;

final class WindowChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        Context context = test.getTargetContext();
        android.accessibilityservice.AccessibilityServiceInfo service = test.getUiAutomation().getServiceInfo();
        service.flags |= android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS;
        test.getUiAutomation().setServiceInfo(service);
        boolean original = AppPreferences.remoteOnly(context);
        boolean floating = AppPreferences.floatingTabs(context);
        java.util.Map<String, ?> saved = context.getSharedPreferences("workspaces", 0).getAll();
        java.util.Map<String, ?> application = context.getSharedPreferences("application", 0).getAll();
        Activity activity = null;
        android.app.Instrumentation.ActivityMonitor monitor = test.addMonitor(MainActivity.class.getName(), null, false);
        try {
            AppPreferences.setRemoteOnly(context, true);
            context.getSharedPreferences("application", 0).edit().putBoolean("system-notifications", false).commit();
            context.startActivity(new Intent(context, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK));
            try (android.os.ParcelFileDescriptor launch = test.getUiAutomation().executeShellCommand("am start -W -f 0x10008000 -n github.shadowbaby.clawproxyhub.core/.MainActivity");
                 java.io.InputStream input = new android.os.ParcelFileDescriptor.AutoCloseInputStream(launch)) { NativePluginStore.read(input, 4096); }
            activity = test.waitForMonitorWithTimeout(monitor, 15000);
            if (activity == null) throw new AssertionError("application window did not open within 15 seconds");
            Activity target = activity;
            AtomicReference<JSONObject> result = new AtomicReference<>();
            AtomicReference<Throwable> failure = new AtomicReference<>();
            for (int retry = 0; retry < 100 && result.get() == null && failure.get() == null; retry++) {
                test.runOnMainSync(() -> {
                    try {
                        ViewGroup frame = target.findViewById(android.R.id.content);
                        ViewGroup container = (ViewGroup) frame.getChildAt(0);
                        View web = container.getChildAt(0);
                        if (!"native.shell".equals(web.getTag())) throw new AssertionError("Miuix shell missing");
                        WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(container);
                        if (insets == null || web.getHeight() == 0) return;
                        Insets safe = insets.getInsets(WindowInsetsCompat.Type.systemBars() | WindowInsetsCompat.Type.displayCutout());
                        int[] position = new int[2];
                        web.getLocationInWindow(position);
                        if (position[1] != safe.top || web.getPaddingTop() != 0 || web.getPaddingBottom() != 0
                                || web.getHeight() != container.getHeight() - safe.top - safe.bottom)
                            throw new AssertionError("system insets applied more than once or content overlaps bars");
                        result.set(new JSONObject().put("ok", true).put("top", position[1]).put("statusInset", safe.top)
                                .put("bottomInset", safe.bottom).put("shellHeight", web.getHeight()));
                    } catch (Throwable error) { failure.set(error); }
                });
                if (result.get() == null && failure.get() == null) Thread.sleep(50);
            }
            if (failure.get() != null) throw new AssertionError(failure.get());
            if (result.get() == null) throw new AssertionError("window did not receive insets");
            for (int i = 0; i < 3; i++) awaitNode(test, "native.tab." + i);
            test.runOnMainSync(() -> ((MainActivity) target).showTab(2));
            awaitNode(test, "native.remote-only");
            if (CoreState.started) throw new AssertionError("remote-only settings started the core");
            awaitNode(test, "native.theme");
            test.runOnMainSync(() -> ((MainActivity) target).showLogs());
            awaitNode(test, "native.log-page"); awaitNode(test, "native.log-content");
            awaitNode(test, "native.log-refresh").performAction(AccessibilityNodeInfo.ACTION_CLICK);
            awaitNode(test, "native.log-back").performAction(AccessibilityNodeInfo.ACTION_CLICK);
            awaitNode(test, "native.remote-only");
            test.runOnMainSync(() -> ((MainActivity) target).floatingTabs(true));
            awaitNode(test, "native.tabs.floating");
            for (int i = 0; i < 3; i++) awaitNode(test, "native.tab." + i);
            capture(test, "settings");
            if (!awaitNode(test, "native.tab.0").performAction(AccessibilityNodeInfo.ACTION_CLICK)) throw new AssertionError("native tab cannot be selected");
            awaitNode(test, "native.plugin-tabs");
            awaitNode(test, "native.plugin-search");
            verifyKeyboard(test, target, true);
            test.runOnMainSync(() -> ((MainActivity) target).floatingTabs(false));
            verifyKeyboard(test, target, false);
            test.runOnMainSync(() -> ((MainActivity) target).floatingTabs(true));
            if (!awaitNode(test, "native.import").performAction(AccessibilityNodeInfo.ACTION_CLICK)) throw new AssertionError("native import dialog unavailable");
            awaitNode(test, "native.dialog.import");
            java.lang.reflect.Field pagesField = MainActivity.class.getDeclaredField("pages"); pagesField.setAccessible(true);
            NativePages pages = (NativePages) pagesField.get(target);
            test.runOnMainSync(() -> { pages.installing("installation-check"); pages.installProgress("downloading", 37, 100); });
            awaitNode(test, "native.install-indicator"); awaitNode(test, "native.install-log");
            capture(test, "install");
            test.runOnMainSync(() -> pages.installComplete("installation-check"));
            test.runOnMainSync(() -> { ((MainActivity) target).getOnBackPressedDispatcher().onBackPressed(); ((MainActivity) target).showTab(1); });
            verifyClipboard(test, target);
            WorkspaceStore store = new WorkspaceStore(context);
            String fixture = store.add("Swipe fixture", "https://swipe-fixture.example");
            for (int i = 0; i < 24; i++) store.add("Workspace " + i, "https://workspace-" + i + ".example");
            if (!awaitNode(test, "native.tab.1").performAction(AccessibilityNodeInfo.ACTION_CLICK)) throw new AssertionError("workspace picker unavailable");
            awaitNode(test, "native.workspace-drawer");
            android.graphics.Rect list = new android.graphics.Rect(), add = new android.graphics.Rect(), row = new android.graphics.Rect();
            awaitNode(test, "native.workspace-list").getBoundsInScreen(list);
            awaitNode(test, "native.workspace.add").getBoundsInScreen(add);
            if (list.bottom > add.top) throw new AssertionError("Workspace list covered add button");
            awaitNode(test, "native.workspace." + fixture).getBoundsInScreen(row);
            swipe(test, target, row.right - 24, row.centerY(), row.left + 24);
            capture(test, "workspaces");
            awaitNode(test, "native.workspace.edit." + fixture).performAction(AccessibilityNodeInfo.ACTION_CLICK);
            awaitNode(test, "native.dialog.add");
            test.runOnMainSync(() -> {
                String error = ((MainActivity) target).saveWorkspace(fixture, "Renamed fixture", "https://ignored.example");
                if (error != null) throw new AssertionError(error);
            });
            if (!"https://swipe-fixture.example".equals(store.find(fixture).getString("baseURL"))) throw new AssertionError("Rename changed workspace URL");
            test.runOnMainSync(() -> ((MainActivity) target).getOnBackPressedDispatcher().onBackPressed());
            result.get().put("nativeUI", "FloatingToolbar, keyboard hides both tab styles, spinner, standalone logs, native clipboard, progress dialog/log, scrollable drawer, swipe edit, rename preserving URL, remote-only without core");
            return result.get();
        } finally {
            test.removeMonitor(monitor);
            if (activity != null) { Activity target = activity; test.runOnMainSync(target::finish); }
            AppPreferences.setRemoteOnly(context, original);
            AppPreferences.floatingTabs(context, floating);
            WorkspaceChecks.restore(context.getSharedPreferences("workspaces", 0), saved);
            WorkspaceChecks.restore(context.getSharedPreferences("application", 0), application);
            TaskScheduler.schedule(context);
        }
    }
    static AccessibilityNodeInfo awaitNode(CoreInstrumentation test, String id) throws Exception {
        long deadline = android.os.SystemClock.elapsedRealtime() + 10000;
        while (android.os.SystemClock.elapsedRealtime() < deadline) {
            AccessibilityNodeInfo root = test.getUiAutomation().getRootInActiveWindow();
            if (root != null) {
                AccessibilityNodeInfo found = findNode(root, id);
                if (found != null) return found;
            }
            Thread.sleep(100);
        }
        throw new AssertionError("Native component not accessible: " + id);
    }
    static void capture(CoreInstrumentation test, String name) throws Exception {
        android.graphics.Bitmap bitmap = test.getUiAutomation().takeScreenshot();
        if (bitmap != null) try (java.io.FileOutputStream output = new java.io.FileOutputStream(new java.io.File(test.getTargetContext().getCacheDir(), "ui-" + name + ".png"))) { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, output); bitmap.recycle(); }
    }
    private static void verifyKeyboard(CoreInstrumentation test, Activity target, boolean floating) throws Exception {
        android.graphics.Rect rect = new android.graphics.Rect(); awaitNode(test, "native.plugin-search").getBoundsInScreen(rect);
        swipe(test, target, rect.centerX(), rect.centerY(), rect.centerX());
        test.runOnMainSync(() -> androidx.core.view.WindowCompat.getInsetsController(target.getWindow(), target.getWindow().getDecorView()).show(WindowInsetsCompat.Type.ime()));
        awaitKeyboard(test, target, true);
        for (int i = 0; i < 50; i++) {
            AccessibilityNodeInfo root = test.getUiAutomation().getRootInActiveWindow();
            if (root != null && findNode(root, "native.tab.0") == null) break;
            if (i == 49) throw new AssertionError("Keyboard pushed native tabs above input area");
            Thread.sleep(100);
        }
        test.runOnMainSync(() -> androidx.core.view.WindowCompat.getInsetsController(target.getWindow(), target.getWindow().getDecorView()).hide(WindowInsetsCompat.Type.ime()));
        awaitKeyboard(test, target, false);
        awaitNode(test, floating ? "native.tabs.floating" : "native.tabs");
    }
    private static void awaitKeyboard(CoreInstrumentation test, Activity target, boolean visible) throws Exception {
        AtomicReference<Boolean> state = new AtomicReference<>(!visible);
        for (int i = 0; i < 100; i++) {
            test.runOnMainSync(() -> { WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(target.getWindow().getDecorView()); state.set(insets != null && insets.isVisible(WindowInsetsCompat.Type.ime())); });
            if (state.get() == visible) return;
            Thread.sleep(50);
        }
        throw new AssertionError("IME visibility did not become " + visible);
    }
    private static void verifyClipboard(CoreInstrumentation test, Activity target) throws Exception {
        java.lang.reflect.Field field = MainActivity.class.getDeclaredField("web"); field.setAccessible(true);
        android.webkit.WebView web = (android.webkit.WebView) field.get(target);
        android.content.ClipboardManager clipboard = target.getSystemService(android.content.ClipboardManager.class);
        AtomicReference<android.content.ClipData> original = new AtomicReference<>();
        test.runOnMainSync(() -> original.set(clipboard.getPrimaryClip()));
        try {
            AtomicReference<Boolean> available = new AtomicReference<>(false);
            for (int i = 0; i < 100 && !available.get(); i++) {
                test.runOnMainSync(() -> {
                    if (target.hasWindowFocus() && web.isShown()) web.evaluateJavascript("Boolean(window.cphPlatform) && document.readyState !== 'loading'", value -> available.set("true".equals(value)));
                }); Thread.sleep(50);
            }
            if (!available.get()) throw new AssertionError("Native clipboard bridge unavailable");
            test.runOnMainSync(() -> web.evaluateJavascript("window.cphPlatform.postMessage(JSON.stringify({type:'clipboard-write',id:'clipboard-check',text:'cph-clipboard-fixture'}))", null));
            AtomicReference<Boolean> copied = new AtomicReference<>(false);
            for (int i = 0; i < 100 && !copied.get(); i++) {
                test.runOnMainSync(() -> { android.content.ClipData value = clipboard.getPrimaryClip(); copied.set(value != null && "cph-clipboard-fixture".contentEquals(value.getItemAt(0).getText())); }); Thread.sleep(30);
            }
            if (!copied.get()) throw new AssertionError("WebView copy did not reach system clipboard");
        } finally { test.runOnMainSync(() -> { if (original.get() != null) clipboard.setPrimaryClip(original.get()); else if (android.os.Build.VERSION.SDK_INT >= 28) clipboard.clearPrimaryClip(); else clipboard.setPrimaryClip(android.content.ClipData.newPlainText("", "")); }); }
    }
    private static void swipe(CoreInstrumentation test, Activity target, int start, int y, int end) throws Exception {
        long down = android.os.SystemClock.uptimeMillis();
        int[] position = new int[2]; test.runOnMainSync(() -> target.getWindow().getDecorView().getLocationOnScreen(position));
        for (int i = 0; i <= 12; i++) {
            int action = i == 0 ? android.view.MotionEvent.ACTION_DOWN : i == 12 ? android.view.MotionEvent.ACTION_UP : android.view.MotionEvent.ACTION_MOVE;
            android.view.MotionEvent event = android.view.MotionEvent.obtain(down, android.os.SystemClock.uptimeMillis(), action, start + (end - start) * i / 12f - position[0], y - position[1], 0);
            test.runOnMainSync(() -> target.getWindow().getDecorView().dispatchTouchEvent(event)); event.recycle();
            Thread.sleep(25);
        }
    }
    static AccessibilityNodeInfo findNode(AccessibilityNodeInfo node, String id) {
        if (id.equals(node.getViewIdResourceName())) return node;
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i);
            if (child != null) { AccessibilityNodeInfo found = findNode(child, id); if (found != null) return found; }
        }
        return null;
    }
}
