# 30 regexp

> Go 版本：>= 1.27.1  
> 相关标准库：regexp

本篇讲 `regexp`：RE2 语义、匹配与替换，以及几个容易踩的坑（比如 `\d`、`\w` 是 ASCII-only）。**它与命令行参数无关**，那一半在 `29-命令行参数.md`，两篇可以分开读。

## regexp

`regexp` 提供正则表达式支持，采用 RE2 语法。它的实现**没有回溯（backtracking）**，包文档明确保证 “guaranteed to run in time linear in the size of the input”，因此不会出现灾难性回溯（ReDoS）——匹配时间随输入长度线性增长。

代价是两种常见特性**不被支持**，编译时会直接报错：

```go
_, err := regexp.Compile(`(a)\1`) // 反向引用（backreference）
fmt.Println(err) // error parsing regexp: invalid escape sequence: `\1`

_, err = regexp.Compile(`(?=a)`) // 环视（lookahead / lookbehind）
fmt.Println(err) // error parsing regexp: invalid or unsupported Perl syntax: `(?=`
```

`(?<=...)`、`(?<!...)`、`(?!...)` 同样不可用。需要这些语义时要换思路（例如先粗匹配、再用代码判断），或者引入第三方正则引擎。

另一个容易踩的坑是**字符类的 ASCII 语义**：`\d`、`\w`、`\s`、`\b` 都是 ASCII 专用，`regexp/syntax` 的文档写得很明确——“Perl character classes (all ASCII-only)”，`\d` 等价于 `[0-9]`、`\w` 等价于 `[0-9A-Za-z_]`、`\b` 是 ASCII 词边界。要按 Unicode 语义匹配，请用 `\p{...}`：

```go
re := regexp.MustCompile(`^\d+$`)
fmt.Println(re.MatchString("٣")) // false —— 阿拉伯-印度数字不是 ASCII 的 \d

re = regexp.MustCompile(`^\p{Nd}+$`)
fmt.Println(re.MatchString("٣")) // true —— \p{Nd} 是 Unicode 十进制数字
```

### 编译与匹配

```go
re, err := regexp.Compile(`^[a-z]+$`)
if err != nil {
    fmt.Println(err)
    return
}

fmt.Println(re.MatchString("golang")) // true
fmt.Println(re.MatchString("Go123"))  // false
```

模式字符串既可以是字面量常量，也可以是运行时拼出来的（后者必须处理 `Compile` 返回的 error）。确定不会出错时用 `MustCompile`，它在模式非法时 panic 而不是返回 error：

```go
re := regexp.MustCompile(`\d+`)
```

### 查找

```go
re := regexp.MustCompile(`\d+`)

fmt.Println(re.FindString("abc123def456")) // 123
```

查找所有匹配：

```go
re := regexp.MustCompile(`\d+`)

fmt.Println(re.FindAllString("abc123def456", -1)) // [123 456]
```

限制数量：

```go
fmt.Println(re.FindAllString("abc123def456", 1)) // [123]
```

### 子匹配

```go
re := regexp.MustCompile(`(\w+)=(\w+)`)

match := re.FindStringSubmatch("name=tom")

fmt.Println(match[0]) // name=tom
fmt.Println(match[1]) // name
fmt.Println(match[2]) // tom
```

### 命名子匹配

```go
re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
match := re.FindStringSubmatch("name=tom")

for i, name := range re.SubexpNames() {
    if name != "" {
        fmt.Println(name, match[i])
    }
}
// key name
// value tom
```

### 子匹配索引

`SubexpIndex`（Go 1.15）用名字换到子匹配在 `FindStringSubmatch` 结果里的下标，比手写遍历 `SubexpNames` 更直接：

```go
re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)

fmt.Println(re.SubexpIndex("key"))   // 1
fmt.Println(re.SubexpIndex("value")) // 2
fmt.Println(re.SubexpIndex("nope"))  // -1

match := re.FindStringSubmatch("name=tom")
fmt.Println(match[re.SubexpIndex("value")]) // tom
```

`FindAllStringSubmatchIndex` 返回所有匹配的**下标对**，适合在原文里定位或做保留原文的替换：

```go
re := regexp.MustCompile(`(\w+)=(\w+)`)
s := "a=1 b=2"

for _, idx := range re.FindAllStringSubmatchIndex(s, -1) {
    fmt.Printf("key=%s value=%s\n", s[idx[2]:idx[3]], s[idx[4]:idx[5]])
}
// key=a value=1
// key=b value=2
```

每个匹配的下标切片结构是 `[整体开始, 整体结束, 第 1 组开始, 第 1 组结束, ...]`；没有参与匹配的组用 `-1` 表示。

### 转义：把用户输入拼进正则

`regexp.QuoteMeta` 转义所有正则元字符，返回一个“只匹配这段字面量”的模式：

```go
user := "a.b(c)"
re := regexp.MustCompile(regexp.QuoteMeta(user))

fmt.Println(regexp.QuoteMeta(user))     // a\.b\(c\)
fmt.Println(re.MatchString("xa.b(c)y")) // true
```

拼接用户输入（搜索框、文件名过滤、`LIKE` 式查询）时**必须**先转义，否则用户输入的 `.`、`*`、`(` 会改变语义，甚至让 `Compile` 直接失败。

### 替换

```go
re := regexp.MustCompile(`\d+`)

result := re.ReplaceAllString("abc123def456", "#")
fmt.Println(result) // abc#def#
```

使用子匹配：

```go
re := regexp.MustCompile(`(\w+)=(\w+)`)

result := re.ReplaceAllString("name=tom", "$1: $2")
fmt.Println(result) // name: tom
```

注意 `ReplaceAllString` 的替换串会经过 `Expand`：里面的 `$1`、`${name}` 会被当成**子匹配引用**展开。所以当替换文本来自用户输入时，必须改用 `ReplaceAllLiteralString`，否则用户写的 `$1` 会被悄悄替换成别的内容：

```go
userRepl := "$1"

fmt.Println(re.ReplaceAllString("name=tom", userRepl))        // name —— $1 被展开成第一个子匹配
fmt.Println(re.ReplaceAllLiteralString("name=tom", userRepl)) // $1  —— 按字面量插入
```

需要“根据匹配到的内容动态决定替换值”时用 `ReplaceAllStringFunc`，回调收到的就是匹配到的子串，返回值也是**按字面量**插入、不走 `Expand`：

```go
fmt.Println(re.ReplaceAllStringFunc("a=1 b=2", strings.ToUpper)) // A=1 B=2
```

### 最长匹配与匹配策略

默认是 leftmost-first（最左优先，且优先选择靠左的分支），和 Perl 一致。`Longest()` 把策略改成 leftmost-longest：

```go
re := regexp.MustCompile(`a|ab`)

fmt.Println(re.FindString("ab")) // a

re.Longest()
fmt.Println(re.FindString("ab")) // ab
```

`Longest` 修改的是 `Regexp` 自身，**不能和其他方法并发调用**，调用后对所有后续匹配生效。

### 分割

```go
re := regexp.MustCompile(`\s*,\s*`)

parts := re.Split("a, b ,c", -1)
fmt.Println(parts) // [a b c]
```

### 性能建议

- **包级编译一次**：`Compile`/`MustCompile` 要做解析并编译成自动机，是重量级操作，把结果放进包级变量复用：

```go
var numRe = regexp.MustCompile(`\d+`) // 包级，只编译一次

func extract(s string) []string {
    return numRe.FindAllString(s, -1)
}
```

- **并发安全**：`*regexp.Regexp` 可以被多个 goroutine 同时使用（包文档：safe for concurrent use by multiple goroutines），所以“包级编译 + 随处使用”是正确写法，不需要自己加锁。例外是 `Longest()` 这类配置方法，必须在开始并发使用之前调用完；`Copy()` 是早期为减少锁竞争而引入的，现在已标记为 Deprecated。
- 简单固定字符串查找优先使用 `strings.Contains`、`strings.Index`、`strings.HasPrefix`。
- 需要在同一段文本上做多次处理后定位，用 `...Index`/`...SubmatchIndex` 系列拿到下标，避免重复扫描。
- 拼接用户输入前先 `regexp.QuoteMeta`，替换用户输入时用 `ReplaceAllLiteralString`。
- 复杂正则可能比手写解析慢很多，热点路径上先测量再优化。

下一篇进入 [`31-网络与HTTP.md`](31-网络与HTTP.md)。
