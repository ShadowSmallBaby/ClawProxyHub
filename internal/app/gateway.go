package app

import (
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/gateway"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/module"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/router"
)

func (a *App) gatewayModule() module.Module {
	return &module.Func{Spec: module.Descriptor{ID: "gateway", Requires: []string{"accounts"}, Capabilities: []string{"gateway", "routes", "keys"}},
		RegisterFunc: func(host module.Host) error {
			gw := gateway.New(a.db, a.cfg.DataDir, a.plugins, router.New(a.db), a.accounts, a.settings)
			return host.Handle("/v1/", gw.Handler())
		}}
}
