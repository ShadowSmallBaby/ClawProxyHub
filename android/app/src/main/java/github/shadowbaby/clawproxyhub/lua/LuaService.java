package github.shadowbaby.clawproxyhub.lua;

import android.app.Service;
import android.content.Intent;
import android.os.Binder;
import android.os.IBinder;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
import android.os.RemoteException;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;
import java.util.NoSuchElementException;

// 私有服务加载已验签的运行时；候选包在独立探测进程中执行，避免影响现用版本。
public class LuaService extends Service {
    public static final class Probe extends LuaService {}
    private static final String PROTOCOL = "github.shadowbaby.clawproxyhub.lua.v1";
    private static String loaded;
    private void load(String archive) {
        try {
            github.shadowbaby.clawproxyhub.core.HostStore store = new github.shadowbaby.clawproxyhub.core.HostStore(this);
            String library = archive == null ? store.library() : store.library(new java.io.File(archive));
            if (library == null) throw new IllegalStateException("Lua Host package is not installed or enabled");
            if (loaded != null) {
                if (!loaded.equals(library)) throw new IllegalStateException("Lua Host worker still uses another version");
                return;
            }
            System.load(library);
            loaded = library;
        } catch (Exception | UnsatisfiedLinkError error) {
            throw new IllegalStateException("Cannot load the installed Lua Host package", error);
        }
    }
    private static native long nativeOpen(int requests, int callbacks, int bundle, String directory);
    private static native void nativeClose(long session);
    private final Map<Long, Runnable> cleanup = new HashMap<>();

    private final Binder binder = new Binder() {
        @Override protected boolean onTransact(int code, Parcel data, Parcel reply, int flags) throws RemoteException {
            if (code == INTERFACE_TRANSACTION) { reply.writeString(PROTOCOL); return true; }
            if (Binder.getCallingUid() != getApplicationInfo().uid) {
                throw new SecurityException("untrusted plugin host");
            }
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
                     ParcelFileDescriptor callbacks = ParcelFileDescriptor.CREATOR.createFromParcel(data);
                     ParcelFileDescriptor bundle = ParcelFileDescriptor.CREATOR.createFromParcel(data)) {
                    synchronized (cleanup) {
                        if (cleanup.size() >= 8) throw new IllegalStateException("too many sessions");
                        String archive = LuaService.this instanceof Probe ? data.readString() : null;
                        if (LuaService.this instanceof Probe && archive == null) throw new SecurityException("missing runtime candidate");
                        load(archive);
                        long session = nativeOpen(requests.detachFd(), callbacks.detachFd(),bundle.detachFd(),getCacheDir().getAbsolutePath());
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
                } catch (IOException e) { throw new IllegalStateException(e); }
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
}
