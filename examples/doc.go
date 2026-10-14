// Package examples 存放 docs/ 学习文档里"应当能跑出所写结果"的核心示例。
//
// 每个示例都是一个 Example 函数，末尾用 `// Output:` 注释写出期望输出，
// `go test ./...` 会真的执行它并逐字比对 —— 文档和代码在这里不会各说各话。
//
// 约定与覆盖范围见同目录的 README.md。
package examples
