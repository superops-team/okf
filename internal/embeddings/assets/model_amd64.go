//go:build amd64

package assets

import _ "embed"

//go:embed models/model_amd64.onnx
var modelData []byte

const modelName = "model_amd64.onnx"

// modelSHA256 锁定 MiniLM-L6-v2 int8 量化（x86 AVX2 model_quint8_avx2.onnx，见 scripts/fetch-model.sh）。
const modelSHA256 = "b941bf19f1f1283680f449fa6a7336bb5600bdcd5f84d10ddc5cd72218a0fd21"
