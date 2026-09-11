//go:build darwin && arm64

package assets

import _ "embed"

//go:embed libs/darwin/arm64/libonnxruntime.dylib
var ortLibData []byte

//go:embed libs/darwin/arm64/libtokenizers.dylib
var tokenizerLibData []byte

const (
	ortLibName       = "libonnxruntime.dylib"
	tokenizerLibName = "libtokenizers.dylib"
)

// ortLibSHA256 锁定 ORT 1.24.1 osx-arm64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "9626bbdd201fbed7e3addcbd9fc7e97ec4954c03afcf40a9883af50498bffb06"

// tokenizerLibSHA256 锁定 pure-tokenizers rust-v0.1.5 macOS arm64。
const tokenizerLibSHA256 = "f3c04b8d10d85affc270f75c79bd1fc6d68affd3d638215f30d72ec191c01a21"
