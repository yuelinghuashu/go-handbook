# 28 UUID

> Go 版本：>= 1.27.1  
> 相关标准库：uuid、encoding/json

需要生成全局唯一标识符（请求 ID、数据库主键），或者解析、比较外部系统传来的 UUID 时，用 Go 1.27 新增的 `uuid` 包。随机性本身来自 `crypto/rand`（见 `25-数学与随机数.md`），日志见上一篇 `27-日志.md`。

## uuid

导入：

```go
import "uuid"
```

### 生成 UUID

```go
id := uuid.New()
fmt.Println(id) // 每次运行都不同：v4 随机 UUID，形如 1bc019cd-80f5-4e02-abd3-7bb477dd3997
```

`New` 当前等价于 `NewV4`：

```go
id := uuid.NewV4()
```

需要按时间排序的 UUID 时，可以使用：

```go
id := uuid.NewV7()
```

### 关于 `New` 与 `NewV4`

`go doc -src uuid.New` 能直接看到实现，注释把这件事写死了：

```go
// New returns a new UUID.
//
// Programs which do not have a need for a specific UUID generation algorithm should use New.
// At this time, New is equivalent to [NewV4].
func New() UUID {
    return NewV4()
}
```

所以“当前等价”是文档明确承诺的现状，不是猜测；哪天 `New` 换了算法，`NewV4` 仍然固定是 v4。

### RFC 9562 与版本选择

`uuid` 包的文档引用的是 **RFC 9562**（它取代了 RFC 4122，补齐了 v6/v7/v8），包内所有随机位都由 **密码学安全的随机源**生成（内部用 `crypto/rand`）。

- `uuid.NewV4()`：122 位随机数据。生成最快、不泄露任何信息，缺点是纯随机分布作为数据库主键时插入位置到处跳，B-tree 索引页分裂多、缓存局部性差。
- `uuid.NewV7()`：**时间有序**——最高 48 位是大端的毫秒级 Unix 时间戳，后面是版本位和至少 62 位随机数据。Go 的实现还会在 `rand_a` 里放 12 位亚毫秒时间戳，并用包级互斥锁保证“新生成的 UUID 一定排在旧的后面前”（`NewV7` 文档：always returns UUIDs which sort in increasing order, except when the system clock moves backwards）。既有 UUID 的跨系统可生成性，又有近似自增主键的索引局部性，适合做主键和分页游标。
- `uuid.New()`：没有特定算法需求时按文档建议用它。

需要注意的是“时间有序”只保证**生成顺序**，时钟回拨时顺序会被打破；需要严格单调的排序键，应该在数据库里另加一列。

### 解析与格式化

```go
id, err := uuid.Parse("f81d4fae-7dec-11d0-a765-00a0c91e6bf6")
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(id.String()) // f81d4fae-7dec-11d0-a765-00a0c91e6bf6 —— 与输入一致，输出固定规范形式
```

`MustParse` 在解析失败时 panic：

```go
id := uuid.MustParse("f81d4fae-7dec-11d0-a765-00a0c91e6bf6")
```

`Parse`（以及 `UUID.UnmarshalText`）接受四种写法，十六进制字母不区分大小写：

```text
f81d4fae-7dec-11d0-a765-00a0c91e6bf6
{f81d4fae-7dec-11d0-a765-00a0c91e6bf6}
urn:uuid:f81d4fae-7dec-11d0-a765-00a0c91e6bf6
f81d4fae7dec11d0a76500a0c91e6bf6   （不带连字符的 32 位十六进制）
```

而 `String()` 的输出**固定**是小写带连字符的 36 字符形式，所以“解析宽容、输出规范”。

### Nil 与 Max

```go
fmt.Println(uuid.Nil()) // 00000000-0000-0000-0000-000000000000
fmt.Println(uuid.Max()) // ffffffff-ffff-ffff-ffff-ffffffffffff
```

### 比较

`UUID` 是 `[16]byte`，可以直接使用 `==` 比较，也提供 `Compare`：

```go
a := uuid.New()
b := uuid.New()

fmt.Println(a == b) // false
fmt.Println(a.Compare(b)) // 随机得到 -1 或 1：两个随机 UUID 谁大并不确定
```

### 文本与 JSON

`go doc uuid.UUID` 列出的方法一共是这五个：

- `String() string`：`fmt.Stringer`
- `MarshalText() ([]byte, error)`：`encoding.TextMarshaler`
- `UnmarshalText(b []byte) error`：`encoding.TextUnmarshaler`（指针接收者）
- `AppendText(b []byte) ([]byte, error)`：`encoding.TextAppender`（不用额外分配）
- `Compare(v UUID) int`：按 RFC 9562 的大端字节序排序

**它没有实现 `json.Marshaler`/`json.Unmarshaler`**：`UUID` 上没有 `MarshalJSON`/`UnmarshalJSON` 方法。JSON 输出之所以正确，是因为 `encoding/json` 在遇到实现了文本接口的类型时会自动走文本路径：

```go
type User struct {
    ID uuid.UUID `json:"id"`
}

var u User
u.ID = uuid.MustParse("f81d4fae-7dec-11d0-a765-00a0c91e6bf6")

b, _ := json.Marshal(u)
fmt.Println(string(b)) // {"id":"f81d4fae-7dec-11d0-a765-00a0c91e6bf6"}

var back User
_ = json.Unmarshal(b, &back)
fmt.Println(back.ID == u.ID) // true
```

也就是说 JSON 里它是**带引号的字符串**；如果协议对端期望别的形态（数字数组之类），需要自己定义包装类型并实现 `MarshalJSON`。

下一篇进入 [`29-命令行与正则.md`](29-命令行与正则.md)。
