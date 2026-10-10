// 源码分析只解析 Lua，不创建 VM，也不执行待编辑的脚本。
package luaeditor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	ext "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/luasource"
	"github.com/yuin/gopher-lua/parse"
)

var pluginName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

type Diagnostic struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

type Analysis struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	SHA256      string            `json:"sha256"`
	Valid       bool              `json:"valid"`
	ValidName   bool              `json:"valid_name"`
	Lines       int               `json:"lines"`
	Bytes       int               `json:"bytes"`
	Diagnostics []Diagnostic      `json:"diagnostics"`
}

func hashSource(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func sourceAnalysis(ctx context.Context, _ *ext.Host, raw json.RawMessage) (any, error) {
	var input struct {
		Content string `json:"content"`
	}
	if err := ext.DecodeStrict(raw, &input); err != nil {
		return nil, err
	}
	return analyze(ctx, input.Content)
}

func analyze(ctx context.Context, source string) (Analysis, error) {
	result := Analysis{Labels: map[string]string{}, SHA256: hashSource(source), Lines: strings.Count(source, "\n") + 1, Bytes: len(source), Diagnostics: []Diagnostic{}}
	if err := validateSource(source); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	metadata, err := luasource.Metadata(source)
	if err == nil {
		result.Valid = true
	} else {
		diagnostic := Diagnostic{Line: result.Lines, Column: 1, Message: err.Error()}
		var syntax *parse.Error
		if errors.As(err, &syntax) {
			if syntax.Pos.Line > 0 {
				diagnostic.Line = syntax.Pos.Line
			}
			diagnostic.Column = max(1, syntax.Pos.Column)
			diagnostic.Message = syntax.Message
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	result.Name = metadata["PLUGIN_NAME"]
	result.ValidName = pluginName.MatchString(result.Name)
	for language, key := range map[string]string{"zh": "PLUGIN_LABEL_ZH", "en": "PLUGIN_LABEL_EN"} {
		if label := metadata[key]; label != "" {
			result.Labels[language] = label
		}
	}
	return result, ctx.Err()
}

func validateSource(source string) error {
	if len(source) > 2<<20 || !utf8.ValidString(source) || strings.ContainsRune(source, 0) {
		return fmt.Errorf("source must be UTF-8 text of at most 2 MiB without NUL bytes")
	}
	return nil
}
