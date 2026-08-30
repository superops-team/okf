//go:build windows && arm64

package assets

// 语义搜索暂未为 windows/arm64 内嵌 ONNX Runtime：官方自 v1.24 起已停止发布
// Windows ARM64 的 CPU 动态库，当前未提供该平台资源。在此平台编译时通过引用
// 未定义符号触发显式编译失败（与 ort_unsupported.go 相同模式）。
// 支持平台列表见 scripts/fetch-ort.sh。
var _ = undefinedPlatformRequiresEmbeddedORT
