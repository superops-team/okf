//go:build arm64

package assets

import _ "embed"

//go:embed models/model.onnx
var modelData []byte

const modelName = "model.onnx"

// modelSHA256 锁定 MiniLM-L6-v2 int8 量化（arm64 专用 model_qint8_arm64.onnx，见 scripts/fetch-model.sh）。
const modelSHA256 = "4278337fd0ff3c68bfb6291042cad8ab363e1d9fbc43dcb499fe91c871902474"
