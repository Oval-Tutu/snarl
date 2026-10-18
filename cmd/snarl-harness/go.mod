module github.com/oval-tutu/snarl/cmd/snarl-harness

go 1.26.9

toolchain go1.27.2

require (
	github.com/hajimehoshi/ebiten/v2 v2.10.4
	github.com/oval-tutu/snarl v0.0.0
)

require (
	github.com/ebitengine/gomobile v0.0.0-20260820040257-d11f821a26a6 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/oto/v3 v3.5.1 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/jfreymuth/pulse v0.1.3 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/oval-tutu/snarl => ../..

replace github.com/hajimehoshi/ebiten/v2 => github.com/flexiondotorg/ebiten/v2 v2.0.0-20261016133855-5d80ed875d23
