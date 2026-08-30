//go:build linux && amd64

package assets

import _ "embed"

//go:embed libs/linux/amd64/libonnxruntime.so
var ortLibData []byte

const ortLibName = "libonnxruntime.so"

// ortLibSHA256 锁定 ORT 1.24.1 linux-x64（官方 release 提取，见 scripts/fetch-ort.sh）。
const ortLibSHA256 = "e5a7e3646718d8f1f8f52c8fcb770fe229ab44305caf3ea702d558e6e426c9aa"
