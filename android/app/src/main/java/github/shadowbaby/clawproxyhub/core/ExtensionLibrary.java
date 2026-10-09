package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.os.Build;
import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Arrays;

// 核心已验签；私有服务在加载前再次核对安装路径、ABI 和核心传入的文件摘要。
public final class ExtensionLibrary {
    public static String verify(Context context, String path, String digest, int minSdk) throws Exception {
        return verify(new File(context.getFilesDir(), "extensions/versions"), path, digest, minSdk,
                Build.VERSION.SDK_INT, Build.SUPPORTED_64_BIT_ABIS).getAbsolutePath();
    }

    static File verify(File root, String path, String digest, int minSdk, int sdk, String[] abis) throws Exception {
        if (path == null || digest == null || !digest.matches("[0-9a-f]{64}") || minSdk < 24 || sdk < minSdk)
            throw new SecurityException("Incompatible Android extension requirements");
        File file = new File(path).getCanonicalFile();
        String base = root.getCanonicalPath() + File.separator;
        if (!file.getPath().startsWith(base) || !file.isFile() || !file.getName().endsWith(".so"))
            throw new SecurityException("Extension library is outside the installed versions");
        String[] parts = file.getPath().substring(base.length()).replace(File.separatorChar, '/').split("/");
        if (parts.length < 3 || parts[0].length() > 96 || !parts[0].matches("[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*") || !parts[1].matches("[0-9a-f]{64}"))
            throw new SecurityException("Invalid installed extension identity");
        MessageDigest hash = MessageDigest.getInstance("SHA-256");
        byte[] header = new byte[20], buffer = new byte[32768];
        long total = 0;
        try (InputStream input = new FileInputStream(file)) {
            int count;
            while ((count = input.read(buffer)) != -1) {
                if (total < header.length) System.arraycopy(buffer, 0, header, (int) total, Math.min(count, header.length - (int) total));
                total += count;
                if (total > 128L * 1024 * 1024) throw new SecurityException("Extension library is too large");
                hash.update(buffer, 0, count);
            }
        }
        int machine = (header[18] & 255) | (header[19] & 255) << 8;
        String abi = machine == 183 ? "arm64-v8a" : machine == 62 ? "x86_64" : "";
        if (total < header.length || header[0] != 127 || header[1] != 'E' || header[2] != 'L' || header[3] != 'F'
                || header[4] != 2 || header[5] != 1 || header[16] != 3 || header[17] != 0 || !Arrays.asList(abis).contains(abi))
            throw new SecurityException("Incompatible Android extension library");
        StringBuilder actual = new StringBuilder();
        for (byte value : hash.digest()) actual.append(String.format(java.util.Locale.ROOT, "%02x", value & 255));
        if (!digest.contentEquals(actual)) throw new SecurityException("Extension library changed after verification");
        return file;
    }
}
