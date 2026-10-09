package github.shadowbaby.clawproxyhub.core;

import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public final class PluginStateTest {
    private JSONObject runtime() throws Exception {
        return new JSONObject().put("hash", "a".repeat(64)).put("enabled", true).put("available", true);
    }
    @Test public void luaAvailabilityFollowsInstalledRuntimeState() throws Exception {
        assertEquals("runtime_missing", PluginState.luaRuntime(null, false));
        assertEquals("runtime_missing", PluginState.luaRuntime(new JSONObject().put("enabled", false), false));
        JSONObject host = runtime();
        assertEquals("", PluginState.luaRuntime(host, false));
        assertEquals("runtime_stopped", PluginState.luaRuntime(host, true));
        host.put("enabled", false).put("available", false);
        assertEquals("runtime_stopped", PluginState.luaRuntime(host, false));
        host.put("enabled", true).put("error", "worker failed");
        assertEquals("runtime_unavailable", PluginState.luaRuntime(host, false));
        host.put("available", true).remove("error");
        assertEquals("", PluginState.luaRuntime(host, false));
    }
    @Test public void marketShowsDependencyThenInstallationAndRunningState() throws Exception {
        JSONObject entry = new JSONObject().put("name", "autoclaw").put("version", "0.1.2").put("runtime", "lua").put("installable", true);
        JSONObject local = new JSONObject(entry.toString()).put("enabled", true).put("running", true);
        assertEquals("runtime_missing", PluginState.market(entry, null, "runtime_missing"));
        assertEquals("not_installed", PluginState.market(entry, null, ""));
        assertEquals("running", PluginState.market(entry, local, ""));
        assertEquals("runtime_stopped", PluginState.market(entry, local, "runtime_stopped"));
        assertEquals("runtime_unavailable", PluginState.installed(local, "runtime_unavailable"));
        local.put("running", false);
        assertEquals("stopped", PluginState.market(entry, local, ""));
        local.put("enabled", false);
        assertEquals("disabled", PluginState.market(entry, local, ""));
        entry.put("version", "0.1.3");
        assertEquals("update_available", PluginState.market(entry, local, ""));
        assertEquals("runtime_stopped", PluginState.market(entry, local, "runtime_stopped"));
    }
    @Test public void nativePluginsDoNotDependOnLuaAndOfflineInstallsKeepTheirStatus() throws Exception {
        JSONObject entry = new JSONObject().put("name", "native").put("version", "1.0.0").put("runtime", "go")
                .put("installable", false).put("unavailable_reason", "unpublished");
        assertEquals("unpublished", PluginState.market(entry, null, "runtime_missing"));
        JSONObject local = new JSONObject(entry.toString());
        assertEquals("installed", PluginState.market(entry, local, "runtime_missing"));
        local.put("enabled", true);
        assertEquals("enabled", PluginState.installed(local, "runtime_stopped"));
        local.put("available", false);
        assertEquals("unavailable", PluginState.installed(local, ""));
    }
}
