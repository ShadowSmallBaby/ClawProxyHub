module github.com/ShadowSmallBaby/ClawProxyHub/extensions/lua-editor/backend

go 1.26.2

require (
	github.com/ShadowSmallBaby/ClawProxyHub v0.0.0
	github.com/yuin/gopher-lua v1.1.1
)

require (
	github.com/golang-migrate/migrate/v4 v4.20.1 // indirect
	github.com/mattn/go-sqlite3 v1.14.52 // indirect
)

replace github.com/ShadowSmallBaby/ClawProxyHub => ../../..
