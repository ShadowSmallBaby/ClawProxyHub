package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.graphics.Rect;
import android.os.Build;
import android.view.MotionEvent;
import android.widget.FrameLayout;
import java.util.Collections;

// 左边缘手势只在超过阈值后接管触摸，网页的普通滚动与点击保持原有行为。
final class WorkspaceView extends FrameLayout {
    private final Runnable openWorkspaces;
    private final float density;
    private float startX, startY;
    private boolean edge, intercepted;
    WorkspaceView(Context context, Runnable openWorkspaces) {
        super(context); this.openWorkspaces = openWorkspaces; density = getResources().getDisplayMetrics().density;
    }
    @Override protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        // 中部保留一小段应用侧栏手势区，其余边缘仍交给系统返回手势。
        if (Build.VERSION.SDK_INT >= 29) setSystemGestureExclusionRects(Collections.singletonList(new Rect(0, Math.max(0, h / 2 - (int)(80 * density)), (int)(24 * density), Math.min(h, h / 2 + (int)(80 * density)))));
    }
    @Override public boolean dispatchTouchEvent(MotionEvent event) {
        if (event.getActionMasked() == MotionEvent.ACTION_DOWN) {
            startX = event.getX(); startY = event.getY(); edge = startX <= 24 * density; intercepted = false;
        } else if (event.getActionMasked() == MotionEvent.ACTION_MOVE && edge && !intercepted) {
            float dx = event.getX() - startX, dy = Math.abs(event.getY() - startY);
            if (dy > 24 * density && dy > Math.abs(dx)) edge = false;
            if (dx > 48 * density && dx > dy * 2) {
                MotionEvent cancel = MotionEvent.obtain(event); cancel.setAction(MotionEvent.ACTION_CANCEL);
                super.dispatchTouchEvent(cancel); cancel.recycle(); intercepted = true; openWorkspaces.run();
            }
        }
        if (intercepted) return true;
        return super.dispatchTouchEvent(event);
    }
}
