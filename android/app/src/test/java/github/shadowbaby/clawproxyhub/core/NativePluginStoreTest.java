package github.shadowbaby.clawproxyhub.core;

import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.assertThrows;

public class NativePluginStoreTest {
    private JSONObject host() throws Exception {
        return new JSONObject().put("id", "lua-runtime").put("name", "Lua Host").put("version", "0.3.0")
                .put("api", 1).put("kind", "runtime").put("target", "backend").put("execution", "android-service");
    }

    @Test public void runtimeDisplayNamesDoNotChangePackageIdentity() throws Exception {
        NativePluginStore.validateIdentity(host(), true);
        NativePluginStore.validateIdentity(host().put("name", "Lua 运行时"), true);
    }

    @Test public void runtimeDisplayNameCannotImpersonateTheTrustedPackage() throws Exception {
        JSONObject wrongID = host().put("id", "another-runtime").put("name", "luahost");
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(wrongID, true));
        JSONObject missingID = host(); missingID.remove("id");
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(missingID, true));
    }

    @Test public void runtimeRequiresItsNativeContract() throws Exception {
        for (String field : new String[]{"kind", "target", "execution"}) {
            JSONObject changed = host().put(field, "other");
            assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(changed, true));
        }
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(host().put("runtime", "host"), true));
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(host().put("protocol_version", 2), true));
    }

    @Test public void businessPluginsStillRequireMachineReadableNames() throws Exception {
        JSONObject plugin = new JSONObject().put("name", "test_plugin").put("version", "1.0.0").put("protocol_version", 2);
        NativePluginStore.validateIdentity(plugin, false);
        plugin.put("name", "Lua Host");
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(plugin, false));
    }
    @Test public void businessPluginProtocolCannotUseTheRuntimeEnvelope() throws Exception {
        JSONObject plugin = new JSONObject().put("name", "test_plugin").put("version", "1.0.0").put("protocol_version", 1);
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(plugin, false));
        plugin.put("protocol_version", 2).put("runtime", "host");
        assertThrows(SecurityException.class, () -> NativePluginStore.validateIdentity(plugin, false));
    }
}
