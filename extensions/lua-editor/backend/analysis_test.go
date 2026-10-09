package luaeditor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAnalyzeParsesMetadataWithoutExecutingSource(t *testing.T) {
	source := "-- local PLUGIN_NAME = 'comment'\n--[[\nlocal PLUGIN_NAME = 'hidden'\n]]\nlocal PLUGIN_NAME = 'actual-plugin'\nlocal PLUGIN_LABEL_ZH = 'Lua 编辑器'\nlocal PLUGIN_LABEL_EN = 'Lua \\" + "\"Editor\\\"'\nerror('must never execute')\nreturn {}"
	result, err := analyze(context.Background(), source)
	if err != nil || !result.Valid || !result.ValidName || result.Name != "actual-plugin" || result.Labels["zh"] != "Lua 编辑器" || result.Labels["en"] != "Lua \"Editor\"" {
		t.Fatalf("analysis: %+v, %v", result, err)
	}
	if result.Bytes != len(source) || result.SHA256 != hashSource(source) || len(result.Diagnostics) != 0 {
		t.Fatalf("source summary: %+v", result)
	}
}

func TestIncompleteSourcePreservesHeaderAndReportsSyntax(t *testing.T) {
	source := "-- local PLUGIN_NAME = 'comment'\nlocal PLUGIN_NAME = 'draft-plugin'\nlocal PLUGIN_LABEL_ZH = [=[草稿]=]\nfunction unfinished(\n"
	result, err := analyze(context.Background(), source)
	if err != nil || result.Valid || !result.ValidName || result.Name != "draft-plugin" || result.Labels["zh"] != "草稿" || len(result.Diagnostics) != 1 || result.Diagnostics[0].Line < 4 {
		t.Fatalf("incomplete source: %+v, %v", result, err)
	}
}

func TestAnalyzeRejectsInvalidTextAndHonorsCancellation(t *testing.T) {
	for _, source := range []string{"nul\x00", "invalid\xff", strings.Repeat("x", (2<<20)+1)} {
		if _, err := analyze(context.Background(), source); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := analyze(ctx, "return {}"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
