package luasource

import (
	"reflect"
	"testing"
)

func TestMetadataReadsOnlyStaticTopLevelDeclarations(t *testing.T) {
	source := `-- local PLUGIN_NAME = 'line-comment'
--[=[
local PLUGIN_NAME = 'block-comment'
]=]
local PLUGIN_NAME, PLUGIN_LABEL_ZH = 'actual-plugin', [=[中文]=]
local PLUGIN_LABEL_EN = "Lua \"Editor\""
local PLUGIN_NAMES = 'different-variable'
local computed = 'prefix' .. os.getenv('NAME')
local plugin = {}
function plugin.run()
  local PLUGIN_NAME = 'nested'
end
error('must not execute')
return plugin`
	metadata, err := Metadata(source)
	want := map[string]string{
		"PLUGIN_NAME": "actual-plugin", "PLUGIN_LABEL_ZH": "中文",
		"PLUGIN_LABEL_EN": `Lua "Editor"`, "PLUGIN_NAMES": "different-variable",
	}
	if err != nil || !reflect.DeepEqual(metadata, want) {
		t.Fatalf("metadata = %#v, %v; want %#v", metadata, err, want)
	}
}

func TestMetadataPreservesOnlyCompleteLiteralHeaderWhenSourceIsIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         map[string]string
	}{
		{"function", "--[[ local PLUGIN_NAME = 'comment' ]]\nlocal PLUGIN_NAME = 'draft'; local PLUGIN_LABEL_ZH = [=[草稿]=]\nfunction unfinished(", map[string]string{"PLUGIN_NAME": "draft", "PLUGIN_LABEL_ZH": "草稿"}},
		{"multiple", "local PLUGIN_NAME, PLUGIN_LABEL_ZH = 'draft', '草稿'\nfunction unfinished(", map[string]string{"PLUGIN_NAME": "draft", "PLUGIN_LABEL_ZH": "草稿"}},
		{"computed", "local PLUGIN_NAME = 'prefix' .. suffix\nfunction unfinished(", map[string]string{}},
		{"unfinished label", "local PLUGIN_NAME = 'draft'\nlocal PLUGIN_LABEL_ZH = 'unfinished", map[string]string{"PLUGIN_NAME": "draft"}},
		{"nested", "function unfinished()\nlocal PLUGIN_NAME = 'nested'", map[string]string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata, err := Metadata(tc.source)
			if err == nil || !reflect.DeepEqual(metadata, tc.want) {
				t.Fatalf("metadata = %#v, %v; want %#v and syntax error", metadata, err, tc.want)
			}
		})
	}
}
