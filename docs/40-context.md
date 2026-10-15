# 40 39 context

> Go 版本：>= 1.27.1  
> 相关标准库：context、time

本篇只讲 `context`：**取消信号、deadline 和请求范围内的值**怎么在调用链上传递。它看起来像个"并发包"，其实更像一条**接口约定**——凡是可能耗时的函数都该接受它，而 HTTP、`os/exec`、数据库、日志这些标准库接口早就接受了它。

本篇的前置是 `39-并发基础.md` 里的 goroutine、channel 与 `select`。

Go **不会自动**把取消传下去。跨函数、跨 goroutine 都不会"继承"任何东西，必须由调用方把同一个 `ctx` 显式一路传下去；不传，下游就永远不知道上游已经放弃。

## 为什么需要取消信号

假设一个请求要串行调用三个下游服务，每个都可能卡住。调用方放弃时（用户关掉页面、上层超时），如果没有机制把"放弃"传下去，那三次调用会继续跑完——CPU、连接、内存全白烧。`context` 解决的就是这件事：把"这条调用链可以结束了"以及**为什么结束**（超时还是主动取消）传播下去。

它同时解决三件相关的事：

- **取消**：调用方主动放弃；
- **超时 / deadline**：到点自动放弃，不用自己起 timer；
- **请求范围内的值**：请求 id、认证主体这类元数据跟着调用链走（**只放元数据**，不传业务参数）。

这三件事都要求"一路传下去"，所以 `Context` 是个接口值，靠**参数**传递，而不是全局变量或 goroutine 局部状态。

## 构造函数一览

| 构造函数                  | 返回                        | 什么时候用                                                                   |
| ------------------------- | --------------------------- | ---------------------------------------------------------------------------- |
| `context.Background()`    | 永不取消、无值、无 deadline | 顶层：`main`、`init`、测试、请求入口                                         |
| `context.TODO()`          | 同上                        | 暂时不确定该用哪个时的占位，语义与 `Background()` 相同，便于以后替换         |
| `WithCancel(parent)`      | `ctx, cancel`               | 能主动结束的场景（用户取消、拿到结果提前退出）                               |
| `WithCancelCause(parent)` | `ctx, cancelCause`          | 同上，但要记录**为什么**取消，之后用 `Cause` 取回                            |
| `WithDeadline(parent, t)` | `ctx, cancel`               | 已知**绝对时刻**的截止时间                                                   |
| `WithTimeout(parent, d)`  | `ctx, cancel`               | 已知**相对时长**的截止时间，等价于 `WithDeadline(parent, time.Now().Add(d))` |
| `WithValue(parent, k, v)` | `ctx`                       | 请求范围内的元数据；key 用自定义类型，不要用内置字符串                       |
| `WithoutCancel(parent)`   | `ctx`                       | 请求结束后仍要继续的任务，同时保留请求内的值                                 |
| `AfterFunc(ctx, f)`       | `stop`                      | ctx 取消后自动执行 `f`（在它自己的 goroutine 里）                            |

读状态用三个方法：`Done()` 给出一个在取消时关闭的 channel（**永不取消的 context 返回 `nil`**），`Err()` 给出结束原因，`Value(k)` 取值。

## 使用规则

官方文档列了三条，照着做能少踩很多坑：

1. **不要存进结构体**，而是作为**第一个参数**显式传递：`func Do(ctx context.Context, req *Request) error`。放进结构体就丢掉了"一次调用一个 ctx"的语义。
2. **不要传 `nil`**：没有现成的就传 `context.TODO()`（或 `Background()`）——传 `nil` 会让下游的 `<-ctx.Done()` 直接 panic。
3. **值只放请求范围内的数据**：请求 id、追踪信息、认证主体这类；不要用它传可选参数或"半个配置"，那会让函数签名变得不可见。

惯例上这个参数叫 `ctx`：

```go
func fetch(ctx context.Context, url string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return "", err
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    return resp.Status, nil
}
```

## 取消

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func() {
    select {
    case <-ctx.Done():
        fmt.Println("canceled") // 输出不确定：cancel() 之后主 goroutine 立刻退出，这句常常来不及执行
    }
}()

cancel()
```

取消是**向下传播**的：取消父 context 会连带取消它派生出的所有子 context；反过来，取消某个子 context 不影响父和其他兄弟：

```go
parent, cancelParent := context.WithCancel(context.Background())
child, cancelChild := context.WithCancel(parent)

cancelChild() // 取消子 context：父不受影响
fmt.Println(parent.Err(), child.Err()) // <nil> context canceled

cancelParent() // 取消父 context：子也跟着结束
fmt.Println(child.Err()) // context canceled
```

## 超时与 deadline

```go
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()

select {
case <-ctx.Done():
    fmt.Println(ctx.Err()) // context deadline exceeded —— 1 秒的超时先到
case <-time.After(2 * time.Second):
    fmt.Println("done")
}
```

`WithDeadline` 和 `WithTimeout` 的差别只是"给绝对时刻"还是"给相对时长"。判断结束原因用 `Err()`，它只会返回两个值：`context.Canceled`（被取消）或 `context.DeadlineExceeded`（超时）：

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

<-ctx.Done()

fmt.Println(ctx.Err()) // context deadline exceeded
fmt.Println(errors.Is(ctx.Err(), context.DeadlineExceeded)) // true
fmt.Println(errors.Is(ctx.Err(), context.Canceled))         // false
```

想记录**具体原因**而不是笼统的"超时"，用 `WithTimeoutCause` / `WithDeadlineCause` 传入一个 error，之后用 `context.Cause` 取回（见下面的 Cause 一节）。

## 传递值

```go
type ctxKey struct{}

ctx := context.WithValue(context.Background(), ctxKey{}, "123")

value := ctx.Value(ctxKey{})
fmt.Println(value) // 123 —— 取回的正是 WithValue 存进去的值
```

值的查找是**沿链条向上遍历**：每套一层 `WithValue` 就多一层，`Value` 从最里层一路找到最外层，找不到返回 `nil`。所以不要在每个函数里都套一层值——链太长时，查找会变慢，排查"这个值哪来的"也会变难。

## Cause

取消时想带上**原因**，用 `WithCancelCause` + `Cause`（Go 1.20 起）：

```go
ctx, cancel := context.WithCancelCause(context.Background())

cancel(errors.New("user canceled"))

fmt.Println(context.Cause(ctx)) // user canceled
```

## AfterFunc：取消后自动执行

`context.AfterFunc`（Go 1.21 起）注册一个回调，ctx 被取消后在**自己的 goroutine 里**执行它。它比“自己 `go func` 里 `select` 等 `ctx.Done()`”更简洁，也不会白白占着一个 goroutine：

```go
ctx, cancel := context.WithCancel(context.Background())

done := make(chan struct{})

stop := context.AfterFunc(ctx, func() {
    defer close(done)
    fmt.Println("cleanup ran after cancel")
})
defer stop() // 不再需要时取消注册

cancel()

<-done // 回调在自己的 goroutine 里跑，必须显式等它结束才能断言输出
```

`cancel()` 之后**不能**紧接着打印或断言“回调已经跑完”：回调在自己的 goroutine 里执行，`cancel()` 不会等它。要观察回调的结果，就用 channel（如上）或 `sync.WaitGroup` 显式建立同步。

返回的 `stop` 用来撤销注册：`stop()` 返回 `true` 表示成功阻止了回调执行，返回 `false` 表示 ctx 已经取消、回调已经（或正在）执行，或者回调此前已经被 `stop()` 掉过。注意如果传入的 ctx **已经取消**，`AfterFunc` 会立刻在自己的 goroutine 里执行回调。

## WithoutCancel：脱离父 context 的取消

有时需要在请求结束后继续做一点事（写审计日志、投递异步消息），但又不该顺手丢掉请求范围内的值。`WithoutCancel`（Go 1.21 起）返回一个派生 context，它**不随父 context 取消**，同时保留父 context 的值：

```go
type key struct{}

parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "req-1"))
detached := context.WithoutCancel(parent)

cancel()

fmt.Println(parent.Err())            // context canceled
fmt.Println(detached.Err())          // <nil>——不受父级取消影响
fmt.Println(detached.Done() == nil)  // true——Done 是 nil，永远不会关闭
fmt.Println(detached.Value(key{}))   // req-1——值仍然继承
```

两个必须注意的点：

- `WithoutCancel` 返回的 context **没有 deadline、没有 Done、Cause 也是 nil**。所以传给它的下游调用不会因为父请求结束而中断，也就意味着**你要自己给它套一个超时**，否则后台任务可能无限期挂着。
- 它只影响**取消**，不影响值。这正是它比 `context.Background()` 合适的地方——后者会把请求范围内的元数据一起丢掉。

## 与标准库的衔接

`context` 的价值在于"大家都认这个接口"。标准库里已经接受它的地方：

| 场景         | 接口                                           | 说明                                                              |
| ------------ | ---------------------------------------------- | ----------------------------------------------------------------- |
| HTTP 服务端  | `http.Request.Context()`                       | 取当前请求的 ctx，请求结束时自动取消                              |
| HTTP 客户端  | `http.NewRequestWithContext`                   | 用同一个 ctx 发出站请求（见 `31-网络与HTTP.md`）                  |
| 执行外部命令 | `exec.CommandContext`                          | ctx 取消时杀掉子进程（见 `34-进程与信号.md`）                     |
| 数据库       | `database/sql` 的 `QueryContext`/`ExecContext` | 让慢查询随请求一起结束                                            |
| 操作系统信号 | `signal.NotifyContext`                         | 把 Ctrl+C / SIGTERM 变成一个 ctx（见 `34-进程与信号.md`）         |
| 结构化日志   | `slog.Logger.LogAttrs(ctx, …)`                 | 让日志带上请求上下文（见 `27-日志.md`）                           |
| 超时测试     | `testing/synctest`                             | 用假时钟测超时逻辑，不用真的等（见 `42-并发模式与运行时行为.md`） |

## 常见坑

- **忘记 `cancel`**：`WithCancel`/`WithDeadline`/`WithTimeout` 都会把自己注册到父 context 上，不调用 `cancel` 就一直不释放（`go vet` 的 `lostcancel` 会提醒）。习惯写 `defer cancel()`。
- **把 ctx 存进结构体**：丢掉"一次调用一个 ctx"的语义；长命对象里存的 ctx 还会一直挂着它的父级。
- **传 `nil`**：下游 `<-ctx.Done()` 会 panic；没有现成的就传 `TODO()` / `Background()`。
- **以为 `cancel()` 之后回调已经跑完**：`AfterFunc` 的回调在自己的 goroutine 里，`cancel()` 不等它（上面的 `AfterFunc` 一节给了同步写法）。
- **`WithoutCancel` 不自己套超时**：它没有 deadline、`Done()` 是 `nil`，后台任务可能无限期挂着。
- **用 `ctx.Value` 传业务参数**：签名变隐形、链式查找变慢、取出来还要类型断言——它是给元数据留的。
- **拿 `Background().Done()` 参与 `select`**：永不取消的 context 的 `Done()` 是 `nil` channel，`select` 永远不会选中它，会一直阻塞。

下一篇进入 [`41-同步原语与内存模型.md`](41-同步原语与内存模型.md)。
