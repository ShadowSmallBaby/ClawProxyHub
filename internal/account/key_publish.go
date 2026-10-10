//go:build !android

package account

import "os"

func publishKey(source, target string) error { return os.Link(source, target) }
