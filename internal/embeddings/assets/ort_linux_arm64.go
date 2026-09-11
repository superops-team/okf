//go:build linux && arm64

package assets

import _ "embed"

//go:embed libs/linux/arm64/libonnxruntime.so
var ortLibData []byte

//go:embed libs/linux/arm64/libtokenizers.so
var tokenizerLibData []byte

const (
	ortLibName       = "libonnxruntime.so"
	tokenizerLibName = "libtokenizers.so"
)

// ortLibSHA256 锁定 ORT 1.24.1 linux-aarch64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "7954e8bdedb497f830c6a679e818d98399b7f4d81ade1126c3e0be74d28111ab"

// tokenizerLibSHA256 锁定 pure-tokenizers rust-v0.1.5 linux-aarch64-gnu。
const tokenizerLibSHA256 = "86e1129a13a1ed6f44abc514c044ff5ee1bb6ab3eb4abe18fde730864ec666d7"
