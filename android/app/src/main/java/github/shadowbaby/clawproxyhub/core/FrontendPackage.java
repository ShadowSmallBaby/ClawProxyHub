package github.shadowbaby.clawproxyhub.core;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.util.Iterator;
import java.util.Locale;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

// 完整 App 界面使用宿主 RSA 证书，签名覆盖清单及全部资源摘要。
final class FrontendPackage {
    static String hash(byte[] data) throws Exception {
        StringBuilder result = new StringBuilder();
        for (byte value : MessageDigest.getInstance("SHA-256").digest(data)) result.append(String.format(Locale.ROOT, "%02x", value & 255));
        return result.toString();
    }

    static boolean safe(String path) {
        if (path.isEmpty() || path.startsWith("/") || path.contains("\\") || path.contains(":")) return false;
        for (String part : path.split("/", -1)) if (part.isEmpty() || part.equals(".") || part.equals("..") || part.endsWith(".") || part.endsWith(" ")) return false;
        return true;
    }

    private static long[] version(String value) {
        if (!value.matches("(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)")) throw new SecurityException("Invalid frontend version");
        String[] parts = value.split("\\.");
        return new long[]{Long.parseLong(parts[0]), Long.parseLong(parts[1]), Long.parseLong(parts[2])};
    }

    static int compare(String left, String right) {
        long[] a = version(left), b = version(right);
        for (int i = 0; i < 3; i++) { int result = Long.compare(a[i], b[i]); if (result != 0) return result; }
        return 0;
    }

    static JSONObject verify(Map<String, byte[]> files, PublicKey publicKey, byte[] signature, String coreVersion, String platform, String embeddedVersion) throws Exception {
        byte[] raw = files.get("manifest.json");
        if (raw == null || raw.length > 1024 * 1024) throw new SecurityException("Invalid frontend manifest");
        PackageSignature.verify(raw, publicKey, signature);
        JSONObject manifest = new JSONObject(new String(raw, StandardCharsets.UTF_8));
        if (!"frontend.app".equals(manifest.getString("id")) || !"frontend-trusted".equals(manifest.getString("kind")) ||
                !"client".equals(manifest.getString("target")) || manifest.getInt("api") != 1 || !"restart".equals(manifest.getString("activation")))
            throw new SecurityException("Incompatible frontend target/API");
        if (compare(manifest.getString("version"), embeddedVersion) < 0) throw new SecurityException("Frontend is older than the embedded interface");
        JSONObject core = manifest.getJSONObject("core");
        if (core.has("min") && compare(coreVersion, core.getString("min")) < 0 ||
                core.has("max_exclusive") && compare(coreVersion, core.getString("max_exclusive")) >= 0)
            throw new SecurityException("Incompatible frontend contract");
        JSONArray platforms = manifest.optJSONArray("platforms");
        if (platforms != null && platforms.length() > 0) {
            boolean supported = false;
            for (int i = 0; i < platforms.length(); i++) supported |= platform.equals(platforms.getString(i));
            if (!supported) throw new SecurityException("Frontend does not support this platform");
        }
        if (manifest.optJSONObject("dependencies") != null && manifest.getJSONObject("dependencies").length() != 0)
            throw new SecurityException("Frontend dependencies are unsupported");
        for (String field : new String[]{"actions", "pages", "contributions"})
            if (manifest.optJSONArray(field) != null && manifest.getJSONArray(field).length() != 0) throw new SecurityException("Unsupported frontend contribution");
        JSONArray permissions = manifest.getJSONArray("permissions");
        if (permissions.length() != 1 || !"client.ui".equals(permissions.getString(0))) throw new SecurityException("Unsupported frontend permissions");
        JSONObject hashes = manifest.getJSONObject("files");
        if (files.size() != hashes.length() + 2 || !files.containsKey("signature.json") || !hashes.has("frontend/index.html"))
            throw new SecurityException("Unsigned frontend resources");
        Iterator<String> names = hashes.keys();
        while (names.hasNext()) {
            String name = names.next();
            if (!safe(name) || !name.startsWith("frontend/") || !files.containsKey(name) || !hash(files.get(name)).equals(hashes.getString(name)))
                throw new SecurityException("Frontend resource hash mismatch");
        }
        return manifest;
    }
}
