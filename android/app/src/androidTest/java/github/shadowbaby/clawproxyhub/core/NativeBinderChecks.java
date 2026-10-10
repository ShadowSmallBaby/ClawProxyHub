package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.os.ParcelFileDescriptor;
import android.os.SystemClock;
import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.TimeoutException;
import org.json.JSONObject;

// 用测试原生库的阻塞标记验证真正的跨进程超时、槽位回收及再次加载。
final class NativeBinderChecks {
  static JSONObject run(Context context) throws Exception {
    NativeCore.initialize(context);
    File directory = new File(context.getCacheDir(), "binder-check/androidtest");
    if (!directory.mkdirs() && !directory.isDirectory())
      throw new AssertionError("fixture directory");
    File manifest = new File(directory, "manifest.json");
    try (FileOutputStream output = new FileOutputStream(manifest)) {
      output.write("{}".getBytes(StandardCharsets.UTF_8));
    }
    File temporary = new File(context.getCacheDir(), "native-plugins/androidtest");
    if (!temporary.mkdirs() && !temporary.isDirectory())
      throw new AssertionError("native directory");
    File openMarker = new File(temporary, "stall-open");
    File closeMarker = new File(temporary, "stall-close");
    int before = usedSlots();
    long opening, closing;
    try {
      if (!openMarker.createNewFile()) throw new AssertionError("unexpected open marker");
      long start = SystemClock.elapsedRealtime();
      try {
        long[] unexpected = NativeCore.open(directory.getPath());
        release(unexpected);
        throw new AssertionError("stalled initialization succeeded");
      } catch (TimeoutException expected) {
        opening = SystemClock.elapsedRealtime() - start;
        if (opening < 9000 || opening > 30000)
          throw new AssertionError("open deadline: " + opening);
      }
      if (!openMarker.delete()) throw new AssertionError("remove open marker");
      long[] session = NativeCore.open(directory.getPath());
      try {
        if (!closeMarker.createNewFile()) throw new AssertionError("unexpected close marker");
        start = SystemClock.elapsedRealtime();
        NativeCore.release(session[2]);
        closing = SystemClock.elapsedRealtime() - start;
        if (closing < 400 || closing > 5000) throw new AssertionError("close deadline: " + closing);
      } finally {
        release(session);
      }
      if (!closeMarker.delete()) throw new AssertionError("remove close marker");
      release(NativeCore.open(directory.getPath()));
      if (usedSlots() != before) throw new AssertionError("worker slot leaked");
      return new JSONObject()
          .put("open_timeout_ms", opening)
          .put("close_timeout_ms", closing)
          .put("reopen", true)
          .put("worker_slots_reclaimed", true);
    } finally {
      openMarker.delete();
      closeMarker.delete();
      manifest.delete();
      directory.delete();
      directory.getParentFile().delete();
    }
  }

  private static void release(long[] session) throws Exception {
    try (ParcelFileDescriptor requests = ParcelFileDescriptor.adoptFd((int) session[0]);
        ParcelFileDescriptor callbacks = ParcelFileDescriptor.adoptFd((int) session[1])) {
      NativeCore.release(session[2]);
    }
  }

  private static int usedSlots() throws Exception {
    java.lang.reflect.Field field = NativeCore.class.getDeclaredField("slots");
    field.setAccessible(true);
    synchronized (NativeCore.class) {
      int count = 0;
      for (boolean used : (boolean[]) field.get(null)) if (used) count++;
      return count;
    }
  }
}
