package github.shadowbaby.clawproxyhub.plugin;

import android.app.Service;
import android.content.Intent;
import github.shadowbaby.clawproxyhub.core.NativePluginStore;
import android.os.Binder;
import android.os.IBinder;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
import android.os.RemoteException;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;
import java.util.NoSuchElementException;

// 每个私有进程只加载一个已校验的 Go 插件，结束时销毁进程释放整个运行时。
public class PluginService extends Service {
    private static final String PROTOCOL = "github.shadowbaby.clawproxyhub.plugin.v1";
    private String loadedPlugin;
    private static native long nativeOpen(int requests, int callbacks);
    private static native void nativeClose(long session);
    private static native boolean nativeInitialize(String directory);
    private final Map<Long, Runnable> cleanup = new HashMap<>();

    private final Binder binder = new Binder() {
        @Override protected boolean onTransact(int code, Parcel data, Parcel reply, int flags) throws RemoteException {
            if (code == INTERFACE_TRANSACTION) { reply.writeString(PROTOCOL); return true; }
            if (Binder.getCallingUid() != android.os.Process.myUid()) throw new SecurityException("Private application service");
            data.enforceInterface(PROTOCOL);
            if (code == FIRST_CALL_TRANSACTION + 2) {
                reply.writeNoException();
                reply.writeInt(android.os.Process.myPid());
                return true;
            }
            if (code == FIRST_CALL_TRANSACTION) {
                IBinder owner = data.readStrongBinder();
                if (owner == null) throw new IllegalArgumentException("missing session owner");
                try (ParcelFileDescriptor requests = ParcelFileDescriptor.CREATOR.createFromParcel(data);
                     ParcelFileDescriptor callbacks = ParcelFileDescriptor.CREATOR.createFromParcel(data)) {
                    String plugin = data.readString();
                    synchronized (cleanup) {
                        if (loadedPlugin == null) {
                            String library = new NativePluginStore(PluginService.this).library(plugin);
                            System.load(library);
                            java.io.File temporary = new java.io.File(getCacheDir(), "native-plugins/" + plugin);
                            if (!nativeInitialize(temporary.getAbsolutePath())) throw new IllegalStateException("Cannot initialize plugin");
                            loadedPlugin = plugin;
                        } else if (!loadedPlugin.equals(plugin)) throw new IllegalStateException("Worker already owns another runtime");

                        if (cleanup.size() >= 8) throw new IllegalStateException("too many sessions");
                        long session = nativeOpen(requests.detachFd(), callbacks.detachFd());
                        if (session == 0) throw new IllegalStateException("native session failed");
                        IBinder.DeathRecipient death = () -> closeSession(session);
                        cleanup.put(session, () -> {
                            try { owner.unlinkToDeath(death, 0); }
                            catch (NoSuchElementException ignored) { /* 连接可能在注册死亡回调前已断开。 */ }
                            finally { nativeClose(session); }
                        });
                        try { owner.linkToDeath(death, 0); }
                        catch (RemoteException e) { closeSession(session); throw e; }
                        reply.writeNoException();
                        reply.writeLong(session);
                    }
                } catch (Exception e) { throw new IllegalStateException(e); }
                return true;
            }
            if (code == FIRST_CALL_TRANSACTION + 1) {
                closeSession(data.readLong());
                reply.writeNoException();
                return true;
            }
            return super.onTransact(code, data, reply, flags);
        }
    };

    private void closeSession(long session) {
        Runnable close;
        synchronized (cleanup) { close = cleanup.remove(session); }
        if (close != null) close.run();
    }

    @Override public IBinder onBind(Intent intent) { return binder; }
    @Override public void onDestroy() {
        Long[] ids;
        synchronized (cleanup) { ids = cleanup.keySet().toArray(new Long[0]); }
        for (long id : ids) closeSession(id);
        super.onDestroy();
        android.os.Process.killProcess(android.os.Process.myPid());
    }
    public static final class Worker0 extends PluginService {}
    public static final class Worker1 extends PluginService {}
    public static final class Worker2 extends PluginService {}
    public static final class Worker3 extends PluginService {}
    public static final class Worker4 extends PluginService {}
    public static final class Worker5 extends PluginService {}
    public static final class Worker6 extends PluginService {}
    public static final class Worker7 extends PluginService {}
    public static final class Worker8 extends PluginService {}
    public static final class Worker9 extends PluginService {}
    public static final class Worker10 extends PluginService {}
    public static final class Worker11 extends PluginService {}
    public static final class Worker12 extends PluginService {}
    public static final class Worker13 extends PluginService {}
    public static final class Worker14 extends PluginService {}
    public static final class Worker15 extends PluginService {}
    public static final class Worker16 extends PluginService {}
    public static final class Worker17 extends PluginService {}
    public static final class Worker18 extends PluginService {}
    public static final class Worker19 extends PluginService {}
    public static final class Worker20 extends PluginService {}
    public static final class Worker21 extends PluginService {}
    public static final class Worker22 extends PluginService {}
    public static final class Worker23 extends PluginService {}
    public static final class Worker24 extends PluginService {}
    public static final class Worker25 extends PluginService {}
    public static final class Worker26 extends PluginService {}
    public static final class Worker27 extends PluginService {}
    public static final class Worker28 extends PluginService {}
    public static final class Worker29 extends PluginService {}
    public static final class Worker30 extends PluginService {}
    public static final class Worker31 extends PluginService {}
}
