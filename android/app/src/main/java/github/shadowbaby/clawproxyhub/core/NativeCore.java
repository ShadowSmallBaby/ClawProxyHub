package github.shadowbaby.clawproxyhub.core;

import android.content.*;
import android.content.pm.*;
import android.os.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicLong;
import org.json.*;

// 宿主从签名安装目录发现插件，以私有进程连接复用业务协议。
public final class NativeCore {
  static {
    System.loadLibrary("cphcore");
  }

  static volatile Context context;
  static volatile JSONObject currentSession;
  static final ExecutorService worker = Executors.newSingleThreadExecutor();
  private static final ConcurrentHashMap<Long, Connection> connections = new ConcurrentHashMap<>();
  private static final AtomicLong next = new AtomicLong();

  static native byte[] start(String dir);

  static native byte[] sync();

  static native byte[] stop();

  static native long newJob();

  static native byte[] due(long id);

  static native void cancel(long id);

  static synchronized void initialize(Context value) {
    if (context == null) context = value.getApplicationContext();
  }

  static JSONObject session() throws Exception {
    if (AppPreferences.remoteOnly(context))
      throw new IllegalStateException("Local mode is disabled");
    PackageAssets.prepare(context);
    JSONObject result = decode(start(context.getFilesDir().getAbsolutePath()));
    currentSession = result;
    CoreState.started = true;
    return result;
  }

  static JSONObject decode(byte[] bytes) throws Exception {
    JSONObject result = new JSONObject(new String(bytes, StandardCharsets.UTF_8));
    if (result.has("error")) throw new IllegalStateException(result.getString("error"));
    return result;
  }

  static native byte[] refresh(String name);

  static native byte[] runtime(String request);

  public static String discover() throws Exception {
    return new NativePluginStore(context).discover();
  }

  private static final boolean[] slots = new boolean[32];

  private static synchronized int acquireSlot() {
    for (int i = 0; i < slots.length; i++)
      if (!slots[i]) {
        slots[i] = true;
        return i;
      }
    throw new IllegalStateException("Plugin worker limit reached");
  }

  private static synchronized void releaseSlot(int slot) {
    if (slot >= 0) slots[slot] = false;
  }

  static byte[] readBounded(java.io.InputStream input, int max) throws Exception {
    java.io.ByteArrayOutputStream output = new java.io.ByteArrayOutputStream();
    byte[] buffer = new byte[16384];
    int read;
    while ((read = input.read(buffer)) != -1) {
      if (output.size() + read > max) throw new IllegalArgumentException("file too large");
      output.write(buffer, 0, read);
    }
    return output.toByteArray();
  }

  public static String systemComponents() throws Exception {
    String pkg = context.getPackageName();
    PackageInfo info = context.getPackageManager().getPackageInfo(pkg, 0);
    JSONObject core =
        new JSONObject()
            .put("id", "android.core")
            .put("name", "ClawProxyHub")
            .put("package", pkg)
            .put("version", info.versionName)
            .put("installed", true)
            .put("available", true)
            .put("execution", "android-service");
    return new JSONArray().put(core).toString();
  }

  public static boolean hasLua() {
    try {
      ServiceInfo service =
          context
              .getPackageManager()
              .getServiceInfo(
                  new ComponentName(
                      context.getPackageName(), "github.shadowbaby.clawproxyhub.lua.LuaService"),
                  0);
      return service.enabled && !service.exported && new HostStore(context).enabled();
    } catch (Exception ignored) {
      return false;
    }
  }

  public static String extensionTrust() throws Exception {
    JSONObject trust = new JSONObject();
    try (java.io.InputStream input = context.getAssets().open("packages/trust.json")) {
      trust = new JSONObject(new String(readBounded(input, 1024 * 1024), StandardCharsets.UTF_8));
    } catch (java.io.FileNotFoundException ignored) {
    }
    PackageInfo info =
        context
            .getPackageManager()
            .getPackageInfo(
                context.getPackageName(),
                Build.VERSION.SDK_INT >= 28
                    ? PackageManager.GET_SIGNING_CERTIFICATES
                    : PackageManager.GET_SIGNATURES);
    android.content.pm.Signature[] signers =
        Build.VERSION.SDK_INT >= 28 ? info.signingInfo.getApkContentsSigners() : info.signatures;
    for (android.content.pm.Signature signer : signers) {
      byte[] cert = signer.toByteArray();
      trust.put(
          "android-app:" + NativePluginStore.hash(cert),
          new JSONObject()
              .put(
                  "certificate",
                  android.util.Base64.encodeToString(cert, android.util.Base64.NO_WRAP))
              .put("publisher", context.getPackageName())
              .put("ids", new JSONArray().put("lua-runtime"))
              .put("permissions", new JSONArray().put("runtime.execute"))
              .put("native", true));
    }
    return trust.toString();
  }

  public static long[] open(String directory) throws Exception {
    return openRuntime(directory, null);
  }

  public static long[] openRuntime(String directory, String runtimeArchive) throws Exception {
    java.io.File dir = new java.io.File(directory);
    String name = dir.getName();
    JSONObject manifest;
    try (java.io.InputStream input =
        new java.io.FileInputStream(new java.io.File(dir, "manifest.json"))) {
      manifest =
          new JSONObject(new String(readBounded(input, 1024 * 1024), StandardCharsets.UTF_8));
    }
    boolean lua = "lua".equals(manifest.optString("runtime"));
    if (!name.matches("[a-z][a-z0-9_-]*")) throw new SecurityException("invalid plugin name");
    if (runtimeArchive != null) {
      if (!lua) throw new SecurityException("runtime probe requires Lua");
      new HostStore(context).library(new java.io.File(runtimeArchive));
    }
    Connection connection = new Connection(name, lua, lua ? bundle(dir) : null, runtimeArchive);
    long id = next.incrementAndGet();
    connections.put(id, connection);
    try {
      return connection.open(id);
    } catch (Exception e) {
      release(id);
      throw e;
    }
  }

  public static void release(long id) {
    Connection value = connections.remove(id);
    if (value != null) value.close();
  }

  public static long[] openExtension(String path, String digest, int minSdk) throws Exception {
    return ExtensionConnections.open(context, path, digest, minSdk);
  }

  public static void releaseExtension(long id) {
    ExtensionConnections.close(id);
  }

  private static java.io.File bundle(java.io.File dir) throws Exception {
    java.io.File output =
        java.io.File.createTempFile("lua-transfer-", ".zip", context.getCacheDir());
    boolean success = false;
    try (java.util.zip.ZipOutputStream zip =
        new java.util.zip.ZipOutputStream(new java.io.FileOutputStream(output))) {
      int[] total = {0};
      addFile(zip, dir, new java.io.File(dir, "manifest.json"), total);
      addFile(zip, dir, new java.io.File(dir, "main.lua"), total);
      java.io.File lib = new java.io.File(dir, "lib");
      if (lib.isDirectory()) addFile(zip, dir, lib, total);
      success = true;
    } finally {
      if (!success) output.delete();
    }
    return output;
  }

  private static void addFile(
      java.util.zip.ZipOutputStream zip, java.io.File root, java.io.File file, int[] total)
      throws Exception {
    String base = root.getCanonicalPath() + java.io.File.separator, path = file.getCanonicalPath();
    if (!path.startsWith(base)) throw new SecurityException("script path outside workspace");
    if (file.isDirectory()) {
      java.io.File[] children = file.listFiles();
      if (children != null) for (java.io.File child : children) addFile(zip, root, child, total);
      return;
    }
    String name = path.substring(base.length()).replace(java.io.File.separatorChar, '/');
    if (!name.equals("manifest.json") && !name.equals("main.lua") && !name.endsWith(".lua")) return;
    byte[] content;
    try (java.io.InputStream input = new java.io.FileInputStream(file)) {
      content = readBounded(input, 8 * 1024 * 1024 - total[0]);
    }
    total[0] += content.length;
    zip.putNextEntry(new java.util.zip.ZipEntry(name));
    zip.write(content);
    zip.closeEntry();
  }

  private static final ExecutorService calls =
      new ThreadPoolExecutor(0, 40, 30, TimeUnit.SECONDS, new SynchronousQueue<>());

  private static final class Connection implements ServiceConnection {
    final String name;
    final int slot;
    final boolean lua;
    final java.io.File bundle;
    final String runtimeArchive;
    final String protocol;
    final CompletableFuture<IBinder> ready = new CompletableFuture<>();
    final CompletableFuture<Void> dead = new CompletableFuture<>();
    final IBinder owner = new Binder();
    final java.util.concurrent.atomic.AtomicBoolean closed =
        new java.util.concurrent.atomic.AtomicBoolean();
    volatile IBinder service;
    volatile boolean bound;
    boolean slotReleased;
    int pid;
    long remote;
    boolean failed;

    Connection(String name, boolean lua, java.io.File bundle, String runtimeArchive) {
      this.name = name;
      this.slot = lua ? -1 : acquireSlot();
      this.lua = lua;
      this.bundle = bundle;
      this.runtimeArchive = runtimeArchive;
      protocol =
          lua
              ? "github.shadowbaby.clawproxyhub.lua.v1"
              : "github.shadowbaby.clawproxyhub.plugin.v1";
    }

    private <T> T call(Callable<T> action, long timeout) throws Exception {
      Future<T> pending = calls.submit(action);
      try {
        return pending.get(timeout, TimeUnit.MILLISECONDS);
      } catch (Exception error) {
        failed = true;
        pending.cancel(true);
        throw error;
      }
    }

    long[] open(long id) throws Exception {
      Intent intent =
          new Intent()
              .setComponent(
                  new ComponentName(
                      context.getPackageName(),
                      lua
                          ? "github.shadowbaby.clawproxyhub.lua.LuaService"
                              + (runtimeArchive == null ? "" : "$Probe")
                          : "github.shadowbaby.clawproxyhub.plugin.PluginService$Worker" + slot));
      bound = context.bindService(intent, this, Context.BIND_AUTO_CREATE);
      if (!bound) throw new IllegalStateException("plugin bind failed");
      service = ready.get(10, TimeUnit.SECONDS);
      pid =
          call(
              () -> {
                Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                try {
                  request.writeInterfaceToken(protocol);
                  if (!service.transact(IBinder.FIRST_CALL_TRANSACTION + 2, request, reply, 0))
                    throw new IllegalStateException("missing plugin worker identity");
                  reply.readException();
                  return reply.readInt();
                } finally {
                  request.recycle();
                  reply.recycle();
                }
              },
              5000);
      if (pid <= 0 || pid == android.os.Process.myPid())
        throw new SecurityException("plugin requires a private process");
      ParcelFileDescriptor[] requests = ParcelFileDescriptor.createSocketPair();
      try (ParcelFileDescriptor a = requests[0];
          ParcelFileDescriptor b = requests[1]) {
        ParcelFileDescriptor[] callbacks = ParcelFileDescriptor.createSocketPair();
        try (ParcelFileDescriptor c = callbacks[0];
            ParcelFileDescriptor d = callbacks[1]) {
          remote =
              call(
                  () -> {
                    Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                    try {
                      request.writeInterfaceToken(protocol);
                      request.writeStrongBinder(owner);
                      b.writeToParcel(request, 0);
                      d.writeToParcel(request, 0);
                      if (lua) {
                        try (ParcelFileDescriptor scripts =
                            ParcelFileDescriptor.open(
                                bundle, ParcelFileDescriptor.MODE_READ_ONLY)) {
                          scripts.writeToParcel(request, 0);
                        }
                        if (runtimeArchive != null) request.writeString(runtimeArchive);
                      } else request.writeString(name);
                      if (!service.transact(IBinder.FIRST_CALL_TRANSACTION, request, reply, 0))
                        throw new IllegalStateException("unsupported plugin protocol");
                      reply.readException();
                      return reply.readLong();
                    } finally {
                      request.recycle();
                      reply.recycle();
                    }
                  },
                  10000);
          if (remote == 0) throw new IllegalStateException("empty plugin session");
          return new long[] {a.detachFd(), c.detachFd(), id};
        }
      }
    }

    public synchronized void onServiceConnected(ComponentName name, IBinder binder) {
      if (closed.get()) return;
      service = binder;
      try {
        binder.linkToDeath(
            () -> {
              dead.complete(null);
              if (!bound) releaseWorker();
            },
            0);
        ready.complete(binder);
      } catch (RemoteException error) {
        dead.complete(null);
        ready.completeExceptionally(error);
        if (!bound) releaseWorker();
      }
    }

    public void onServiceDisconnected(ComponentName name) {
      ready.completeExceptionally(new IllegalStateException("plugin disconnected"));
    }

    public void onBindingDied(ComponentName name) {
      onServiceDisconnected(name);
    }

    public void onNullBinding(ComponentName name) {
      onServiceDisconnected(name);
    }

    synchronized void releaseWorker() {
      if (!slotReleased) {
        slotReleased = true;
        releaseSlot(slot);
      }
    }

    synchronized void close() {
      if (!closed.compareAndSet(false, true)) return;
      if (service != null && remote != 0 && service.isBinderAlive() && !failed) {
        try {
          call(
              () -> {
                Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                try {
                  request.writeInterfaceToken(protocol);
                  request.writeLong(remote);
                  if (!service.transact(IBinder.FIRST_CALL_TRANSACTION + 1, request, reply, 0))
                    throw new IllegalStateException("unsupported plugin close");
                  reply.readException();
                  return null;
                } finally {
                  request.recycle();
                  reply.recycle();
                }
              },
              500);
        } catch (Exception ignored) {
          failed = true;
        }
      }
      if (bound) {
        bound = false;
        context.unbindService(this);
      }
      boolean last =
          !lua
              || runtimeArchive != null
              || connections.values().stream().noneMatch(c -> c.lua && c.runtimeArchive == null);
      // 超时后终止整个私有进程，迟到调用及其 FD 一并撤销；共享 Lua 会话通过断开的 socket 感知失败。
      if (service != null && (failed || last)) {
        if (pid > 0 && pid != android.os.Process.myPid() && service.isBinderAlive())
          android.os.Process.killProcess(pid);
        try {
          dead.get(2, TimeUnit.SECONDS);
        } catch (Exception ignored) {
          /* 死亡回调继续负责回收。 */
        }
      }
      if (service == null || dead.isDone() || !service.isBinderAlive()) releaseWorker();
      if (bundle != null) bundle.delete();
    }
  }
}
