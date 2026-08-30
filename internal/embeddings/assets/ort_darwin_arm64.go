//go:build darwin && arm64

package assets

import _ "embed"

//go:embed libs/darwin/arm64/libonnxruntime.dylib
var ortLibData []byte

const ortLibName = "libonnxruntime.dylib"

// ortLibSHA256 锁定 ORT 1.24.1 osx-arm64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "9626bbdd201fbed7e3addcbd9fc7e97ec4954c03afcf40a9883af50498bffb06"
