# 26 JSON与序列化

> Go 版本：>= 1.27.1  
> 相关标准库：encoding/json、encoding/json/v2、encoding/json/jsontext

Go 1.27 起标准库里有三层 JSON：**v1**（`encoding/json`，兼容性最好，行为承诺不变）、**v2**（`encoding/json/v2`，默认更严格、选项体系完整）、**jsontext**（令牌级流式读写）。它们不是"新的取代旧的"：既有项目继续用 v1，新项目可以评估 v2，需要跳过大部分内容只读一个字段时才下到 `jsontext`。三层怎么选、差异在哪，见篇末的"v1 与 v2 的选择"和"v1 与 v2 的差异"两节；只想解决"结构体怎么转 JSON"的话，看完 v1 那一节就够用。

## encoding/json

传统 `encoding/json` 仍然是兼容性最好的 JSON 包。Go 1.27 之后它被正式称为 v1 包：它的全部行为现在都是基于 v2 包加上一组保持历史语义的选项实现的：既有 API 的语义承诺不变，但 Go 1.27 也为 v1 包新增了一批导出符号（`Options` 及 `DefaultOptionsV1`、`FormatDurationAsNano`、`CallMethodsWithLegacySemantics` 等选项构造函数，用法见下文“encoding/json/v2”一节）。

### Marshal

```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

u := User{Name: "Tom", Age: 18}

data, err := json.Marshal(u)
if err != nil {
    return err
}

fmt.Println(string(data)) // {"name":"Tom","age":18}
```

### Unmarshal

```go
data := []byte(`{"name":"Tom","age":18}`)

var u User

err := json.Unmarshal(data, &u)
if err != nil {
    return err
}

fmt.Println(u.Name, u.Age) // Tom 18
```

### struct tag

```go
type User struct {
    Name    string `json:"name"`
    Age     int    `json:"age,omitempty"`
    Address string `json:"address,omitempty"`
}
```

常用 tag 选项：

- `-`：忽略字段
- `omitempty`：Go 值为空值时省略（false、0、nil 指针、nil 接口，以及长度为零的数组、切片、map、字符串）
- `omitzero`：Go 值为零值时省略
- `string`：以字符串形式编码数字或布尔值

### omitempty 和 omitzero 的区别

这两个选项看起来接近，但判断标准完全不同：

- `omitzero` 判断的是 **Go 值是不是零值**。类型如果有 `IsZero() bool` 方法就调用它，否则直接比较零值。
- `omitempty` 判断的是**输出结果**。v2 里明确为“编码后是不是空的 JSON 值”，JSON null、空字符串、空对象、空数组**都算空、都会被省略**；v1 里则退化为“Go 值是不是空”，也就是 false、0、nil 指针、nil 接口，以及空的数组、切片、map、字符串。

时间类型是两者分歧最明显的地方。`time.Time` 是结构体，它的零值既不是数字也不是 nil，所以 **`omitempty` 对它无效**：

```go
type Event struct {
    At time.Time `json:"at,omitempty"`
}

data, _ := json.Marshal(Event{})
fmt.Println(string(data)) // {"at":"0001-01-01T00:00:00Z"}，零值时间照样输出
```

`time.Duration` 的底层是 `int64`，情况更微妙：v1 认为 0 属于“空”所以会省略它，v2 认为数字 0 不是空的 JSON 值所以**不**省略，紧接着又发现 v2 没有 Duration 的默认表示，于是直接报错：

```go
type Event struct {
    D time.Duration `json:"d,omitempty"`
}

_, err := jsonv2.Marshal(Event{})
fmt.Println(err) // json: cannot marshal from Go time.Duration within "/d": no default representation
```

同一份结构体在 v1 和 v2 下表现不同，正是官方建议把 bool、数字、指针、接口字段上的 `omitempty` 换成 `omitzero` 的原因。换成 `omitzero` 之后，两个包对这个字段的空值判断就一致了：

```go
type Event struct {
    At time.Time     `json:"at,omitzero"`
    D  time.Duration `json:"d,omitzero"`
}

data, _ := json.Marshal(Event{})
fmt.Println(string(data)) // {}
```

在切片和 map 上两者也有差别：只有 nil 切片或 nil map 会被 `omitzero` 省略，而 `omitempty` 会把非 nil 的空切片、空 map 一并省略。

### 忽略未导出字段

未导出字段不会被 JSON 编码：

```go
type User struct {
    Name   string
    secret string
}
```

### RawMessage

需要延迟解析 JSON 片段时，可以使用 `json.RawMessage`：

```go
type Event struct {
    Type string          `json:"type"`
    Data json.RawMessage `json:"data"`
}
```

### Decoder 与 Encoder

处理流式 JSON 或文件时使用 Decoder、Encoder：

```go
decoder := json.NewDecoder(reader)

var u User
if err := decoder.Decode(&u); err != nil {
    return err
}
```

```go
encoder := json.NewEncoder(writer)

if err := encoder.Encode(u); err != nil {
    return err
}
```

`Decoder.DisallowUnknownFields` 可以在遇到未知字段时返回错误：

```go
decoder := json.NewDecoder(reader)
decoder.DisallowUnknownFields()
```

### 其它常用函数

```go
data, err := json.MarshalIndent(u, "", "  ") // 带缩进的输出，用于人读或落盘
if err != nil {
    return err
}

fmt.Println(json.Valid(data)) // 判断一段字节是否是合法 JSON
```

### HTML 转义

v1 默认会把 JSON 字符串里的 HTML 和 JavaScript 特殊字符转义，`<`、`>`、`&` 会变成 `\u003c`、`\u003e`、`\u0026`：

```go
data, _ := json.Marshal(map[string]string{"u": "a<b>&c"})
fmt.Println(string(data)) // {"u":"a\u003cb\u003e\u0026c"}
```

这是为了避免把 JSON 直接嵌进 HTML 页面时产生 XSS。如果输出不用于 HTML，可以用 Encoder 关掉：

```go
encoder := json.NewEncoder(writer)
encoder.SetEscapeHTML(false)

encoder.Encode(value)
```

### 时间与 Duration 的表示

v1 对两个时间类型的处理是固定的：

- `time.Time` 编码为 RFC 3339 字符串，带纳秒精度，例如 `"2026-08-28T12:00:00Z"`。
- `time.Duration` 编码为**纳秒数**的 JSON 数字，例如 90 秒是 `90000000000`。

这个 Duration 表示很容易被误读（看起来像一个普通数字），跨语言交互时建议自己定义包装类型或改用字符串。

## encoding/json/v2

Go 1.27 标准库提供 `encoding/json/v2`。它提供了新的语义和更丰富的选项。

导入：

```go
import jsonv2 "encoding/json/v2"
```

### Marshal

```go
data, err := jsonv2.Marshal(u)
if err != nil {
    return err
}

fmt.Println(string(data)) // {"name":"Tom","age":18}
```

### Unmarshal

```go
var u User

err := jsonv2.Unmarshal(data, &u)
if err != nil {
    return err
}
```

### Options

v2 的很多行为通过 Options 控制：

```go
data, err := jsonv2.Marshal(u,
    jsonv2.Deterministic(true),
    jsonv2.OmitZeroStructFields(true),
)
if err != nil {
    return err
}
```

```go
err := jsonv2.Unmarshal(data, &u,
    jsonv2.RejectUnknownMembers(true),
)
if err != nil {
    return err
}
```

常用选项：

- `Deterministic`：让 map 的编码顺序稳定（v1 本来就有序，v2 默认无序）
- `OmitZeroStructFields`：省略零值结构体字段
- `RejectUnknownMembers`：拒绝未知 JSON 字段
- `StringifyNumbers`：数字以字符串形式输出
- `MatchCaseInsensitiveNames`：字段名大小写不敏感匹配
- `FormatNilSliceAsNull` / `FormatNilMapAsNull`：让 nil 集合输出成 `null`，而不是 `[]` / `{}`

### Duration 在 v2 中会直接报错

这是从 v1 迁移到 v2 最容易踩的坑：**v2 默认没有 `time.Duration` 的表示方式，编码时会返回错误**。

```go
type Job struct {
    Timeout time.Duration `json:"timeout"`
}

_, err := jsonv2.Marshal(Job{Timeout: 90 * time.Second})
fmt.Println(err) // json: cannot marshal from Go time.Duration within "/timeout": no default representation
```

要让 v2 按 v1 的方式输出纳秒数，需要显式传选项。这个选项由 **v1 包**提供：

```go
b, err := jsonv2.Marshal(Job{Timeout: 90 * time.Second}, json.FormatDurationAsNano(true))
fmt.Println(string(b), err) // {"timeout":90000000000} <nil>
```

`encoding/json` 里的 `Options` 类型是 `jsonv2.Options` 的别名，它导出的这一组选项（`DefaultOptionsV1`、`FormatDurationAsNano`、`CallMethodsWithLegacySemantics` 等）就是用来让 v2 按 v1 的语义运行的，可以直接传给 v2 的函数。所以“不要混用 v1 和 v2”的说法并不准确：**混用这两个包的类型是设计好的用法**，关键是要清楚自己选的是哪一套语义。

### v2 中的 struct tag

v2 支持的 tag 选项包括：

- `-`：忽略字段
- `omitzero`：Go 值为零值时省略
- `omitempty`：编码结果为空 JSON 值时省略
- `string`：把本应编码为数字的字段用字符串包裹，避免 64 位整数在其它 JSON 实现中丢精度
- `case:strict` / `case:ignore`：指定该字段的字段名匹配方式，优先于 `MatchCaseInsensitiveNames` 选项
- `embed`：把该字段的内容提升到父结构体，相当于 JSON 层面的结构体嵌入

注意 `string` 在 v2 中被收紧了：只对“本来会编码成 JSON 数字”的类型生效，对数组、切片、结构体和 map 使用会直接报错。

### Writer 和 Reader

```go
if err := jsonv2.MarshalWrite(writer, u); err != nil {
    return err
}
```

```go
if err := jsonv2.UnmarshalRead(reader, &u); err != nil {
    return err
}
```

### Encoder 与 Decoder

```go
encoder := jsontext.NewEncoder(writer)

if err := jsonv2.MarshalEncode(encoder, u); err != nil {
    return err
}
```

```go
decoder := jsontext.NewDecoder(reader)

if err := jsonv2.UnmarshalDecode(decoder, &u); err != nil {
    return err
}
```

## encoding/json/jsontext

`jsontext` 是 JSON 的底层文本处理包，适合流式读取和写入。同样是"按 token 流式读"，XML 侧的对应物是 `xml.Decoder.Token`（见 `31-编码与序列化.md` 的“encoding/xml”一节）。

### 读取 Token

```go
// NewDecoder：在 reader 上建流式解码器；ReadToken：一次读一个最小单元（{、"name"、123…）
decoder := jsontext.NewDecoder(reader)

for {
    token, err := decoder.ReadToken()
    if errors.Is(err, io.EOF) {
        break
    }
    if err != nil {
        return err
    }

    // Kind 是 token 种类，String 是它的原始字面量
    fmt.Println(token.Kind(), token.String()) // 每个 token 打印一行"种类 字面量"，取决于 reader 里的 JSON
}
```

### 读取完整 Value

```go
// ReadValue：一次读出下一个完整的 JSON 值（对象、数组、标量都算一个），不逐 token 拆分
decoder := jsontext.NewDecoder(reader)

value, err := decoder.ReadValue()
if err != nil {
    return err
}

// Value 可以当 []byte 用：Kind 看种类，直接转字符串就是原始 JSON 文本
fmt.Println(value.Kind()) // 最外层值的种类，取决于 reader 里的 JSON（对象就是 {）

var data []byte = value
fmt.Println(string(data)) // 按原始 JSON 文本打印，内容与 reader 里那一段一致
```

### 写入 Token

```go
encoder := jsontext.NewEncoder(writer)

if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
    return err
}
if err := encoder.WriteToken(jsontext.String("name")); err != nil {
    return err
}
if err := encoder.WriteToken(jsontext.String("Tom")); err != nil {
    return err
}
if err := encoder.WriteToken(jsontext.EndObject); err != nil {
    return err
}
```

## v1 与 v2 的差异

v2 的默认行为更严格，也更接近 RFC 8259 的推荐做法。下面这些差异在迁移时必须逐条确认。每条都可以用对应的选项改回 v1 行为。

| 行为               | v1                                               | v2                                                     |
| ------------------ | ------------------------------------------------ | ------------------------------------------------------ |
| 字段名匹配         | 大小写不敏感                                     | 大小写敏感                                             |
| 重复成员名         | 允许，后者覆盖前者                               | 报错                                                   |
| 非法 UTF-8         | 静默替换为 `U+FFFD`                              | 报错                                                   |
| nil 切片 / map     | 编码为 `null`                                    | 编码为 `[]` / `{}`                                     |
| map 编码顺序       | 固定（按键排序）                                 | 不确定                                                 |
| HTML / JS 转义     | 默认转义 `<`、`>`、`&`                           | 最小转义，只转义 JSON 语法要求的部分                   |
| `time.Duration`    | 编码为纳秒数                                     | 报错，需要 `FormatDurationAsNano`                      |
| 定长数组 `[N]byte` | 编码为数字数组                                   | 编码为 Base64 字符串                                   |
| 数组反序列化       | 长度不匹配也接受                                 | 长度必须完全一致，否则报错                             |
| `omitempty` 判据   | Go 值是否为空（false、0、nil、空集合、空字符串） | 编码结果是否为空的 JSON 值（null、空字符串/对象/数组） |
| `string` 选项      | 可作用于字符串、布尔、数字及其指针               | 只作用于会编码成数字的类型                             |
| JSON null 反序列化 | 有时清零、有时不动，行为不一致                   | 一定清零目标值                                         |
| 结构性错误         | 运行时通常不报错                                 | 报错（例如只有未导出字段的结构体无法序列化）           |
| 指针接收者方法     | 仅在值可寻址时调用 `MarshalJSON`                 | 无论是否可寻址都会调用                                 |
| map 的 key         | 不会调用 `MarshalJSON` / `UnmarshalJSON`         | 会调用                                                 |

字节这一行只适用于定长数组 `[N]byte`：`[]byte` 在两个包里都编码为 Base64 字符串，不受版本影响。

几条实测结果，能直观感受到差异：

```go
m := map[string]int{"c": 3, "a": 1, "b": 2}

v1, err := json.Marshal(m)
fmt.Println(string(v1), err) // {"a":1,"b":2,"c":3} <nil>，v1 每次都是这个顺序

v2, err := jsonv2.Marshal(m)
fmt.Println(string(v2), err) // 顺序随机：v2 的 map 编码不确定，连着跑两次可能就不同
```

```go
s := map[string]string{"s": string([]byte{0xff})}

v1, err := json.Marshal(s)
fmt.Println(string(v1), err) // {"s":"�"} <nil>，非法字节被替换成 U+FFFD 字符

_, err = jsonv2.Marshal(s)
fmt.Println(err) // jsontext: invalid UTF-8 within "/s" after offset 5
```

```go
dup := []byte(`{"a":1,"a":2}`)

out := map[string]int{}
err := json.Unmarshal(dup, &out)
fmt.Println(out, err) // map[a:2] <nil>，前者被后者覆盖

out2 := map[string]int{}
err = jsonv2.Unmarshal(dup, &out2)
fmt.Println(err) // jsontext: duplicate object member name "a"
```

## v1 与 v2 的选择

先按需求定位到层，再看下面的取舍：

| 你的处境 | 用哪一层 | 关键理由 |
| --- | --- | --- |
| 已有项目、不想动行为 | `encoding/json`（v1） | 语义承诺不变，底层已是 v2，长期仍维护 |
| 新项目，想要严格解析与完整选项 | `encoding/json/v2` | 默认拒绝非法 UTF-8 与重复成员名，脏数据当场暴露 |
| 只要读出一两个字段、其余跳过；或按 token 增量写大 JSON | `encoding/json/jsontext` | 不想为了一点内容把整个文档解析成 `any` |
| 想用 v2 但某些字段要保留 v1 行为 | v2 + v1 选项 | `jsonv2.Marshal(v, json.DefaultOptionsV1())`，不必退回 v1 包 |

- **现有项目继续用 `encoding/json`**。它的行为不会变，而且现在底层就是 v2，长期看也会持续维护。
- **新项目在 Go 1.27 上可以评估 `encoding/json/v2`**：默认更严格，能挡掉非法 UTF-8 和重复成员名这类脏数据，选项体系也更完整。
- **需要细粒度流式处理时用 `jsontext`**，例如只要读出一个字段就跳过其余部分（`SkipValue`）、或者按 token 增量写大 JSON。
- **迁移时逐条对照上面的差异表**，尤其是 `time.Duration`、nil 集合输出和 map 顺序这三项，它们最容易被线上监控或下游解析发现。
- **需要保留 v1 语义时直接传 v1 的选项**，例如 `jsonv2.Marshal(v, json.DefaultOptionsV1())`，不必为此回退到 v1 包。

> **关于稳定性**：v2 的**核心语义**（严格 UTF-8、拒绝重复成员名、大小写敏感匹配）是设计目标，不会反复。但**选项体系仍在演进**——`jsonv2.Options` 以及 `encoding/json` 里那批 `*LegacySemantics` / `Format*` 选项是 Go 1.27 一并补齐的，后续小版本仍可能新增或调整。所以启用 v2 时，建议把用到的选项集中封装在一处，别散落在各个调用点上；`encoding/json`（v1）的行为承诺不变，求稳就继续用它。

下一篇进入 [`27-日志.md`](27-日志.md)。
