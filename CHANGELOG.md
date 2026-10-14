# 变更记录

记录正文（`docs/`）、示例（`examples/`）与校验工具（`tools/checkdocs`）的调整。纠错、去重与表述调整走补丁版本号。

## [1.0.1] - 2026-10-14

### 变更

- `00-工具链.md` 删去 `## go tool pprof` 与 `## go tool trace` 两个小节，压成一段指路，指向 `34-runtime与性能剖析.md`。
- `34-runtime与性能剖析.md` 的 trace 一节改名 `## runtime/trace 与 go tool trace：采集与查看`，补上采集用法（`trace.Start`/`trace.Stop`、`go tool trace trace.out`、采集注意事项）；篇首「相关标准库」补 `runtime/trace`。
- `README.md` 目录：00 篇条目去掉 `go tool trace`，34 篇条目补上 `go tool pprof`/`go tool trace`。

## [1.0.0] - 2026-10-13

首个版本：42 篇编号正文（`00`–`41`）、`examples/` 的 130 个可运行示例、`tools/checkdocs` 的四类校验（断链 / 锚点 / 版本标注 / 示例输出）、`Makefile` 与 CI；全部基于 Go 1.27.1，无第三方依赖。
