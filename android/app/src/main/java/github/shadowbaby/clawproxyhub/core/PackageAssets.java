package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.util.AtomicFile;
import java.io.*;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

// APK 携带原始包作为安装来源，首次启动由核心统一导入。
final class PackageAssets {
    private static boolean prepared;
    static synchronized void prepare(Context context) throws Exception {
        if (prepared) return;
        File directory = new File(context.getFilesDir(), "packages/bundled");
        if (!directory.isDirectory() && !directory.mkdirs()) throw new IOException("Cannot create package source directory");
        String[] assets = context.getAssets().list("packages");
        Set<String> names = new HashSet<>();
        if (assets != null) for (String name : assets) {
            if (!name.matches("[A-Za-z0-9._-]+\\.(cphext|cphhost)")) continue;
            names.add(name);
            byte[] bytes;
            try (InputStream input = context.getAssets().open("packages/" + name)) { bytes = NativePluginStore.read(input, 128 * 1024 * 1024); }
            AtomicFile file = new AtomicFile(new File(directory, name));
            if (file.getBaseFile().isFile()) {
                try (InputStream input = file.openRead()) { if (Arrays.equals(bytes, NativePluginStore.read(input, 128 * 1024 * 1024))) continue; }
            }
            FileOutputStream output = null;
            try { output = file.startWrite(); output.write(bytes); file.finishWrite(output); }
            catch (Exception error) { file.failWrite(output); throw error; }
        }
        File[] existing = directory.listFiles();
        if (existing != null) for (File file : existing) {
            if (file.isFile() && !names.contains(file.getName()) && !file.delete()) throw new IOException("Cannot update bundled package source");
        }
        prepared = true;
    }
}
