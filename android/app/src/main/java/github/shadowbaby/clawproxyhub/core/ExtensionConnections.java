package github.shadowbaby.clawproxyhub.core;

import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.os.Binder;
import android.os.IBinder;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;

// 槽位仅在原生进程退出后回收，失败安装不会把新连接交给旧版本进程。
final class ExtensionConnections {
    private static final String PROTOCOL = "github.shadowbaby.clawproxyhub.extension.v1";
    private static final boolean[] slots = new boolean[8];
    private static final ConcurrentHashMap<Long, Connection> connections = new ConcurrentHashMap<>();
    private static final AtomicLong next = new AtomicLong();
    private static final ExecutorService calls = Executors.newCachedThreadPool();

    private static synchronized int acquire() {
        for (int i = 0; i < slots.length; i++) if (!slots[i]) { slots[i] = true; return i; }
        throw new IllegalStateException("Android supports at most 8 active Go extensions");
    }
    private static synchronized void free(int slot) { slots[slot] = false; }

    static long[] open(Context context, String path, String digest, int minSdk) throws Exception {
        path = ExtensionLibrary.verify(context, path, digest, minSdk);
        Connection connection = new Connection(context, acquire());
        long id = next.incrementAndGet();
        connections.put(id, connection);
        try { return connection.open(id, path, digest, minSdk); }
        catch (Exception error) { close(id); throw error; }
    }

    static void close(long id) {
        Connection connection = connections.remove(id);
        if (connection != null) connection.close();
    }

    private static final class Connection implements ServiceConnection {
        final Context context;
        final int slot;
        final IBinder owner = new Binder();
        final CompletableFuture<IBinder> ready = new CompletableFuture<>();
        final CompletableFuture<Void> dead = new CompletableFuture<>();
        volatile IBinder service;
        int pid;
        long remote;
        final AtomicBoolean closed = new AtomicBoolean();
        volatile boolean bound;
        boolean freed;
        Connection(Context context, int slot) { this.context = context; this.slot = slot; }

        long[] open(long id, String path, String digest, int minSdk) throws Exception {
            Intent intent = new Intent().setComponent(new ComponentName(context.getPackageName(),
                    "github.shadowbaby.clawproxyhub.extension.ExtensionService$Worker" + slot));
            bound = context.bindService(intent, this, Context.BIND_AUTO_CREATE);
            if (!bound) throw new IllegalStateException("Extension service bind failed");
            service = ready.get(10, TimeUnit.SECONDS);
            pid = call(() -> {
                Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                try {
                    request.writeInterfaceToken(PROTOCOL);
                    if (!service.transact(IBinder.FIRST_CALL_TRANSACTION + 2, request, reply, 0)) throw new IllegalStateException("Missing extension worker identity");
                    reply.readException();
                    return reply.readInt();
                } finally { request.recycle(); reply.recycle(); }
            }, 5000);
            if (pid <= 0 || pid == android.os.Process.myPid()) throw new SecurityException("Extension requires a private worker process");
            ParcelFileDescriptor[] sockets = ParcelFileDescriptor.createSocketPair();
            try (ParcelFileDescriptor local = sockets[0]; ParcelFileDescriptor remoteSocket = sockets[1]) {
                remote = call(() -> {
                    Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                    try {
                        request.writeInterfaceToken(PROTOCOL);
                        request.writeStrongBinder(owner);
                        remoteSocket.writeToParcel(request, 0);
                        request.writeString(path); request.writeString(digest); request.writeInt(minSdk);
                        if (!service.transact(IBinder.FIRST_CALL_TRANSACTION, request, reply, 0)) throw new IllegalStateException("Unsupported extension service protocol");
                        reply.readException();
                        return reply.readLong();
                    } finally { request.recycle(); reply.recycle(); }
                }, 10000);
                if (remote == 0) throw new IllegalStateException("Empty extension session");
                return new long[]{local.detachFd(), id};
            }
        }
        private <T> T call(Callable<T> action, long timeout) throws Exception {
            return calls.submit(action).get(timeout, TimeUnit.MILLISECONDS);
        }
        public void onServiceConnected(ComponentName name, IBinder value) {
            service = value;
            try {
                value.linkToDeath(() -> { dead.complete(null); if (!bound) releaseSlot(); }, 0);
                ready.complete(value);
            } catch (android.os.RemoteException error) {
                dead.complete(null);
                ready.completeExceptionally(error);
                if (!bound) releaseSlot();
            }
        }
        public void onServiceDisconnected(ComponentName name) { ready.completeExceptionally(new IllegalStateException("Extension service disconnected")); }
        public void onBindingDied(ComponentName name) { onServiceDisconnected(name); }
        public void onNullBinding(ComponentName name) { onServiceDisconnected(name); }
        synchronized void releaseSlot() { if (!freed) { freed = true; free(slot); } }

        void close() {
            if (!closed.compareAndSet(false, true)) return;
            if (service != null && remote != 0 && service.isBinderAlive()) {
                try {
                    call(() -> {
                        Parcel request = Parcel.obtain(), reply = Parcel.obtain();
                        try {
                            request.writeInterfaceToken(PROTOCOL); request.writeLong(remote);
                            service.transact(IBinder.FIRST_CALL_TRANSACTION + 1, request, reply, 0);
                            return null;
                        } finally { request.recycle(); reply.recycle(); }
                    }, 500);
                } catch (Exception ignored) { /* 进程退出同样撤销会话。 */ }
            }
            if (bound) { bound = false; context.unbindService(this); }
            if (service == null) { releaseSlot(); return; }
            // 原生初始化或退出可能卡住；已验证的私有 worker 可以由同 UID 宿主终止。
            if (pid > 0 && pid != android.os.Process.myPid() && service.isBinderAlive()) android.os.Process.killProcess(pid);
            try { dead.get(2, TimeUnit.SECONDS); } catch (Exception ignored) { /* 死亡回调稍后回收槽位。 */ }
            if (dead.isDone() || !service.isBinderAlive()) releaseSlot();
        }
    }
}
