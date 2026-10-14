package examples

// synctest 需要 *testing.T，tools/checkdocs 把代码块包成 main() 跑，验证不了它，
// 所以 39 篇那一节的可靠性由这个测试保证。

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

// 41 synctest：假时钟把 5 秒直接推过去，真实耗时接近 0；超时时间一定是 5s
func TestSynctestTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		done := make(chan string, 1)

		never := make(chan int) // 永远不会有值
		go func() {
			select {
			case v := <-never:
				done <- fmt.Sprintf("received %d", v)
			case <-time.After(5 * time.Second):
				done <- fmt.Sprintf("timeout after %v", time.Since(start))
			}
		}()

		synctest.Wait()                 // 等 bubble 内所有 goroutine 都阻塞住
		synctest.Sleep(5 * time.Second) // 假时钟直接推进

		if got := <-done; got != "timeout after 5s" {
			t.Fatalf("got %q, want %q", got, "timeout after 5s")
		}
	})
}

// 19 t.Context：文档说它"在 Cleanup 执行之前就被取消"，这里把这条语义真的验一遍。
// 顺带钉住那个反直觉的后果——Cleanup 里拿到的 t.Context() 已经是取消状态，
// 所以 srv.Shutdown(t.Context()) 在那里等不到在途请求。
func TestContextCanceledBeforeCleanup(t *testing.T) {
	ctx := t.Context()

	if err := ctx.Err(); err != nil {
		t.Fatalf("测试进行中 ctx 不该已取消: %v", err)
	}
	// 注意 Cleanup 是后进先出：断言和取值必须写在同一个 Cleanup 里，
	// 拆成两个注册会先跑到断言那个（那时还没取值）。
	t.Cleanup(func() {
		if err := ctx.Err(); err != context.Canceled {
			t.Errorf("Cleanup 里 ctx.Err() = %v，期望 context.Canceled", err)
		}
	})
}

// 19 t.Deadline：没设 -timeout 时（go test 默认是 10m，但 -timeout=0 表示不限时）
// 要按 ok 分支处理。这里只钉住"返回的截止时间要么不存在，要么在将来"。
func TestDeadlineIsInTheFuture(t *testing.T) {
	if d, ok := t.Deadline(); ok && !d.After(time.Now()) {
		t.Fatalf("t.Deadline() = %v，已经过期", d)
	}
}
