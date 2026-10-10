package github.shadowbaby.clawproxyhub.core;

import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public final class AppUpdatesTest {
    private static final String REPOSITORY = "https://github.com/example/fork";
    private JSONObject manifest() throws Exception {
        String filename = "ClawProxyHub-1.0.0-4-android-arm64-v8a.apk";
        JSONObject artifact = new JSONObject().put("name", filename).put("download_url", REPOSITORY + "/releases/download/v1.6.0/" + filename)
                .put("sha256", "a".repeat(64)).put("size", 1024);
        JSONObject android = new JSONObject().put("target", "android-app").put("application_id", BuildConfig.APPLICATION_ID)
                .put("version", "1.0.0").put("version_code", 4).put("min_sdk", 24).put("artifacts", new JSONObject().put("arm64-v8a", artifact))
                .put("changelog", new JSONObject().put("items", new JSONArray().put(new JSONObject().put("en", "Android change"))));
        return new JSONObject().put("schema_version", 1).put("version", "999.0.0").put("tag", "v1.6.0")
                .put("release_url", REPOSITORY + "/releases/tag/v1.6.0").put("platform", new JSONObject().put("android", android));
    }
    @Test public void selectsApplicationCodeAndChangelogIndependently() throws Exception {
        JSONObject result = AppUpdates.select(manifest(), 3, new String[]{"arm64-v8a"}, 35, REPOSITORY);
        assertTrue(result.getBoolean("newer"));
        assertEquals("1.0.0", result.getString("version"));
        assertTrue(result.getString("url").startsWith(REPOSITORY + "/releases/"));
        assertEquals("Android change", result.getJSONObject("changelog").getJSONArray("items").getJSONObject(0).getString("en"));
        assertFalse(AppUpdates.select(manifest(), 4, new String[]{"arm64-v8a"}, 35, REPOSITORY).getBoolean("newer"));
        assertFalse(AppUpdates.select(manifest(), 5, new String[]{"arm64-v8a"}, 35, REPOSITORY).getBoolean("newer"));
    }
    @Test public void unsupportedDevicesDoNotReceiveAnApk() throws Exception {
        for (JSONObject result : new JSONObject[]{
                AppUpdates.select(manifest(), 3, new String[]{"x86_64"}, 35, REPOSITORY),
                AppUpdates.select(manifest(), 3, new String[]{"arm64-v8a"}, 23, REPOSITORY)}) {
            assertTrue(result.getBoolean("newer"));
            assertEquals("", result.getString("url"));
        }
    }
    @Test public void rejectsMismatchedIdentityAndDownloads() throws Exception {
        for (String field : new String[]{"repository", "application_id", "schema_version", "download_url", "name", "sha256", "size"}) {
            JSONObject value = manifest();
            JSONObject android = value.getJSONObject("platform").getJSONObject("android");
            JSONObject artifact = android.getJSONObject("artifacts").getJSONObject("arm64-v8a");
            if (field.equals("application_id")) android.put(field, "another.application");
            else if (field.equals("schema_version")) value.put(field, 2);
            else if (!field.equals("repository")) artifact.put(field, field.equals("size") ? 0 : "invalid");
            try {
                AppUpdates.select(value, 3, new String[]{"arm64-v8a"}, 35, field.equals("repository") ? "https://github.com/another/app" : REPOSITORY);
                fail("Accepted invalid " + field);
            } catch (java.io.IOException expected) { }
        }
    }
}
