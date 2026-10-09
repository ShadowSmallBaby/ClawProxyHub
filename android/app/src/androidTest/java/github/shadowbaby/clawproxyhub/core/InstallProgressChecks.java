package github.shadowbaby.clawproxyhub.core;

import android.app.Activity;
import android.app.Instrumentation;
import android.graphics.Rect;
import android.os.SystemClock;
import android.view.accessibility.AccessibilityNodeInfo;
import org.json.JSONArray;
import org.json.JSONObject;

// 只模拟界面进度，不安装包或修改插件、运行时与用户配置。
final class InstallProgressChecks {
    static JSONObject run(CoreInstrumentation test) throws Exception {
        android.accessibilityservice.AccessibilityServiceInfo service = test.getUiAutomation().getServiceInfo();
        service.flags |= android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS;
        test.getUiAutomation().setServiceInfo(service);
        Instrumentation.ActivityMonitor monitor = test.addMonitor(MainActivity.class.getName(), null, false);
        Activity activity = null;
        try {
            String component = test.getTargetContext().getPackageName() + "/" + MainActivity.class.getName();
            try (android.os.ParcelFileDescriptor launch = test.getUiAutomation().executeShellCommand("am start -W -n " + component);
                 java.io.InputStream input = new android.os.ParcelFileDescriptor.AutoCloseInputStream(launch)) {
                NativePluginStore.read(input, 4096);
            }
            activity = test.waitForMonitorWithTimeout(monitor, 15000);
            if (activity == null) throw new AssertionError("application window did not open");
            java.lang.reflect.Field field = MainActivity.class.getDeclaredField("pages");
            field.setAccessible(true);
            NativePages pages = (NativePages) field.get(activity);
            test.runOnMainSync(() -> pages.selectTab(0));
            WindowChecks.awaitNode(test, "native.plugin-tabs");
            test.runOnMainSync(() -> { pages.installing("UI check"); pages.installProgress("downloading", 37, 100); });
            WindowChecks.awaitNode(test, "native.install-log");
            click(test, "native.dialog.close");
            Rect button = new Rect(), tabs = new Rect();
            WindowChecks.awaitNode(test, "native.plugin-progress").getBoundsInScreen(button);
            WindowChecks.awaitNode(test, "native.plugin-tabs").getBoundsInScreen(tabs);
            if (button.top <= tabs.bottom || button.width() >= tabs.width() / 2)
                throw new AssertionError("installation details still occupy the top of the list");
            WindowChecks.capture(test, "install-floating");
            click(test, "native.plugin-progress");
            WindowChecks.awaitNode(test, "native.install-log");
            test.runOnMainSync(pages::back);
            WindowChecks.awaitNode(test, "native.plugin-progress");
            test.runOnMainSync(() -> pages.installComplete("UI check"));
            click(test, "native.plugin-progress");
            WindowChecks.awaitNode(test, "native.install-log");
            click(test, "native.dialog.close");
            awaitNoProgress(test);

            test.runOnMainSync(() -> pages.installing("UI check"));
            WindowChecks.awaitNode(test, "native.install-log");
            test.runOnMainSync(() -> pages.installComplete("UI check"));
            test.runOnMainSync(pages::back);
            awaitNoProgress(test);

            test.runOnMainSync(() -> pages.installing("UI check"));
            WindowChecks.awaitNode(test, "native.install-log");
            click(test, "native.dialog.close");
            WindowChecks.awaitNode(test, "native.plugin-progress");
            test.runOnMainSync(() -> pages.installFailed("UI check failure"));
            WindowChecks.awaitNode(test, "native.install-log");
            click(test, "native.dialog.close");
            awaitNoProgress(test);

            test.runOnMainSync(() -> pages.updateResult(null, null));
            Rect loading = new Rect(), spinner = new Rect();
            WindowChecks.awaitNode(test, "native.update-loading").getBoundsInScreen(loading);
            WindowChecks.awaitNode(test, "native.update-spinner").getBoundsInScreen(spinner);
            if (loading.width() <= spinner.width() || Math.abs(loading.centerX() - spinner.centerX()) > 2)
                throw new AssertionError("update spinner is not centered");
            WindowChecks.capture(test, "update-loading");
            click(test, "native.dialog.close");
            return new JSONObject().put("ok", true).put("floating_bounds", button.toShortString()).put("loading_bounds", loading.toShortString()).put("checks", new JSONArray(new String[]{
                    "floating-details", "reopen-during-install", "dismiss-viewed-result", "complete-while-open",
                    "dismiss-failure", "new-operation-restores-details", "centered-update-spinner"}));
        } finally {
            test.removeMonitor(monitor);
            if (activity != null) { Activity target = activity; test.runOnMainSync(target::finish); }
        }
    }

    private static void click(CoreInstrumentation test, String id) throws Exception {
        if (!WindowChecks.awaitNode(test, id).performAction(AccessibilityNodeInfo.ACTION_CLICK))
            throw new AssertionError("Native component cannot be clicked: " + id);
    }

    private static void awaitNoProgress(CoreInstrumentation test) throws Exception {
        WindowChecks.awaitNode(test, "native.plugin-tabs");
        long deadline = SystemClock.elapsedRealtime() + 10000;
        while (SystemClock.elapsedRealtime() < deadline) {
            AccessibilityNodeInfo root = test.getUiAutomation().getRootInActiveWindow();
            if (root != null && WindowChecks.findNode(root, "native.plugin-progress") == null) return;
            Thread.sleep(100);
        }
        throw new AssertionError("viewed installation result remains visible");
    }
}
