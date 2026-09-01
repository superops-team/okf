//go:build darwin && amd64

package assets

import _ "embed"

//go:embed libs/darwin/amd64/libonnxruntime.dylib
var ortLibData []byte

const ortLibName = "libonnxruntime.dylib"

// ortLibSHA256 锁定 ORT 1.23.1 osx-x86_64（官方自 1.24.1 起不再发布 x86_64 macOS 包）。
const ortLibSHA256 = "583a6f3738eca06878c32cc1d14adac95af22689ad20d18933dbcc53974ced53"
