package github.shadowbaby.clawproxyhub.core;

import static org.junit.Assert.*;
import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;

public final class RuntimePackagesTest {
    private static final String REPOSITORY = "https://github.com/example/fork";

    private JSONObject index() throws Exception {
        String filename = "luahost-0.2.0-android-arm64-v8a.cphhost";
        return new JSONObject().put("schema_version", 1).put("tag", "v1.5.2").put("release_url", REPOSITORY + "/releases/tag/v1.5.2")
                .put("packages", new JSONArray().put(new JSONObject().put("id", "lua-runtime").put("kind", "runtime").put("target", "backend")
                        .put("version", "0.2.0").put("core", new JSONObject().put("min", "1.5.2")).put("platforms", new JSONArray().put("android/arm64"))
                        .put("name", filename).put("sha256", "a".repeat(64)).put("size", 1234)
                        .put("download_url", REPOSITORY + "/releases/download/v1.5.2/" + filename)));
    }

    @Test public void selectsRuntimeIndependentlyOfAppAndProjectVersions() throws Exception {
        assertEquals("0.2.0", RuntimePackages.select(index(), new String[]{"arm64-v8a"}, "1.5.2", REPOSITORY).getString("version"));
        assertNull(RuntimePackages.select(index(), new String[]{"x86_64"}, "1.5.2", REPOSITORY));
        assertNull(RuntimePackages.select(index(), new String[]{"arm64-v8a"}, "1.5.1", REPOSITORY));
    }

    @Test public void rejectsUnexpectedReleaseUrlsAndPackageTypes() throws Exception {
        JSONObject index = index();
        index.getJSONArray("packages").getJSONObject(0).put("download_url", "https://example.org/host.cphhost");
        assertThrows(SecurityException.class, () -> RuntimePackages.select(index, new String[]{"arm64-v8a"}, "1.5.2", REPOSITORY));
        index.getJSONArray("packages").getJSONObject(0).put("kind", "frontend-trusted");
        assertNull(RuntimePackages.select(index, new String[]{"arm64-v8a"}, "1.5.2", REPOSITORY));
    }
}
