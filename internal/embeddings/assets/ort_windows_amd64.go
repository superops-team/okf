//go:build windows && amd64

package assets

import _ "embed"

//go:embed libs/windows/amd64/onnxruntime.dll
var ortLibData []byte

//go:embed libs/windows/amd64/tokenizers.dll
var tokenizerLibData []byte

const (
	ortLibName       = "onnxruntime.dll"
	tokenizerLibName = "tokenizers.dll"
)

// ortLibSHA256 锁定 ORT 1.24.1 win-x64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "8a1aad8d59d02a5337d4e3f5bbd1158c3f7bf84fe3b3f0052f957dd3e75a91cb"

// tokenizerLibSHA256 锁定 pure-tokenizers rust-v0.1.5 Windows x86_64 MSVC。
const tokenizerLibSHA256 = "9525d1b060aad5a19156fcbecb83bda5dc8d04cf8fa0219174fe97f3efe4f970"
