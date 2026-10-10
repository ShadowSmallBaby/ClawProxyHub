package github.shadowbaby.clawproxyhub.core;

import android.app.*;
import android.content.*;
import android.net.Uri;
import android.os.Build;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import java.util.*;
import org.json.*;

// 同一工作台的提醒合并展示，按消息身份去重；系统通知不改变站内已读状态。
final class AppNotifications {
    static final String CHANNEL = "workspace-messages";
    static boolean system(Context context) { return context.getSharedPreferences("application", 0).getBoolean("system-notifications", true); }
    static void system(Context context, boolean value) {
        context.getSharedPreferences("application", 0).edit().putBoolean("system-notifications", value).apply();
        if (!value) NotificationManagerCompat.from(context).cancelAll();
    }
    static boolean available(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(new NotificationChannel(CHANNEL, AppPreferences.locale(context).equals("en") ? "Workspace notifications" : "工作台通知", NotificationManager.IMPORTANCE_DEFAULT));
            if (manager.getNotificationChannel(CHANNEL).getImportance() == NotificationManager.IMPORTANCE_NONE) return false;
        }
        return NotificationManagerCompat.from(context).areNotificationsEnabled();
    }
    private static String tag(String workspace) { return "workspace-" + workspace; }
    static String identity(JSONObject message) { return message.optLong("id") + ":" + message.optString("created_at"); }
    static List<JSONObject> pending(JSONArray messages, Set<String> seen) throws JSONException {
        if (messages.length() > 200) throw new IllegalArgumentException("Too many notifications");
        List<JSONObject> pending = new ArrayList<>();
        for (int i = 0; i < messages.length(); i++) {
            JSONObject message = messages.getJSONObject(i);
            if (message.optLong("id") > 0 && !message.optBoolean("read") && !seen.contains(identity(message))) pending.add(message);
        }
        return pending;
    }
    static synchronized boolean deliver(Context context, String workspace, String name, JSONArray messages, int unread) throws Exception {
        if (!system(context) || !available(context)) return false;
        SharedPreferences preferences = context.getSharedPreferences("notification-history", 0);
        Set<String> seen = new LinkedHashSet<>(preferences.getStringSet(workspace, Collections.emptySet()));
        List<JSONObject> fresh = pending(messages, seen);
        NotificationManagerCompat manager = NotificationManagerCompat.from(context);
        if (unread == 0) manager.cancel(tag(workspace), 1);
        if (fresh.isEmpty()) return true;
        JSONObject latest = fresh.get(0);
        Intent intent = new Intent(context, MainActivity.class).setAction("cph.notification")
                .setData(Uri.parse("cph://notification/" + Uri.encode(workspace)))
                .putExtra("notification-workspace", workspace).putExtra("notification-id", latest.getLong("id"))
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent open = PendingIntent.getActivity(context, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        String title = latest.optString("title"), content = latest.optString("content");
        if (title.length() > 200) title = title.substring(0, 200);
        if (content.length() > 2000) content = content.substring(0, 2000);
        Notification notification = new NotificationCompat.Builder(context, CHANNEL).setSmallIcon(R.drawable.tab_workspace)
                .setContentTitle(title).setContentText(content).setSubText(name).setNumber(Math.max(0, unread))
                .setStyle(new NotificationCompat.BigTextStyle().bigText(content)).setContentIntent(open).setAutoCancel(true)
                .setVisibility(NotificationCompat.VISIBILITY_PRIVATE).setOnlyAlertOnce(false).build();
        if (Build.VERSION.SDK_INT >= 33 && context.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) return false;
        manager.notify(tag(workspace), 1, notification);
        for (JSONObject item : fresh) seen.add(identity(item));
        // 保留最近一批身份，避免每次轮询或应用重启重复提醒。
        if (seen.size() > 400) { seen.clear(); for (int i = 0; i < messages.length(); i++) seen.add(identity(messages.getJSONObject(i))); }
        preferences.edit().putStringSet(workspace, seen).apply();
        return true;
    }
    static void local(Context context) {
        if (!system(context) || !available(context) || AppPreferences.remoteOnly(context)) return;
        try {
            JSONObject result = CoreApi.notifications();
            if (result != null) deliver(context, WorkspaceStore.LOCAL, AppPreferences.locale(context).equals("en") ? "Built-in workspace" : "内置工作台", result.getJSONArray("notifications"), result.optInt("unread"));
        } catch (Exception error) { AppLog.write(context, "warn", "notifications", "could not read workspace messages"); }
    }
}
