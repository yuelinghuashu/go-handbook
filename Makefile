.PHONY: test check check-fast verify clean-cache

# 构建缓存固定放用户目录，别让它落在仓库里（一次完整构建 + 反复跑变体可以长到多个 GiB）。
# 若在沙箱里 ~/.cache 不可写，请显式指定别处并随手清理，例如：
#   GOCACHE=/dev/shm/gocache make test && make clean-cache
export GOCACHE ?= $(HOME)/.cache/go-build

# 跑 examples/ 里全部可运行示例（约 130 个：Example + Test/Fuzz）
test:
	go test ./...

# 校验全部文档：断链 / 锚点 / 示例输出。
# 示例是并行跑的（-j，默认按 CPU 数），跑成功的块会按内容缓存起来：
# 冷启动约 10 秒，之后只重跑改动到的那几个块（缓存放用户缓存目录，见 clean-cache）。
check:
	go run ./tools/checkdocs -dir docs

# 只查断链和锚点，不跑代码（很快）
check-fast:
	go run ./tools/checkdocs -dir docs -examples=false

# 提交前跑这个
verify: test check

# 清理缓存：默认的 GOCACHE、万一被指向仓库内的 .gocache，以及 checkdocs 的块级结果缓存
clean-cache:
	go clean -cache
	rm -rf $(CURDIR)/.gocache
	go run ./tools/checkdocs -cache-clear
