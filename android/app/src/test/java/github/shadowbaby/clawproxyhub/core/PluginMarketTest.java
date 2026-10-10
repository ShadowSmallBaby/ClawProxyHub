package github.shadowbaby.clawproxyhub.core;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public final class PluginMarketTest {
    private JSONObject entry(String runtime, String... abis) throws Exception {
        return new JSONObject().put("name", "test").put("version", "1.0.0").put("runtime", runtime)
                .put("platforms", new JSONObject().put("android", new JSONArray(abis)))
                .put("release_manifest", new JSONObject().put("download_url", "https://example.com/manifest.json").put("sha256", "a".repeat(64)));
    }
    private JSONObject artifact(String format, String filename) throws Exception {
        return new JSONObject().put("format", format).put("download_url", "https://example.com/" + filename)
                .put("sha256", "b".repeat(64)).put("size", 100).put("min_sdk", 24);
    }
    private byte[] manifest(JSONObject entry, JSONObject artifacts) throws Exception {
        byte[] raw = new JSONObject().put("schema_version", 1).put("protocol_version", 2)
                .put("name", entry.getString("name")).put("version", entry.getString("version")).put("runtime", entry.getString("runtime"))
                .put("artifacts", artifacts).toString().getBytes(StandardCharsets.UTF_8);
        entry.getJSONObject("release_manifest").put("sha256", NativePluginStore.hash(raw));
        return raw;
    }
    @Test public void nativeCatalogRequiresAnExplicitPlatformAndMatchingAbi() throws Exception {
        String[] device = {"arm64-v8a"};
        JSONObject entry = entry("go", "armeabi-v7a", "arm64-v8a");
        assertEquals("", PluginMarket.unavailableReason(entry, device));
        assertEquals("abi", PluginMarket.unavailableReason(entry, new String[]{"x86_64"}));
        entry.getJSONObject("platforms").put("android", new JSONArray());
        assertEquals("unpublished", PluginMarket.unavailableReason(entry, device));
        entry.getJSONObject("platforms").remove("android");
        entry.getJSONObject("platforms").put("ios", new JSONArray().put("arm64"));
        assertEquals("unpublished", PluginMarket.unavailableReason(entry, device));
        entry.remove("platforms");
        entry.put("android", new JSONObject().put("arm64-v8a", artifact("cph-native-v1", "old.cphplugin")));
        assertEquals("unpublished", PluginMarket.unavailableReason(entry, device));
    }
    @Test public void catalogRejectsMalformedPlatformOrManifestReference() throws Exception {
        JSONObject entry = entry("go", "arm64-v8a");
        entry.getJSONObject("platforms").put("android", true);
        assertEquals("metadata", PluginMarket.unavailableReason(entry, new String[]{"arm64-v8a"}));
        entry.getJSONObject("platforms").put("android", new JSONArray().put("arm64-v8a").put(7));
        assertEquals("metadata", PluginMarket.unavailableReason(entry, new String[]{"arm64-v8a"}));
        entry = entry("go", "arm64-v8a");
        entry.remove("release_manifest");
        assertEquals("metadata", PluginMarket.unavailableReason(entry, new String[]{"arm64-v8a"}));
    }
    @Test public void downloadsComeFromVerifiedPlatformManifests() throws Exception {
        JSONObject entry = entry("go", "arm64-v8a", "x86_64");
        byte[] raw = manifest(entry, new JSONObject()
                .put("android-arm64", artifact("cph-native-v1", "arm.cphplugin"))
                .put("android-amd64", artifact("cph-native-v1", "x86.cphplugin")));
        assertTrue(PluginMarket.releaseArtifact(entry, raw, new String[]{"arm64-v8a"}, 35).getString("download_url").endsWith("arm.cphplugin"));
        assertTrue(PluginMarket.releaseArtifact(entry, raw, new String[]{"x86_64"}, 35).getString("download_url").endsWith("x86.cphplugin"));
    }
    @Test public void luaUsesOneScriptPackageIndependentOfTheHostAbi() throws Exception {
        JSONObject script = new JSONObject().put("name", "autoclaw").put("version", "0.1.2").put("runtime", "lua")
                .put("download_url", "https://example.com/autoclaw-0.1.2.cphplugin").put("sha256", "a".repeat(64));
        for (String[] abis : new String[][]{{"arm64-v8a"}, {"x86_64"}, {}}) {
            assertEquals("", PluginMarket.unavailableReason(script, abis));
        }
        JSONObject artifact = PluginMarket.scriptArtifact(script);
        assertEquals(script.getString("download_url"), artifact.getString("download_url"));
        assertEquals(script.getString("sha256"), artifact.getString("sha256"));
        assertFalse(artifact.has("size"));
        script.put("size", 128);
        assertEquals(128, PluginMarket.scriptArtifact(script).getLong("size"));
        script.put("runtime", "go");
        assertThrows(IOException.class, () -> PluginMarket.scriptArtifact(script));
    }
    @Test public void scriptPackagesStillRequireValidDownloadsAndMatchingIdentity() throws Exception {
        JSONObject script = new JSONObject().put("name", "autoclaw").put("version", "0.1.2").put("runtime", "lua")
                .put("download_url", "https://example.com/script.cphplugin").put("sha256", "a".repeat(64));
        for (JSONObject invalid : new JSONObject[]{new JSONObject(script.toString()).put("sha256", ""),
                new JSONObject(script.toString()).put("download_url", "http://example.com/script.cphplugin"),
                new JSONObject(script.toString()).put("download_url", "https://"),
                new JSONObject(script.toString()).put("size", 0)}) {
            assertEquals("metadata", PluginMarket.unavailableReason(invalid, new String[]{"arm64-v8a"}));
            assertThrows(IOException.class, () -> PluginMarket.scriptArtifact(invalid));
        }
        byte[] data = "verified-script-package".getBytes(StandardCharsets.UTF_8);
        script.put("sha256", NativePluginStore.hash(data));
        PluginMarket.verifyDownload(PluginMarket.scriptArtifact(script), data);
        assertThrows(SecurityException.class, () -> PluginMarket.verifyDownload(PluginMarket.scriptArtifact(script), new byte[]{1}));
        script.put("size", data.length + 1);
        assertThrows(SecurityException.class, () -> PluginMarket.verifyDownload(PluginMarket.scriptArtifact(script), data));
        JSONObject metadata = new JSONObject().put("name", "autoclaw").put("version", "0.1.2").put("runtime", "lua").put("protocol_version", 2);
        PluginMarket.verifyPackage(script, metadata);
        for (JSONObject wrong : new JSONObject[]{new JSONObject(metadata.toString()).put("name", "another"),
                new JSONObject(metadata.toString()).put("version", "1.0.0"), new JSONObject(metadata.toString()).put("runtime", "go"),
                new JSONObject(metadata.toString()).put("protocol_version", 99)}) {
            assertThrows(SecurityException.class, () -> PluginMarket.verifyPackage(script, wrong));
        }
    }
    @Test public void platformHintsDoNotOverrideManifestCompatibility() throws Exception {
        JSONObject entry = entry("go", "arm64-v8a");
        byte[] wrongAbi = manifest(entry, new JSONObject().put("android-amd64", artifact("cph-native-v1", "x86.cphplugin")));
        assertThrows(IOException.class, () -> PluginMarket.releaseArtifact(entry, wrongAbi, new String[]{"arm64-v8a"}, 35));
        for (JSONObject artifact : new JSONObject[]{artifact("cph-native-v1", "arm.cphplugin").put("min_sdk", 36),
                artifact("cph-go-v1", "desktop.cphplugin"), artifact("cph-native-v1", "arm.cphplugin").put("size", 0)}) {
            byte[] raw = manifest(entry, new JSONObject().put("android-arm64", artifact));
            assertThrows(IOException.class, () -> PluginMarket.releaseArtifact(entry, raw, new String[]{"arm64-v8a"}, 35));
        }
        JSONObject unknown = entry("go", "unknown-abi");
        byte[] raw = manifest(unknown, new JSONObject().put("android-amd64", artifact("cph-native-v1", "x86.cphplugin")));
        assertThrows(IOException.class, () -> PluginMarket.releaseArtifact(unknown, raw, new String[]{"unknown-abi"}, 35));
    }
    @Test public void rejectsManifestTamperingAndIdentityMismatch() throws Exception {
        JSONObject entry = entry("go", "arm64-v8a");
        byte[] raw = manifest(entry, new JSONObject().put("android-arm64", artifact("cph-native-v1", "arm.cphplugin")));
        entry.put("version", "2.0.0");
        assertThrows(SecurityException.class, () -> PluginMarket.releaseArtifact(entry, raw, new String[]{"arm64-v8a"}, 35));
        entry.put("version", "1.0.0");
        entry.getJSONObject("release_manifest").put("sha256", "0".repeat(64));
        assertThrows(SecurityException.class, () -> PluginMarket.releaseArtifact(entry, raw, new String[]{"arm64-v8a"}, 35));
    }
}
