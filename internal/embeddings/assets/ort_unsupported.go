//go:build !linux && !darwin && !windows

package assets

// 本包仅支持 linux / darwin / windows（需内嵌 ONNX Runtime 动态库）。
// 在不受支持的平台编译时，通过引用未定义符号触发显式编译失败。
// 如需新增平台，先提供对应 ORT 动态库并新增 ort_<goos>_<goarch>.go。
var _ = undefinedPlatformRequiresEmbeddedORT
