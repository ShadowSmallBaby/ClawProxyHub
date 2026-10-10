package github.shadowbaby.clawproxyhub.core;

import android.app.Activity;
import android.graphics.Color;
import android.view.View;
import android.view.WindowManager;
import android.widget.FrameLayout;
import androidx.core.graphics.Insets;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowCompat;
import androidx.core.view.WindowInsetsCompat;

// 原生容器统一避让系统栏、挖孔和键盘，并消费 Insets，防止 WebView 再计一次状态栏。
final class WindowLayout {
    static void appearance(Activity activity, int color, boolean light) {
        android.view.ViewGroup frame = activity.findViewById(android.R.id.content);
        if (frame.getChildCount() == 0) return;
        View container = frame.getChildAt(0); container.setBackgroundColor(color);
        WindowCompat.getInsetsController(activity.getWindow(), container).setAppearanceLightStatusBars(light);
        WindowCompat.getInsetsController(activity.getWindow(), container).setAppearanceLightNavigationBars(light);
    }
    static void setContent(Activity activity, View content, java.util.function.Consumer<Boolean> keyboard) {
        WindowCompat.setDecorFitsSystemWindows(activity.getWindow(), false);
        activity.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        activity.getWindow().setStatusBarColor(Color.TRANSPARENT);
        activity.getWindow().setNavigationBarColor(Color.TRANSPARENT);
        FrameLayout container = new FrameLayout(activity);
        container.setBackgroundColor(Color.WHITE);
        container.addView(content, new FrameLayout.LayoutParams(-1, -1));
        ViewCompat.setOnApplyWindowInsetsListener(container, (view, insets) -> {
            keyboard.accept(insets.isVisible(WindowInsetsCompat.Type.ime()));
            Insets safe = insets.getInsets(WindowInsetsCompat.Type.systemBars()
                    | WindowInsetsCompat.Type.displayCutout() | WindowInsetsCompat.Type.ime());
            view.setPadding(safe.left, safe.top, safe.right, safe.bottom);
            return WindowInsetsCompat.CONSUMED;
        });
        activity.setContentView(container);
        WindowCompat.getInsetsController(activity.getWindow(), container).setAppearanceLightStatusBars(true);
        WindowCompat.getInsetsController(activity.getWindow(), container).setAppearanceLightNavigationBars(true);
        ViewCompat.requestApplyInsets(container);
    }
}
