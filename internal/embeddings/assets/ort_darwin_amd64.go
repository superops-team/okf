//go:build darwin && amd64

package assets

import _ "embed"

//go:embed libs/darwin/amd64/libonnxruntime.dylib
var ortLibData []byte

//go:embed libs/darwin/amd64/libtokenizers.dylib
var tokenizerLibData []byte

const (
	ortLibName       = "libonnxruntime.dylib"
	tokenizerLibName = "libtokenizers.dylib"
)

// ortLibSHA256 锁定 ORT 1.23.1 osx-x86_64（官方自 1.24.1 起不再发布 x86_64 macOS 包）。
const ortLibSHA256 = "583a6f3738eca06878c32cc1d14adac95af22689ad20d18933dbcc53974ced53"

// tokenizerLibSHA256 锁定 pure-tokenizers rust-v0.1.5 macOS x86_64。
const tokenizerLibSHA256 = "be1acf13d3832ba730392ee622e6d9ddab1ab8c1a85a152ee283eabbfa777e72"
