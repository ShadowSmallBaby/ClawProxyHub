// Package project 使直接 go build 与发行脚本使用同一份项目配置。
package project

import (
	_ "embed"

	"github.com/pelletier/go-toml/v2"
)

//go:embed project.toml
var source []byte

var CoreVersion, UpdateRepository = defaults()

func defaults() (string, string) {
	var config struct {
		Format     int
		Updates    struct{ Repository string }
		Components map[string]struct{ Version string }
	}
	if err := toml.Unmarshal(source, &config); err != nil {
		panic(err)
	}
	if config.Format != 1 || config.Components["core"].Version == "" || config.Updates.Repository == "" {
		panic("invalid project.toml")
	}
	return config.Components["core"].Version, config.Updates.Repository
}
