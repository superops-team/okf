# add-agent-native-dynamic-knowledge-tool

为 OKF 增加 agent-native 的动态知识库管理工具：不依赖 MCP、不要求额外安装 CodeGraph 或 okf CLI，以 OKF 现有 Markdown 知识库和代码维度索引为基础，提供仓库索引、增量刷新、结构化检索、上下文打包和机器可读工具接口，供 Bingo/当前 agent 通过内置 `bingo okf` 子命令和 Go package 直接调用。
