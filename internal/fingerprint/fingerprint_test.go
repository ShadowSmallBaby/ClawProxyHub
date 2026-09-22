package fingerprint

import "testing"

func TestClaudeHeaders(t *testing.T) {
	h := ClaudeHeaders("")
	if h.Get("user-agent") != DefaultClaudeUserAgent {
		t.Errorf("user-agent = %q", h.Get("user-agent"))
	}
	if h.Get("x-app") != "cli" {
		t.Errorf("x-app = %q", h.Get("x-app"))
	}
	if h.Get("anthropic-beta") == "" {
		t.Error("missing anthropic-beta")
	}
	if h.Get("authorization") != "" {
		t.Error("authorization must not be set")
	}
}

func TestCodexHeaders(t *testing.T) {
	h := CodexHeaders("")
	if h.Get("originator") != "codex-tui" {
		t.Errorf("originator = %q", h.Get("originator"))
	}
	for _, k := range []string{HeaderSessionID, HeaderThreadID, HeaderTurnID, HeaderInstallationID, HeaderWindowID, HeaderTurnMetadata} {
		if h.Get(k) == "" {
			t.Errorf("missing %s", k)
		}
	}
	if h.Get(HeaderThreadID) != h.Get(HeaderSessionID) {
		t.Error("thread_id should default to session_id")
	}
}
