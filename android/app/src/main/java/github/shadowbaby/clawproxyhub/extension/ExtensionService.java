package github.shadowbaby.clawproxyhub.extension;

import android.app.Service;
import android.content.Intent;
import android.os.Binder;
import android.os.IBinder;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
import android.os.RemoteException;
import github.shadowbaby.clawproxyhub.core.ExtensionLibrary;

// 一个 worker 只加载一个 Go 扩展，卸载绑定时终止进程，避免旧原生库继续存活。
public class ExtensionService extends Service {
    public static final class Worker0 extends ExtensionService {}
    public static final class Worker1 extends ExtensionService {}
    public static final class Worker2 extends ExtensionService {}
    public static final class Worker3 extends ExtensionService {}
    public static final class Worker4 extends ExtensionService {}
    public static final class Worker5 extends ExtensionService {}
    public static final class Worker6 extends ExtensionService {}
    public static final class Worker7 extends ExtensionService {}
    private static final String PROTOCOL = "github.shadowbaby.clawproxyhub.extension.v1";
    private static native long nativeOpen(int socket);
    private static native void nativeClose(long session);
    private String loaded;
    private long session;
    private IBinder owner;
    private IBinder.DeathRecipient death;

    private final Binder binder = new Binder() {
        @Override protected boolean onTransact(int code, Parcel data, Parcel reply, int flags) throws RemoteException {
            if (code == INTERFACE_TRANSACTION) { reply.writeString(PROTOCOL); return true; }
            if (Binder.getCallingUid() != getApplicationInfo().uid) throw new SecurityException("Untrusted extension host");
            data.enforceInterface(PROTOCOL);
            if (code == FIRST_CALL_TRANSACTION + 2) {
                reply.writeNoException();
                reply.writeInt(android.os.Process.myPid());
                return true;
            }
            if (code == FIRST_CALL_TRANSACTION) {
                synchronized (ExtensionService.this) {
                    if (loaded != null) throw new IllegalStateException("Extension worker is already in use");
                    IBinder nextOwner = data.readStrongBinder();
                    if (nextOwner == null) throw new SecurityException("Missing extension session owner");
                    try (ParcelFileDescriptor socket = ParcelFileDescriptor.CREATOR.createFromParcel(data)) {
                        String library = ExtensionLibrary.verify(ExtensionService.this, data.readString(), data.readString(), data.readInt());
                        System.load(library);
                        loaded = library;
                        session = nativeOpen(socket.detachFd());
                        if (session == 0) throw new IllegalStateException("Extension native session failed");
                        owner = nextOwner;
                        death = () -> { closeSession(); android.os.Process.killProcess(android.os.Process.myPid()); };
                        try { owner.linkToDeath(death, 0); }
                        catch (RemoteException error) { closeSession(); throw error; }
                        reply.writeNoException();
                        reply.writeLong(session);
                    } catch (Exception | UnsatisfiedLinkError error) {
                        closeSession();
                        throw new IllegalStateException("Cannot start Android extension: " + error.getMessage(), error);
                    }
                }
                return true;
            }
            if (code == FIRST_CALL_TRANSACTION + 1) {
                long id = data.readLong();
                synchronized (ExtensionService.this) { if (id == session) closeSession(); }
                reply.writeNoException();
                return true;
            }
            return super.onTransact(code, data, reply, flags);
        }
    };

    private synchronized void closeSession() {
        if (session != 0) { nativeClose(session); session = 0; }
        if (owner != null && death != null) {
            try { owner.unlinkToDeath(death, 0); }
            catch (java.util.NoSuchElementException ignored) { /* 宿主可能已断连。 */ }
        }
        owner = null;
        death = null;
    }

    @Override public IBinder onBind(Intent intent) { return binder; }
    @Override public void onDestroy() {
        closeSession();
        super.onDestroy();
        android.os.Process.killProcess(android.os.Process.myPid());
    }
}
