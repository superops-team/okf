//go:build linux && amd64

package assets

import _ "embed"

//go:embed libs/linux/amd64/libonnxruntime.so
var ortLibData []byte

//go:embed libs/linux/amd64/libtokenizers.so
var tokenizerLibData []byte

const (
	ortLibName       = "libonnxruntime.so"
	tokenizerLibName = "libtokenizers.so"
)

// ortLibSHA256 锁定 ORT 1.24.1 linux-x64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "e5a7e3646718d8f1f8f52c8fcb770fe229ab44305caf3ea702d558e6e426c9aa"

// tokenizerLibSHA256 锁定 pure-tokenizers rust-v0.1.5 linux-x86_64-gnu。
const tokenizerLibSHA256 = "1554be874461ac70037c9a2375b0c043588bce663bf39a68d1628a67c0cdc8fe"
