# 34 runtime 与性能剖析

> Go 版本：>= 1.27.1  
> 相关标准库：runtime、runtime/debug、runtime/pprof、unique、weak

本篇讲与 Go 运行时打交道：用 `runtime` 读运行时状态、用 `runtime/debug` 看调用栈与模块信息、用 `runtime/pprof` 采集 CPU 与内存 profile，另外还有句柄驻留（`unique`）与弱引用（`weak`）这两个运行时能力。进程与信号见上一篇 `33-进程与信号.md`，反射与 `unsafe` 见 `35-reflect与unsafe.md`。

## runtime

什么时候会用到这一节：想在程序里读 CPU 数、堆内存、goroutine 数量，或者排查"内存怎么不降"。**不要把它当监控用**——`ReadMemStats` 内部会 `stopTheWorld`（公开注释里没写这件事，`go doc -src runtime.ReadMemStats` 能看到），频繁调用本身就有成本，真要持续观测请用 `runtime/pprof`（见下文）。

```go
fmt.Println(runtime.NumCPU()) // 逻辑 CPU 数
```

`GOMAXPROCS` 的默认值、容器 CPU 限额和 `SetDefaultGOMAXPROCS` 都在并发篇里讲，见 `41-并发模式与运行时行为.md`。

查看内存统计：

```go
var stats runtime.MemStats

runtime.ReadMemStats(&stats)

fmt.Println(stats.Alloc)      // 当前堆上占用，字节
fmt.Println(stats.TotalAlloc) // 累计分配过，字节
fmt.Println(stats.Sys)        // 向系统申请的总内存，字节
fmt.Println(stats.NumGC)      // 已完成的 GC 次数
// 这四个数都随程序运行状态变化，不能写死
```

主动触发垃圾回收：

```go
runtime.GC()
```

`runtime.GC` 通常只用于调试或特殊场景，不要在生产代码中频繁调用。

### 对象生命周期：cleanup 与弱引用

这一节以及下面的 `unique` 都不属于 `runtime` 包（分别是 `runtime` 的 cleanup、`weak` 包的弱指针、`unique` 包的驻留句柄），放在这里是因为它们都是"运行时替你管对象生命周期"的能力。

GC 回收一个对象时，有时需要顺带释放它包装的外部资源（文件描述符、C 内存、连接）。Go 为这件事提供过 `runtime.SetFinalizer`，但它的坑很多：**不保证一定执行**（程序退出前可能来不及跑）、对象可能在 finalizer 里"复活"、多个 finalizer 的执行顺序不确定、finalizer 里不能做复杂操作。

**Go 1.24 起优先用 `runtime.AddCleanup`**，它把参数和对象分开，避开了 finalizer 的几个主要陷阱：

```go
type Conn struct {
    fd int
}

func NewConn() *Conn {
    c := &Conn{fd: openFD()}
    runtime.AddCleanup(c, func(fd int) {
        closeFD(fd) // 只拿到 fd，拿不到 c —— 不会因为闭包引用而阻止 c 被回收
    }, c.fd)
    return c
}
```

和 `SetFinalizer` 的差别：

- **cleanup 的参数是独立的**，所以清理函数里不会（也不该）引用被回收的对象本身，从根本上避免了"对象复活"。
- 可以用返回的 `runtime.Cleanup` 句柄**提前取消**：`c := runtime.AddCleanup(...); c.Stop()`。
- 同一个对象可以挂**多个** cleanup，执行顺序有保证。

两者共同的限制不变：**不保证执行时机，也不保证一定执行**（进程直接退出就没了）。所以 cleanup 只能当"兜底"，真正的资源释放仍然要靠显式的 `Close()`。

**弱引用**用标准库的 `weak`（也是 Go 1.24 起的）——它持有的引用**不会阻止对象被回收**：

```go
type cacheEntry struct{ data []byte }

var cache = map[string]weak.Pointer[cacheEntry]{}

func lookup(key string) *cacheEntry {
    if w, ok := cache[key]; ok {
        if v := w.Value(); v != nil { // 还在，直接命中
            return v
        }
        delete(cache, key) // 已经被回收，清掉这个失效条目
    }
    return nil
}
```

`weak.Make(ptr)` 得到 `weak.Pointer[T]`，`Value()` 在对象已被回收时返回 `nil`。两个 `weak.Pointer` 相等当且仅当它们由相同的指针创建；文档明确说 **`Value()` 不保证最终一定返回 `nil`**，所以只能把 `nil` 当作"可能已经没了"的信号，不能拿它当"还在"的证明。

典型用途就是上面这种**缓存**、以及 `unique` 包这类"驻留表"：既想复用已有对象，又不想因为缓存本身把对象永久留在内存里。

### unique：把重复的值收敛成句柄

`unique`（Go 1.23 起）做的是**值驻留**（interning，也叫规范化）：把大量相等的值收敛成同一个身份，之后比较身份而不是比较值。

```go
h1 := unique.Make("a-very-long-repeated-string")
h2 := unique.Make("a-very-long-repeated-string")

fmt.Println(h1 == h2)      // true —— 相等当且仅当原值相等
fmt.Println(h1.Value())    // a-very-long-repeated-string
```

`unique.Make[T]` 返回 `unique.Handle[T]`，`T` 必须是可比较类型。要点：

- **句柄的比较是平凡的**（比指针），文档原话是"typically much more efficient than comparing the values used to create them"。所以当值的比较很贵（长字符串、大结构体）时，驻留能显著省时间。
- **可以安全并发使用**（`Make` 和 `Value` 都是）。`Make` 对同一个值只会留下一份，也就顺带省了内存。
- 它不改变值本身：`Value()` 返回的是**浅拷贝**，取回来还是那个值。
- 适用的场景是"同一个值反复出现很多次"——日志里的字段名、URL 路径、枚举字符串、来自 JSON 的大量重复 key。值本身不重复时用它只会白白增加开销。
- **不要把它当缓存用**。它保证的是"相等的东西拿到同一个句柄"，没有过期、容量、淘汰这套机制；缓存该用上面的 `weak.Pointer` 或者专门的缓存库。

## runtime/debug 与 runtime/pprof

`runtime/debug` 是调试入口，最直接的是把当前 goroutine 的调用栈打到标准错误：

```go
debug.PrintStack()
```

在 `recover` 里记录 panic 时，用 `debug.Stack` 拿到栈内容交给日志，这样它才会出现在日志文件和采集系统里，而不是只留在 stderr：

```go
defer func() {
    if r := recover(); r != nil {
        log.Printf("panic: %v\n%s", r, debug.Stack())
    }
}()
```

`runtime/pprof` 按名字取出运行时 profile。排查卡死、goroutine 泄露时最常看的是 goroutine：

```go
if err := pprof.Lookup("goroutine").WriteTo(os.Stderr, 1); err != nil {
    return err
}
```

`WriteTo` 的第二个参数是 `debug`：传 `1` 输出带函数名和行号的可读文本，传 `0` 输出 pprof 二进制格式。除了 `goroutine`，`heap`、`allocs`、`block`、`mutex`、`threadcreate` 都是固定的 profile 名字；`pprof.Lookup` 找不到时返回 `nil`。

GC 调优用 `debug.SetGCPercent`（对应 `GOGC`）、`debug.SetMemoryLimit`（软内存上限）、`debug.FreeOSMemory`（强制 GC 并把内存还给操作系统）。这些都会改变整机行为，**只在 profile 指出问题之后再用**，不要凭感觉调；设置环境变量通常比在代码里调用更合适。

服务进程通常不做手工采样，而是把 profile 挂到 HTTP 上（这里的 goroutine 见 `38-并发基础.md` 的“goroutine：启动之后没人管”一节）：

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

空白导入 `net/http/pprof` 就会把 handler 注册到 `http.DefaultServeMux`，路径都以 `/debug/pprof/` 开头（Go 1.22 起必须用 GET 请求）。这些路径可以直接作为 `go tool pprof` 的数据源：

```bash
go tool pprof http://localhost:6060/debug/pprof/profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

生产环境不要把 pprof handler 暴露到公网，它包含完整的函数名、路径和内存内容摘要。

下一篇进入 [`35-reflect与unsafe.md`](35-reflect与unsafe.md)。
