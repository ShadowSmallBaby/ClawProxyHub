package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"archive/zip"
	"bytes"
	luahost "github.com/ShadowSmallBaby/ClawProxyHub/hosts/luahost"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/platform/androidconn"
	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
	"github.com/ShadowSmallBaby/ClawProxyHub/sdk/transport"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var sessions androidconn.Registry

//export CPHLuaOpen
func CPHLuaOpen(requestFD, callbackFD, bundleFD C.int, directory *C.char) C.longlong {
	requests, e1 := androidconn.Take(int(requestFD))
	callbacks, e2 := androidconn.Take(int(callbackFD))
	f := os.NewFile(uintptr(bundleFD), "script-bundle")
	defer f.Close()
	accepted := false
	defer func() {
		if !accepted {
			if requests != nil {
				requests.Close()
			}
			if callbacks != nil {
				callbacks.Close()
			}
		}
	}()
	if e1 != nil || e2 != nil {
		return 0
	}
	data, err := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if err != nil || len(data) > 8<<20 {
		return 0
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0
	}
	dir, err := os.MkdirTemp(C.GoString(directory), "lua-")
	if err != nil {
		return 0
	}
	defer func() {
		if !accepted {
			os.RemoveAll(dir)
		}
	}()
	total := 0
	seen := map[string]bool{}
	for _, entry := range zr.File {
		name := entry.Name
		if !spec.SafePath(name) || seen[name] || entry.Mode()&os.ModeSymlink != 0 || !(name == "manifest.json" || name == "main.lua" || strings.HasPrefix(name, "lib/") && strings.HasSuffix(name, ".lua")) {
			return 0
		}
		seen[name] = true
		r, e := entry.Open()
		if e != nil {
			return 0
		}
		content, e := io.ReadAll(io.LimitReader(r, 8<<20+1))
		r.Close()
		total += len(content)
		if e != nil || total > 8<<20 {
			return 0
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if os.MkdirAll(filepath.Dir(path), 0700) != nil || os.WriteFile(path, content, 0600) != nil {
			return 0
		}
	}
	if !seen["manifest.json"] || !seen["main.lua"] {
		return 0
	}
	impl := luahost.New(dir)
	session, err := transport.ServePlugin(requests, callbacks, impl)
	if err != nil {
		impl.Close()
		return 0
	}
	accepted = true
	go func() { <-session.Done(); impl.Close(); os.RemoveAll(dir) }()
	return C.longlong(sessions.Add(session))
}

//export CPHLuaClose
func CPHLuaClose(id C.longlong) { sessions.Close(int64(id)) }
func main()                     {}
