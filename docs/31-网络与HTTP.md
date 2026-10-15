# 31 30 网络与HTTP

> Go 版本：>= 1.27.1  
> 相关标准库：net、net/url、net/http、crypto/tls、context、errors、os/signal、syscall

## 与框架的分工

这一篇讲的是**框架底下的标准库底座**：`net/url`、`net/netip`、HTTP 客户端与服务端（含 `ServeMux` 路由、超时、TLS、优雅关闭），以及极少直接使用的裸 TCP。实际项目的服务端几乎都用 gin、echo、chi，路由、参数绑定、校验、中间件都由框架提供，所以本篇只保留**框架不管、但每个项目都要写**的部分。用框架不等于可以不看 `net/http`：框架引擎本身都实现了 `http.Handler`，最终还是要塞进 `http.Server`，而超时、优雅关闭、TLS 这些配置框架不替你做；出站请求框架则完全不提供。路由与中间件见文末“边界”。

正文的小节顺序就是任务顺序：先出站（拼地址 `net/url` → 发请求 → 超时 → 状态码与错误），再入站（`ServeMux` 路由 → `Server` 该配的字段 → 优雅关闭 → TLS → 请求上下文 → 请求体限制），最后是 `netip` 与裸 TCP 两个独立话题。只想发请求的读者看完"HTTP 客户端"一节就可以停；写服务端的从"HTTP 服务端"开始。

## net/url

```go
u, err := url.Parse("https://example.com/path?name=tom&age=18")
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(u.Scheme)   // https
fmt.Println(u.Host)     // example.com
fmt.Println(u.Path)     // /path
fmt.Println(u.RawQuery) // name=tom&age=18

q := u.Query()             // url.Values，即 map[string][]string
fmt.Println(q.Get("name")) // tom
fmt.Println(q.Get("age"))  // 18
```

**`u.Query()` 返回的是副本**，改它不会影响 `u`，必须写回 `RawQuery`：

```go
// Query：把 RawQuery 解析成 url.Values（返回的是副本）；Set：覆盖同名键，Add：追加一个值
q := u.Query()
q.Set("page", "1")
q.Add("tag", "go")
q.Add("tag", "backend")

// Encode：把 url.Values 编码回查询串（会转义特殊字符、按 key 排序）
u.RawQuery = q.Encode() // 关键
fmt.Println(u.String()) // ...?age=18&name=tom&page=1&tag=go&tag=backend
```

从零构造查询串用 `url.Values` 加 `Encode()`，只解析一段查询串用 `url.ParseQuery`，转义和拼路径各有现成函数：

```go
v := url.Values{}
v.Set("q", "golang")
v.Add("tag", "go")
fmt.Println(v.Encode()) // q=golang&tag=go

values, err := url.ParseQuery("a=1&a=2&b=3")
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(values["a"]) // [1 2]

fmt.Println(url.PathEscape("a b/c")) // a%20b%2Fc
fmt.Println(url.QueryEscape("a b"))  // a+b

joined, err := url.JoinPath("https://example.com", "api", "v1", "users")
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(joined) // https://example.com/api/v1/users
```

Go 1.27 给这两个类型都加了 `Clone`：`u.Clone()` 得到 `*url.URL` 的深拷贝，`v.Clone()` 得到 `url.Values` 的深拷贝，需要独立副本时不用再手工复制字段。

## HTTP 客户端

```go
resp, err := http.Get("https://example.com")
if err != nil {
    fmt.Println(err)
    return
}
defer resp.Body.Close()

body, err := io.ReadAll(resp.Body)
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(resp.StatusCode, string(body))
```

这两行的 `http.Get` 内部用的是 `http.DefaultClient`，它的 `Timeout` 是零值，也就是**可以无限等待**——只适合本地实验。

`resp.Body` 必须关闭。想复用底层连接，还要在关闭前把 body 读干净——没读到 EOF 就 `Close`，连接会被丢弃（`io.Copy(io.Discard, resp.Body)` 再 `Close`）。

需要设置 header、指定方法或加超时时，自己构造请求（下面用 `context.WithTimeout` 给这次请求设总时限，`context` 的完整讲解见 `40-context.md`）：

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", nil)
if err != nil {
    return err
}

req.Header.Set("Accept", "application/json")

resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
```

`ctx` 控制整个请求生命周期：拿连接、发请求、读响应头和 body。这是**按单个请求**设超时的正确方式，不要为不同超时反复新建 `Client`。发送 JSON 就用同一个 `client` 加 `bytes.NewReader(data)`，不要图省事用 `http.Post`，它走的同样是没有超时的默认客户端。

### 超时

请求外部服务一律显式带超时：

```go
client := &http.Client{
    Timeout: 5 * time.Second,
}
```

`Client.Timeout` 覆盖**整次**请求（建连、重定向、发送 body、读响应头和 body），粒度粗但省事。需要区分是建连慢还是响应慢时，把超时下移到 `Transport`：

```go
tr := &http.Transport{
    DialContext: (&net.Dialer{
        Timeout:   5 * time.Second, // 建连
        KeepAlive: 30 * time.Second,
    }).DialContext,
    TLSHandshakeTimeout:   5 * time.Second,  // TLS 握手
    ResponseHeaderTimeout: 10 * time.Second, // 等响应头
    IdleConnTimeout:       90 * time.Second, // 空闲连接保留多久
}
```

两者不冲突：`Transport` 管各阶段，`Client.Timeout` 当总兜底。另外两点：**复用同一个 `client`**，它内部有连接池；**不要改 `http.DefaultClient`**，它是全局的，会影响所有依赖它的库。

超时判定用 `errors.Is`，或者从 `*url.Error` 里拿到操作与 URL：

```go
if errors.Is(err, context.DeadlineExceeded) {
    // 我们设的 ctx 到期了
}

if ue, ok := errors.AsType[*url.Error](err); ok {
    fmt.Println(ue.Op, ue.URL, ue.Timeout()) // 操作、URL、是否超时
}
```

`errors.AsType` 是 Go 1.26 起的泛型版 `errors.As`，完整规则见 `05-错误处理.md`。

### 状态码

4xx / 5xx **不是 error**，`err` 只表示传输层失败，所以必须自己看状态码——**只有 2xx 算成功**（201、204、206 都是成功，所以不能写成“不等于 200 就算失败”，通常判 `resp.StatusCode >= 400`），`resp.Status` 里带原因短语。常用常量是 `StatusOK`、`StatusCreated`、`StatusNoContent`、`StatusBadRequest`、`StatusUnauthorized`、`StatusForbidden`、`StatusNotFound`、`StatusTooManyRequests`、`StatusInternalServerError`，方法常量同族（`MethodGet`、`MethodPost`、`MethodPatch` 等）；`http.StatusText(code)` 按码取标准短语，完整清单见 `go doc net/http`。

## HTTP 服务端

服务端的核心只有两个类型：

```go
type Handler interface {
    ServeHTTP(ResponseWriter, *Request)
}

type HandlerFunc func(ResponseWriter, *Request)
```

`HandlerFunc` 让普通函数直接满足 `Handler`；gin、echo、chi 的引擎类型也都实现了 `ServeHTTP`，所以可以原样赋给 `http.Server.Handler`，这就是框架的接入点。理解它还有个实际用处：给框架挂标准库风格的中间件，或者把第三方库提供的 handler 塞进框架路由，都要靠这个接口。

### http.ServeMux：Go 1.22 起不再是"只能做健康检查"

`http.ServeMux` 是标准库自带的路由器。**Go 1.22 起它支持了方法匹配和路径通配符**，能覆盖的服务端场景比过去多得多：

```go
mux := http.NewServeMux()

mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "id=%s", r.PathValue("id"))
})

mux.HandleFunc("POST /users/{id}/posts/{post}", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "id=%s post=%s", r.PathValue("id"), r.PathValue("post"))
})

// {$} 表示到此为止，所以这条只匹配 /exact/，不匹配 /exact
mux.HandleFunc("/exact/{$}", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprint(w, "exact only")
})

mux.HandleFunc("/health", fn)
```

规则要点：

- 模式形如 `[METHOD ][HOST]/[PATH]`，方法省略时匹配所有方法；同一路径同时注册 `GET /x` 和 `/x` 时**更具体的优先**。
- `{name}` 匹配一个路径段，`{name...}` 匹配剩余全部（含 `/`），`{$}` 表示路径结束。
- 通配值用 `r.PathValue(name)` 取；`r.PathValue` 对未定义的段返回空字符串。
- 注册冲突的模式会在 `Handle*` 时**直接 panic**，所以路由错误在启动时就能发现，不会拖到运行时。

框架仍然有它的价值（它管哪些事见文末“边界”）。但"标准库 mux 只能写个健康检查"已经过时了——纯 JSON API、内部服务这类场景完全可以不上框架。

一条不变的注意事项：`http.DefaultServeMux` 是全局变量，任何导入的库都能往里注册 handler（`net/http/pprof` 就是这么做的，见 `35-runtime与性能剖析.md`），生产代码不要往全局 mux 上挂自己的路由，用 `http.NewServeMux()` 建自己的。

### http.Server 该配的字段

直接 `http.ListenAndServe` 用的全是默认值，而其中超时默认是**零，也就是不超时**。线上必须显式配置：

```go
server := &http.Server{
    Addr:    ":8080",
    Handler: handler,

    // 最难的一条：读请求头的超时，缺了它 slowloris 连接能一直占着 goroutine
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       10 * time.Second, // 读整个请求（含 body）
    WriteTimeout:      20 * time.Second, // 写响应，从读完请求头开始计时
    IdleTimeout:       60 * time.Second, // keep-alive 空闲连接保留多久
}
```

`MaxHeaderBytes`（请求头字节上限，默认 1 MiB）也可以在这里设，它和 body 无关。`ReadTimeout` / `WriteTimeout` 是**连接级**的固定截止时间，没法按接口区分；要给不同路由不同预算只能用 `r.Context()`（见下文）。

### 优雅关闭

进程收到信号后不能直接退出，否则在途请求会被切断。标准组合是 `signal.NotifyContext` + `Server.Shutdown`（下面用 goroutine 把 `ListenAndServe` 挪到后台，语法见 `39-并发基础.md` 的“goroutine：启动之后没人管”一节）：

```go
server := &http.Server{Addr: ":8080", Handler: handler}

// 收到 SIGINT / SIGTERM 时 ctx 被取消
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

go func() {
    // ErrServerClosed 是正常关闭路径，不是致命错误
    if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Fatal(err)
    }
}()

<-ctx.Done()
stop() // 恢复信号默认行为，让第二次 Ctrl+C 能强制退出

shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := server.Shutdown(shutdownCtx); err != nil {
    log.Println("shutdown:", err) // 超时，说明有请求卡住了
}
```

要点：

- **`Shutdown` 会等待在途请求返回**：先关 listener（不再接受新连接），再关空闲连接，然后一直等处理中的请求结束。
- **`Shutdown` 之后 `ListenAndServe` 立即返回 `http.ErrServerClosed`**，必须用 `errors.Is` 排除，否则每次正常关闭都走 `log.Fatal`，在 systemd / Kubernetes 里会被记成 crash。
- `Shutdown` **不等待**被 hijack 的连接（WebSocket 等），这类长连接要用 `Server.RegisterOnShutdown` 自己通知；被 `Shutdown` 过的 `Server` 也不可复用。

### TLS

```go
server := &http.Server{
    Addr:              ":8443",
    Handler:           handler,
    TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
    ReadHeaderTimeout: 5 * time.Second,
}

if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
    fmt.Println(err)
    return
}
```

只想一行起服务、不需要配其他字段时，用 `http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", handler)`。

两条安全约定：显式写 `MinVersion: tls.VersionTLS12`，保证将来默认值变化时行为不变；**不要在非测试代码里用 `InsecureSkipVerify: true`**，它跳过证书链与主机名校验，加密还在但对端身份完全不可信。`cert.pem` 要按“服务端证书 + 中间证书”的顺序拼接，顺序错了客户端会报证书链不完整。

### 请求上下文

`r.Context()` 是这个请求的 context，在**客户端断开连接**、收到请求级取消信号、以及 `ServeHTTP` 返回时被取消。这是按请求做精细化超时的入口，也是框架不会替你传的东西：

```go
func handler(w http.ResponseWriter, r *http.Request) {
    // 给这一个请求单独的预算，比连接级超时更精细
    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    // 下游调用都要传 ctx，取消信号才能一路传下去
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    resp, err := client.Do(req)
    if err != nil {
        return // 客户端可能已经走了，不值得再拼响应
    }
    defer resp.Body.Close()
}
```

两个容易出错的地方：

- **链路上任何一环丢掉 `ctx`**（换成 `context.Background()`，或用不带 ctx 的下游调用），客户端断开后取消就无法传播，后面的查询、RPC 会继续空转。
- **需要比请求活得更久的任务**（写审计日志、投递异步消息）必须换 context：`context.WithoutCancel(r.Context())` 不再随请求取消，但保留请求内的值（它没有 deadline 和 Done，所以记得自己再套一个超时，写法见 `40-context.md` 的“WithoutCancel：脱离父 context 的取消”一节）。直接拿 `r.Context()` 起后台 goroutine 是典型 bug——请求一结束它立刻被取消。

### 请求体大小限制

```go
r.Body = http.MaxBytesReader(w, r.Body, 8<<20) // 8 MiB

data, err := io.ReadAll(r.Body)
if err != nil {
    // 先区分“超限”和“其它读取错误”，超限必须回 413，其余按自己的策略处理
    if mbe, ok := errors.AsType[*http.MaxBytesError](err); ok {
        _ = mbe.Limit // == 8<<20
        http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
        return
    }
    http.Error(w, "bad request", http.StatusBadRequest)
    return
}

_ = data // 到这里 data 才是可信的完整请求体，正常路径继续解析它
```

必须把 `*http.MaxBytesError` 单独判成 413，否则会和普通解析错误一起变成 400，客户端不知道要改小请求体。不想改 handler 内部时可以用 `http.MaxBytesHandler` 包一层。

### 逐个请求地控制响应：http.ResponseController

`http.Server` 上的超时是**连接级**的，而 `Flush`、`Hijack`、按请求设置写截止时间这些能力，用的是 `ResponseWriter` 上的可选接口（`http.Flusher`、`http.Hijacker`……）。Go 1.20 起不用再自己写类型断言，`http.NewResponseController(w)` 把它们统一成一组返回 `error` 的方法：

```go
func streamHandler(w http.ResponseWriter, r *http.Request) {
    rc := http.NewResponseController(w)

    // 给这一个响应单独设写截止时间，覆盖 Server.WriteTimeout 里那个连接级的值
    if err := rc.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }

    for _, line := range []string{"a\n", "b\n", "c\n"} {
        fmt.Fprint(w, line)
        if err := rc.Flush(); err != nil { // 立刻把这一行推给客户端，SSE、日志流常用
            return
        }
    }
}
```

三个要点：

- **它靠 `Unwrap() ResponseWriter` 一路往下找**。套在 handler 外面的中间件如果包装了自己的 `ResponseWriter`，就必须实现 `Unwrap()` 返回被包的那个 writer，否则 `rc.Flush()` 之类的调用会拿到 `http.ErrNotSupported`——这是自定义中间件最常见的坑。
- **它把"有没有这个能力"变成可判错的 `error`**：`Flush`/`Hijack`/`SetReadDeadline`/`SetWriteDeadline` 都要判错，`errors.Is(err, http.ErrNotSupported)` 表示底层 writer 不支持。原来那几个隐式接口（`http.Flusher` 等）依然可用，`ResponseController` 只是给了统一入口。
- **`ServeHTTP` 返回之后就不能再用它**（标准库文档明确写了）。要在后台 goroutine 里写响应，先把该设的 deadline 设好。

配套还有 `EnableFullDuplex()`：默认情况下 HTTP/1.1 服务器会先读完请求体再开始写响应，想边读边写（双向流）就用它打开全双工。

### Go 1.27 新增的两个 Server 字段

```go
server := &http.Server{
    Addr:    ":8080",
    Handler: handler,

    // 限制单个请求的“请求头值个数”，0 表示用默认值 500
    MaxHeaderValueCount: http.DefaultMaxHeaderValueCount,

    // 关闭 RFC 9218 客户端优先级，请求改为 round-robin
    DisableClientPriority: true,
}
```

`MaxHeaderBytes` 挡的是 header 太大，`MaxHeaderValueCount` 挡的是“很多个很小的 header”这种 CPU 消耗型攻击，两个维度可以同时设。计数细节略反直觉：同一行里逗号分隔的值算一个，拆成多行则各算一个。`DisableClientPriority` 只在 HTTP/2 且没有自定义 write scheduler 时生效，其他情况是 no-op。

## net/netip：处理地址的正确类型

只要代码里出现"IP 地址"或"IP 网段"，先记住一条：**新代码用 `net/netip`，不要用 `net.IP`**。

`net.IP` 是 `[]byte`，这带来一连串问题：它是可变的、可比较但比较的是字节而不是地址、`nil` 和空切片都"看起来像"地址、把 IPv4 写进 16 字节的 IPv4-mapped 形式时打印出来是 `::ffff:192.0.2.1`。而 `net/netip.Addr` 是**不可变的值类型**，可以安全地当 map key、直接 `==` 比较：

```go
a := netip.MustParseAddr("192.0.2.1")   // 解析失败会 panic，字面量用它
b, err := netip.ParseAddr(userInput)    // 处理外部输入用这个

fmt.Println(a.Is4(), a.IsPrivate())    // true false
fmt.Println(a.Next())                  // 192.0.2.2
fmt.Println(a == netip.MustParseAddr("192.0.2.1")) // true，直接比较

// IPv4 保持 IPv4，不会被悄悄转成 IPv4-mapped 形式
fmt.Println(a.String())                // 192.0.2.1

// 端口：AddrPort 把地址和端口放在一个值里
ap := netip.MustParseAddrPort("192.0.2.1:8080")
fmt.Println(ap.Port(), ap.Addr())      // 8080 192.0.2.1

// 网段
p := netip.MustParsePrefix("192.0.2.0/24")
fmt.Println(p.Contains(a), p.Masked()) // true 192.0.2.0/24
fmt.Println(a.IsValid(), netip.Addr{}.IsValid()) // true false —— 零值无效，用 IsValid 判断
```

和 HTTP 相关的实际用处：

- **`r.RemoteAddr` 是字符串**（形如 `192.0.2.1:54321` 或 `[2001:db8::1]:54321`），取客户端 IP 要解析：

  ```go
  ap, err := netip.ParseAddrPort(r.RemoteAddr)
  if err != nil { /* ... */ }
  ip := ap.Addr()
  ```

  手工用 `strings.Split(r.RemoteAddr, ":")` 取 IP 在 IPv6 上会直接错。

- **判断"是不是内网/回环"**：`ip.IsPrivate()`、`ip.IsLoopback()`、`ip.IsUnspecified()`，比手写网段比较清楚。
- **做黑名单/白名单**：`netip.Prefix` 的 `Contains` 是理想工具；`Addr` 可以当 map key，`net.IP` 不行（它是切片）。
- **注意 HTTP 头里的 `X-Forwarded-For` 是不可信输入**：除非你确知前面有可信代理，否则不能拿它当客户端 IP（它只是客户端可以随便写的字符串）。要做信任链判断，得从最右边按已知代理逐个剥离。

`net.IP` 并没有被废弃，`net.Dial`、`net.Listen`、`http` 等 API 的签名里到处都是它——所以两者互转是常态：

```go
addr := netip.MustParseAddr("192.0.2.1")
ip := addr.AsSlice()          // netip.Addr -> net.IP（[]byte）
back, ok := netip.AddrFromSlice(ip) // net.IP -> netip.Addr，ok 为 false 表示不是合法地址
```

原则是：**外部边界（解析用户输入、读 socket 信息）一进来就转成 `netip`，自己的逻辑里只用 `netip`，要交给标准库 API 时再转回 `net.IP`。**

## 裸 TCP（少见）

应用层几乎不会直接用 `net.Dial` / `net.Listen`，但 `net.Conn` 的超时有个容易踩的点：`SetDeadline` / `SetReadDeadline` / `SetWriteDeadline` 的参数是**绝对时间**（要写 `time.Now().Add(d)`）而不是时长，而且 deadline 作用于这条连接**之后所有**的 I/O 且**不会自动重置**——只设一次的话，那个时刻之后连接再也读不出东西，长连接必须在每次操作前重设。超时判定用 `errors.Is(err, os.ErrDeadlineExceeded)`，而 `net.Error` 的 `Timeout()` 为 true 不一定就是 deadline 超时。

## 边界

本篇只覆盖框架不管的部分，以下内容请查所用框架的文档：路由与路径参数、请求参数绑定与校验、分组路由与框架中间件、统一响应封装、模板渲染与静态文件服务。标准库也能做这些，但项目既然已经在用框架，直接学框架的写法更省事。HTTP 的通用概念（方法、状态码、报文头、Cookie、缓存）与语言无关，在任何语言或框架里都会遇到，本篇不再展开。

CSRF 防护平时由框架中间件负责，但标准库在 Go 1.25 起也提供了 `http.NewCrossOriginProtection()`——没有框架时可以用它包一层 handler（`protection.Handler(mux)`），它靠 `Sec-Fetch-Site` 或 `Origin`/`Host` 比对来拒绝跨源的非安全请求。

下一篇进入 [`32-编码与序列化.md`](32-编码与序列化.md)。
