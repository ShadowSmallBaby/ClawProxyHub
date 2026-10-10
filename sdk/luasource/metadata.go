// Package luasource 只静态解析 Lua 元数据，供编辑器与工作区共享，不创建 VM 或执行源码。
package luasource

import (
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"
)

// Metadata 读取顶层 local 字符串声明；语法未完成时仍返回完整文件头及原始语法错误。
func Metadata(source string) (map[string]string, error) {
	chunk, err := parse.Parse(strings.NewReader(source), "main.lua")
	if err != nil {
		return headerMetadata(source), err
	}
	metadata := map[string]string{}
	for _, statement := range chunk {
		if declaration, ok := statement.(*ast.LocalAssignStmt); ok {
			for i, name := range declaration.Names {
				if i < len(declaration.Exprs) {
					if value, ok := declaration.Exprs[i].(*ast.StringExpr); ok {
						if _, exists := metadata[name]; !exists {
							metadata[name] = value.Value
						}
					}
				}
			}
		}
	}
	return metadata, nil
}

func headerMetadata(source string) map[string]string {
	metadata := map[string]string{}
	scanner, lexer := parse.NewScanner(strings.NewReader(source), "main.lua"), &parse.Lexer{}
	next := func() ast.Token {
		token, err := scanner.Scan(lexer)
		if err != nil {
			return ast.Token{Type: parse.EOF}
		}
		lexer.PrevTokenType = token.Type
		return token
	}
	token := next()
	for {
		if token.Type == ';' {
			token = next()
			continue
		}
		if token.Type != parse.TLocal {
			return metadata
		}
		var names, values []string
		for {
			token = next()
			if token.Type != parse.TIdent {
				return metadata
			}
			names = append(names, token.Str)
			token = next()
			if token.Type != ',' {
				break
			}
		}
		if token.Type != '=' {
			return metadata
		}
		for {
			token = next()
			if token.Type != parse.TString {
				return metadata
			}
			values = append(values, token.Str)
			token = next()
			if token.Type != ',' {
				break
			}
		}
		// 字面量后仍有运算符或调用时，不把表达式的一部分误认为插件身份。
		switch token.Type {
		case parse.EOF, ';', parse.TLocal, parse.TFunction, parse.TReturn, parse.TBreak,
			parse.TDo, parse.TFor, parse.TWhile, parse.TRepeat, parse.TIf, parse.TIdent:
		default:
			return metadata
		}
		for i, name := range names {
			if i < len(values) {
				if _, exists := metadata[name]; !exists {
					metadata[name] = values[i]
				}
			}
		}
	}
}
