package github.shadowbaby.clawproxyhub.core;

import android.app.job.JobInfo;
import android.app.job.JobScheduler;
import android.content.ComponentName;
import android.content.Context;

final class TaskScheduler {
    static final int JOB_ID = 1001;
    static void setRemoteOnly(Context context, boolean value) throws Exception {
        boolean previous = AppPreferences.remoteOnly(context);
        if (value == previous) return;
        AppPreferences.setRemoteOnly(context, value);
        try {
            schedule(context);
            if (value && CoreState.started) { NativeCore.decode(NativeCore.stop()); CoreState.started = false; }
        } catch (Exception error) {
            AppPreferences.setRemoteOnly(context, previous);
            schedule(context);
            throw error;
        }
    }
    static void schedule(Context context) {
        JobScheduler jobs = context.getSystemService(JobScheduler.class);
        if (AppPreferences.remoteOnly(context)) { jobs.cancel(JOB_ID); return; }
        if (jobs.getPendingJob(JOB_ID) == null) {
            int result = jobs.schedule(new JobInfo.Builder(JOB_ID, new ComponentName(context, TaskJobService.class))
                    .setPeriodic(15 * 60 * 1000L).setPersisted(true)
                    .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY).build());
            if (result != JobScheduler.RESULT_SUCCESS) throw new IllegalStateException("Cannot schedule local tasks");
        }
    }
}
