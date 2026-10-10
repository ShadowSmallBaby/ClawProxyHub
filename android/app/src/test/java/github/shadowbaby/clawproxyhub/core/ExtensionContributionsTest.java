package github.shadowbaby.clawproxyhub.core;

import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;
import static org.junit.Assert.*;

public class ExtensionContributionsTest {
    @Test public void contributionsRespectActivationEnvironmentAndEditableSource() throws Exception {
        JSONObject manifest = new JSONObject().put("id", "lua-editor")
                .put("pages", new JSONArray().put(new JSONObject().put("id", "editor")))
                .put("contributions", new JSONArray()
                        .put(new JSONObject().put("id", "new").put("page", "editor").put("location", "plugins.toolbar").put("label", "New"))
                        .put(new JSONObject().put("id", "edit").put("page", "editor").put("location", "plugins.item.actions").put("when", "editable")));
        JSONObject state = new JSONObject().put("available", true).put("manifest", manifest);
        JSONArray states = new JSONArray().put(state);
        assertEquals(1, ExtensionContributions.list(states, "plugins.toolbar", false).length());
        assertEquals(0, ExtensionContributions.list(states, "plugins.item.actions", false).length());
        JSONObject entry = ExtensionContributions.list(states, "plugins.item.actions", true).getJSONObject(0);
        assertEquals("#/extensions/lua-editor/editor?contribution=edit&name=example", ExtensionContributions.route(entry, "example"));
        state.put("available", false);
        assertEquals(0, ExtensionContributions.list(states, "plugins.toolbar", true).length());
        state.put("available", true);
        manifest.put("environments", new JSONArray().put("web-desktop"));
        assertEquals(0, ExtensionContributions.list(states, "plugins.toolbar", true).length());
        manifest.put("environments", new JSONArray().put("app"));
        assertEquals(1, ExtensionContributions.list(states, "plugins.toolbar", true).length());
        manifest.put("pages", new JSONArray());
        assertEquals(0, ExtensionContributions.list(states, "plugins.toolbar", true).length());
    }
}
