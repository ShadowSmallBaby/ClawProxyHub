package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.SharedPreferences;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.*;

// 只记录原生操作与生命周期，不收集系统 logcat、请求正文或认证信息。
public final class AppLog {
    private static SharedPreferences preferences(Context context) { return context.getSharedPreferences("android-logs", 0); }
    public static boolean enabled(Context context) { return preferences(context).getBoolean("enabled", false); }
    public static String level(Context context) { return preferences(context).getString("level", "info"); }
    public static int days(Context context) { return preferences(context).getInt("days", 7); }
    public static synchronized void configure(Context context, boolean enabled, String level, int days) {
        if (!Arrays.asList("debug", "info", "warn", "error").contains(level) || !Arrays.asList(1, 3, 7, 14, 30).contains(days)) throw new IllegalArgumentException("Invalid logging preference");
        if (!preferences(context).edit().putBoolean("enabled", enabled).putString("level", level).putInt("days", days).commit()) throw new IllegalStateException("Cannot save logging preference");
        prune(context);
    }
    private static File root(Context context) { return new File(context.getFilesDir(), "android-logs"); }
    private static int rank(String value) { return Arrays.asList("debug", "info", "warn", "error").indexOf(value); }
    public static synchronized void write(Context context, String level, String component, String event) {
        if (!enabled(context) || rank(level) < rank(level(context))) return;
        try {
            File root = root(context); if (!root.isDirectory() && !root.mkdirs()) return;
            prune(context);
            String process = Integer.toString(android.os.Process.myPid());
            File file = new File(root, new SimpleDateFormat("yyyy-MM-dd", Locale.ROOT).format(new Date()) + "-" + process + ".log");
            if (file.length() >= 1024 * 1024) return;
            String line = new SimpleDateFormat("HH:mm:ss", Locale.ROOT).format(new Date()) + " " + level.toUpperCase(Locale.ROOT) + " " + component + " " + event.replace('\n', ' ').replace('\r', ' ') + "\n";
            try (FileOutputStream output = new FileOutputStream(file, true)) { output.write(line.getBytes(StandardCharsets.UTF_8)); }
        } catch (IOException ignored) { /* 日志存储失败不影响应用运行。 */ }
    }
    private static void prune(Context context) {
        File[] files = root(context).listFiles(); if (files == null) return;
        long cutoff = System.currentTimeMillis() - days(context) * 86400000L;
        Arrays.sort(files, Comparator.comparingLong(File::lastModified).reversed());
        long total = 0;
        for (File file : files) if (file.isFile()) {
            total += file.length();
            if (file.lastModified() < cutoff || total > 8 * 1024 * 1024) file.delete();
        }
    }
    static synchronized String read(Context context) throws IOException {
        prune(context); File[] files = root(context).listFiles(); if (files == null) return "";
        Arrays.sort(files, Comparator.comparingLong(File::lastModified).reversed());
        StringBuilder result = new StringBuilder(); int remaining = 256 * 1024;
        for (File file : files) {
            if (!file.isFile() || remaining <= 0) continue;
            try (RandomAccessFile input = new RandomAccessFile(file, "r")) {
                int count = (int) Math.min(input.length(), remaining); input.seek(input.length() - count);
                byte[] bytes = new byte[count]; input.readFully(bytes); remaining -= count;
                result.append(file.getName()).append('\n').append(new String(bytes, StandardCharsets.UTF_8)).append('\n');
            }
        }
        return result.toString();
    }
    static synchronized void clear(Context context) { File[] files = root(context).listFiles(); if (files != null) for (File file : files) if (file.isFile()) file.delete(); }
}
