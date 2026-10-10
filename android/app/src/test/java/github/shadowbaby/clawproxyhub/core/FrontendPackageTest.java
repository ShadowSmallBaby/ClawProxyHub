package github.shadowbaby.clawproxyhub.core;

import static org.junit.Assert.*;
import java.nio.charset.StandardCharsets;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.PublicKey;
import java.security.Signature;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Base64;
import java.util.zip.ZipInputStream;
import java.util.zip.ZipEntry;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;

public final class FrontendPackageTest {
    private final KeyPair identity;
    private final PublicKey publicKey;
    private final Map<String, byte[]> files = new LinkedHashMap<>();
    private final JSONObject manifest;

    public FrontendPackageTest() throws Exception {
        identity = KeyPairGenerator.getInstance("RSA").generateKeyPair();
        publicKey = identity.getPublic();
        byte[] html = "<html>App</html>".getBytes(StandardCharsets.UTF_8);
        files.put("frontend/index.html", html);
        files.put("signature.json", new byte[0]);
        manifest = new JSONObject().put("id", "frontend.app").put("version", "0.6.0").put("name", "App")
                .put("kind", "frontend-trusted").put("target", "client").put("api", 1).put("activation", "restart")
                .put("core", new JSONObject().put("min", "1.5.2"))
                .put("permissions", new JSONArray().put("client.ui"))
                .put("platforms", new JSONArray().put("android/arm64"))
                .put("files", new JSONObject().put("frontend/index.html", FrontendPackage.hash(html)));
    }

    private byte[] sign() throws Exception {
        byte[] raw = manifest.toString().getBytes(StandardCharsets.UTF_8);
        files.put("manifest.json", raw);
        Signature signer = Signature.getInstance("SHA256withRSA");
        signer.initSign(identity.getPrivate()); signer.update(raw); return signer.sign();
    }

    @Test public void validatesSignedAppAndRejectsTamperedBytes() throws Exception {
        byte[] signed = sign();
        assertEquals("0.6.0", FrontendPackage.verify(files, publicKey, signed, "1.5.2", "android/arm64", "0.6.0").getString("version"));
        files.put("frontend/index.html", "changed".getBytes(StandardCharsets.UTF_8));
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, signed, "1.5.2", "android/arm64", "0.6.0"));
    }

    @Test public void rejectsForeignSignerPlatformAndContract() throws Exception {
        byte[] signed = sign();
        PublicKey wrongKey = KeyPairGenerator.getInstance("RSA").generateKeyPair().getPublic();
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, wrongKey, signed, "1.5.2", "android/arm64", "0.6.0"));
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, signed, "1.5.1", "android/arm64", "0.6.0"));
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, signed, "1.5.2", "android/amd64", "0.6.0"));
    }

    @Test public void rejectsInstalledInterfaceOlderThanUpdatedApp() throws Exception {
        byte[] signed = sign();
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, signed, "1.5.2", "android/arm64", "0.7.0"));
    }

    @Test public void rejectsUnsignedResourcesAndExtraPermissions() throws Exception {
        byte[] signed = sign();
        files.put("unsigned.js", new byte[0]);
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, signed, "1.5.2", "android/arm64", "0.6.0"));
        files.remove("unsigned.js");
        manifest.getJSONArray("permissions").put("runtime.execute");
        byte[] expanded = sign();
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(files, publicKey, expanded, "1.5.2", "android/arm64", "0.6.0"));
    }

    @Test public void verifiesGradlePackageAndRequiresPinnedAppCertificate() throws Exception {
        Map<String, byte[]> archive = new LinkedHashMap<>();
        try (ZipInputStream zip = new ZipInputStream(getClass().getResourceAsStream("/good.cphui"))) {
            ZipEntry entry;
            while ((entry = zip.getNextEntry()) != null) archive.put(entry.getName(), zip.readAllBytes());
        }
        JSONObject signed = new JSONObject(new String(archive.get("signature.json"), StandardCharsets.UTF_8));
        assertEquals("SHA256withRSA", signed.getString("algorithm"));
        byte[] certificate = Base64.getDecoder().decode(signed.getString("certificate"));
        PublicKey key = PackageSignature.trustedKey(certificate, new byte[][]{certificate});
        assertEquals("1.0.0", FrontendPackage.verify(archive, key, Base64.getDecoder().decode(signed.getString("value")),
                "1.5.2", "android/arm64", "0.6.0").getString("version"));
        assertThrows(SecurityException.class, () -> PackageSignature.trustedKey(certificate, new byte[][]{publicKey.getEncoded()}));
        archive.get("manifest.json")[0] ^= 1;
        assertThrows(SecurityException.class, () -> FrontendPackage.verify(archive, key, Base64.getDecoder().decode(signed.getString("value")),
                "1.5.2", "android/arm64", "0.6.0"));
    }
}
