package assets

import _ "embed"

//go:embed models/tokenizer.json
var tokenizerData []byte

// tokenizerSHA256 锁定 MiniLM-L6-v2 tokenizer（sentence-transformers 官方，跨平台共用）。
const tokenizerSHA256 = "be50c3628f2bf5bb5e3a7f17b1f74611b2561a3a27eeab05e5aa30f411572037"
