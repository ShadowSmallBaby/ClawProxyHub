package github.shadowbaby.clawproxyhub.core;

import android.app.job.JobParameters;
import android.app.job.JobService;
import android.os.Build;
import android.os.SystemClock;
import java.util.concurrent.ConcurrentHashMap;

// 系统周期为尽力调度，不承诺锁屏精准唤醒或进程永久存活。
public final class TaskJobService extends JobService {
    private static final String TAG = "CPH-task";
    private final ConcurrentHashMap<Integer, Long> running = new ConcurrentHashMap<>();

    @Override public boolean onStartJob(JobParameters params) {
        if (AppPreferences.remoteOnly(this)) return false;
        NativeCore.initialize(this);
        long execution = NativeCore.newJob();
        long started = SystemClock.elapsedRealtime();
        running.put(params.getJobId(), execution);
        AppLog.write(this, "info", TAG, "system job started: job=" + params.getJobId() + " execution=" + execution);
        new Thread(() -> {
            boolean retry = false;
            String phase = "start";
            try {
                NativeCore.session();
                phase = "sync";
                NativeCore.decode(NativeCore.sync());
                phase = "due";
                NativeCore.decode(NativeCore.due(execution));
                if (running.containsKey(params.getJobId())) AppNotifications.local(this);
            } catch (Exception e) {
                AppLog.write(this, "warn", TAG, "system job failed: execution=" + execution + " phase=" + phase + " error=" + e.getClass().getSimpleName());
                retry = true;
            } finally {
                NativeCore.cancel(execution);
                // 被系统停止或替换的执行不能完成同编号的新任务。
                if (running.remove(params.getJobId(), execution)) {
                    AppLog.write(this, "info", TAG, "system job finished: execution=" + execution + " retry=" + retry
                            + " elapsed_ms=" + (SystemClock.elapsedRealtime() - started));
                    jobFinished(params, retry);
                }
            }
        }, "cph-system-job").start();
        return true;
    }

    @Override public boolean onStopJob(JobParameters params) {
        Long execution = running.remove(params.getJobId());
        // 保留 Android 的停止原因，便于把省电/网络/配额限制与业务错误分开诊断。
        String reason = Build.VERSION.SDK_INT >= 31 ? Integer.toString(params.getStopReason()) : "unavailable";
        AppLog.write(this, "warn", TAG, "system job stopped: job=" + params.getJobId() + " execution=" + execution
                + " stop_reason=" + reason);
        if (execution != null) NativeCore.cancel(execution);
        return false;
    }
}
