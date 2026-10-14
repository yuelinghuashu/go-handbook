# 35 reflect 与 unsafe

> Go 版本：>= 1.27.1  
> 相关标准库：reflect、unsafe、structs

本篇讲两件"绕过类型系统"的事：`reflect` 在运行时检查类型与值（读结构体标签、动态取值），`unsafe` 加内存布局（`unsafe.Sizeof`/`Offsetof`/`Alignof`、零拷贝转换、`structs.HostLayout`）用来看清一个值在内存里到底占多少、字段落在哪里。前两篇分别讲进程与信号（`33-进程与信号.md`）和 runtime 与性能剖析（`34-runtime与性能剖析.md`）。

读这一篇要先熟悉指针（`10-指针.md`）和接口（`12-接口.md`）：`unsafe.Pointer` 建立在指针之上，`reflect` 的 `Value` 围绕接口的动态类型工作。

## reflect

**先判断要不要用它**：能用接口或泛型表达的，就用它们——反射的代价是编译期检查全丢、性能差（下面列了具体开销），只在"类型只有运行时才知道"的场景才值得，例如通用序列化、依赖注入、ORM。

```go
// ValueOf：把任意值装进 reflect.Value（这里 v 的类型是 reflect.Value，不是 int）
v := reflect.ValueOf(42)

// Kind：底层种类（这里 int）；Type：完整类型（带包名，如 time.Duration）；Int：按 int64 取出值
fmt.Println(v.Kind())       // int
fmt.Println(v.Type())       // int
fmt.Println(v.Int())        // 42
```

检查类型：

```go
var x any = "hello"

t := reflect.TypeOf(x)

fmt.Println(t.Name()) // string
fmt.Println(t.Kind()) // string
```

如果类型在编译期就已经确定，用 `reflect.TypeFor`（Go 1.22 起）直接拿到 `reflect.Type`，不需要先构造一个值：

```go
fmt.Println(reflect.TypeFor[os.PathError]()) // fs.PathError
```

它替代了过去这两种写法：

```go
reflect.TypeOf(os.PathError{})              // 要构造零值
reflect.TypeOf((*os.PathError)(nil)).Elem() // 要绕一圈指针再 Elem

fmt.Println(reflect.TypeOf(os.PathError{}) == reflect.TypeFor[os.PathError]()) // true
```

`go fix` 的 `reflecttypefor` 重写器会把 `reflect.TypeOf(x)` 自动改写成 `reflect.TypeFor[T]()`（类型在编译期已知时才改写，`reflect.TypeOf(r)` 这种动态类型不动），详见 `00-工具链.md`。

反射适合通用序列化、依赖注入、ORM 等基础设施。普通业务代码应优先使用接口和泛型，避免过度反射。

反射的代价包括：

- 运行时开销更高
- 类型错误可能推迟到运行时
- 代码可读性下降

### 读取结构体标签

反射在标准库里最日常的用途是读结构体标签（标签的书写格式与 `go vet` 检查见 `09-struct.md` 的"标签"一节）。`reflect.StructTag` 提供 `Get` 和 `Lookup` 两个方法：

```go
type Config struct {
    Host string `json:"host" xml:"host,attr"`
}

f, _ := reflect.TypeFor[Config]().FieldByName("Host")

// Get：按 key 取值，取不到就返回空字符串；Lookup：多返回一个"是否存在"
fmt.Println(f.Tag.Get("json")) // host
fmt.Println(f.Tag.Get("xml"))  // host,attr
fmt.Println(f.Tag.Get("yaml")) // 空字符串

v, ok := f.Tag.Lookup("json")
fmt.Println(v, ok) // host true
```

`Get` 在 key 不存在时返回空字符串，**无法区分"显式设为空"和"不存在"**，需要区分时用 `Lookup`。

## unsafe

```go
p := unsafe.Pointer(&x)
```

`unsafe.Pointer` 可以在任意指针类型和 `uintptr` 之间转换（**转换规则、合法使用模式和前提见 `10-指针.md`**）；`unsafe.Sizeof`、`unsafe.Alignof`、`unsafe.Offsetof` 给出类型大小、对齐和字段偏移，对固定大小的类型它们是编译期常量：

```go
fmt.Println(unsafe.Sizeof(int64(0))) // 8
```

这三个函数是观察内存布局的唯一手段，实测见下文"内存布局与 unsafe"一节。

### 零拷贝的字符串与切片转换

`unsafe.Add` 和 `unsafe.Slice` 自 Go 1.17 起提供，`unsafe.String`、`unsafe.StringData`、`unsafe.SliceData` 自 Go 1.20 起提供（版本划分容易记错，`unsafe.Slice` 比同名的 `SliceData` 早了三个版本），用它们替代过去 `unsafe.Pointer` 加强制转换的老写法：

```go
b := []byte("hello")

// []byte -> string，不复制
s := unsafe.String(&b[0], len(b))
fmt.Println(s) // hello

// string -> *byte
p := unsafe.StringData(s)
fmt.Println(*p) // 104

// *byte -> []byte，不复制
bs := unsafe.Slice(p, len(s))
fmt.Println(bs) // [104 101 108 108 111]
```

`unsafe.Slice` 也常用来把数组（或 C 数组）当成切片使用：

```go
arr := [4]int{1, 2, 3, 4}

s := unsafe.Slice(&arr[0], len(arr))
fmt.Println(s) // [1 2 3 4]
```

反向的 `unsafe.SliceData` 返回切片底层数组首元素的指针，指针偏移用 `unsafe.Add`（不要转成 `uintptr` 做算术再转回来）：

```go
// SliceData：拿到切片底层数组首元素的指针
p := unsafe.SliceData(arr[:])
fmt.Println(*p) // 1

// Add：在指针上做字节偏移；Sizeof：一个元素占多少字节（这里 8）
q := unsafe.Add(unsafe.Pointer(p), unsafe.Sizeof(arr[0]))
fmt.Println(*(*int)(q)) // 2
```

几个必须记住的约束：

- 这些转换**共享底层内存**，不是拷贝。`unsafe.String` 得到的字符串只要还存在，原始字节就绝不能修改；`unsafe.Slice` 得到的切片超出原数组范围的访问是未定义行为
- `unsafe.String` 在 `len` 为负，或者 `ptr` 为 `nil` 而 `len` 不为 0 时会 panic
- `unsafe.StringData` 对空字符串的返回值未指定，可能是 `nil`
- `go vet` 能查出部分 `unsafe.Pointer` 的非法用法，但 vet 不报错不代表代码合法

`unsafe` 只应在非常底层、性能敏感或与 C 交互的场景中使用，而且应该先有 profile 证据。普通业务代码不应依赖它；上面这些“优化”在大多数程序里省下的时间远小于维护风险。

## 内存布局与 unsafe

**这一节偏底层，初读可以跳过**——除非你要和 C 共享内存、或要为大对象抠内存，否则知道"字段顺序会影响结构体大小"就够了。

所有字节数都以 **64 位平台**（`GOARCH=amd64`、`arm64`、`riscv64` 等）为准：32 位平台上 `int` 和指针是 4 字节，`string` 和接口值是 8 字节，下面这些数字会相应变小。字节数是用来理解规则的，不要写进业务代码做判断。

### 数组：连续且无填充

数组本身没有头部，也不在元素之间插入额外填充，所以 `unsafe.Sizeof` 正好等于"元素大小 × 长度"（16 = 4 × 4，32 = 4 × 8）：

```go
fmt.Println(unsafe.Sizeof([4]int32{})) // 16
fmt.Println(unsafe.Sizeof([4]int64{})) // 32
fmt.Println(unsafe.Sizeof([3]int32{})) // 12
fmt.Println(unsafe.Sizeof([7]bool{}))  // 7
```

相邻元素的地址差正好是一个元素的大小：

```go
var a [4]int32

step := uintptr(unsafe.Pointer(&a[1])) - uintptr(unsafe.Pointer(&a[0]))
fmt.Println(step) // 4
```

元素如果是结构体，步长就是结构体本身的大小（含结构体内部的对齐填充）：

```go
var s [4]struct {
    X int32
    Y int64
}

step := uintptr(unsafe.Pointer(&s[1])) - uintptr(unsafe.Pointer(&s[0]))
fmt.Println(step == unsafe.Sizeof(s[0])) // true（具体字节数取决于平台，32 位上是 12）
```

数组的数组同样是一整块连续内存，第二行紧接第一行：

```go
var m [2][3]int32

d := uintptr(unsafe.Pointer(&m[1][0])) - uintptr(unsafe.Pointer(&m[0][0]))
fmt.Println(d) // 12，等于 unsafe.Sizeof([3]int32{})
```

这里把指针转成 `uintptr` 只是为了**测量差值**，不涉及指针运算；真正需要按偏移访问字段时要用 `unsafe.Add`（见上文"unsafe"一节）。

长度为零的数组 `[0]T` 的 `unsafe.Sizeof` 是 `0`，不占空间：

```go
var z [0]int

fmt.Println(len(z))                   // 0
fmt.Println(unsafe.Sizeof([0]int{}))  // 0
```

但 `[0]T` 作为结构体字段出现时，它的**对齐要求**仍可能给结构体带来填充，不要把它当成绝对"免费"。

### 结构体：填充与字段顺序

结构体的大小不等于各字段大小之和。每个字段的起始地址必须满足它自己的对齐要求，编译器会在字段之间和结构体末尾插入填充字节（padding）。因此**字段顺序会直接改变结构体大小**：

```go
type Bad struct {
    A int8
    B int64
    C int8
}

type Good struct {
    B int64
    A int8
    C int8
}

fmt.Println(unsafe.Sizeof(Bad{}))  // 24
fmt.Println(unsafe.Sizeof(Good{})) // 16
```

`Bad` 里 `B` 需要 8 字节对齐，所以 `A` 后面填充 7 字节，`C` 后面再填充 7 字节，让整个结构体的大小也是 8 的倍数。`Good` 把 `int64` 放在最前面，只在尾部留下填充，省下 8 字节。

### 偏移与对齐

`unsafe.Offsetof` 返回字段相对于结构体起始地址的字节偏移，可以直接看到填充的位置：

```go
fmt.Println(unsafe.Offsetof(Bad{}.A))  // 0
fmt.Println(unsafe.Offsetof(Bad{}.B))  // 8
fmt.Println(unsafe.Offsetof(Bad{}.C))  // 16

fmt.Println(unsafe.Offsetof(Good{}.B)) // 0
fmt.Println(unsafe.Offsetof(Good{}.A)) // 8
fmt.Println(unsafe.Offsetof(Good{}.C)) // 9
```

宽度混用时同样有填充，例如 `struct{ A int8; B int32; C int8 }` 的大小是 12：`A` 后面填充 3 字节，`C` 后面再填充 3 字节。

```go
type Mixed struct {
    A int8
    B int32
    C int8
}

fmt.Println(unsafe.Sizeof(Mixed{})) // 12

fmt.Println(unsafe.Offsetof(Mixed{}.A)) // 0
fmt.Println(unsafe.Offsetof(Mixed{}.B)) // 4
fmt.Println(unsafe.Offsetof(Mixed{}.C)) // 8
```

`unsafe.Alignof` 返回某个表达式的对齐要求，也就是能保证地址是它整数倍的最大值，它等于 `reflect.Type.Align()` 的返回值（`unsafe.Alignof` 的文档原文写作 `reflect.TypeOf(x).Align()`，同样可以写成 `reflect.TypeFor[T]().Align()`）：

```go
fmt.Println(unsafe.Alignof(int8(0)))  // 1
fmt.Println(unsafe.Alignof(int32(0))) // 4
fmt.Println(unsafe.Alignof(int64(0))) // 8
fmt.Println(unsafe.Alignof(Bad{}.B))  // 8
```

### 值的大小 vs 引用的大小

`unsafe.Sizeof` 返回的是值本身的大小，**不包含它引用的内存**。所以 `string` 和接口值都是 16 字节（指针 + 长度，或类型指针 + 数据指针），而空结构体是 0：

```go
var name string

fmt.Println(unsafe.Sizeof(name))         // 16
fmt.Println(unsafe.Sizeof(any(nil)))     // 16
fmt.Println(unsafe.Sizeof(struct{}{}))   // 0
```

按字段宽度从大到小排列通常最省内存，但可读性更重要。只有结构体实例非常多、或者需要和外部二进制格式对齐时，才值得为布局专门调整字段顺序。

### structs.HostLayout

Go 1.23 起，标准库提供 `structs.HostLayout` 这个标记类型，用来告诉编译器和宿主工具链：这个结构体必须按照宿主的期望布局，通常指宿主的 C ABI。

```go
type S struct {
    _ structs.HostLayout
    X int32
    Y int64
}

fmt.Println(unsafe.Sizeof(S{})) // 16
```

按约定，它应该作为名为 `_` 的字段放在结构体定义的第一个字段位置。它适用于需要与 C 代码共享内存、或者要和系统调用参数结构体对齐的场景。

`HostLayout` 只作用于它所在的那个结构体：它不影响该结构体中其他结构体类型字段的内部布局，也不影响包含这个结构体的外层结构体的布局。

## 想看真实的符号与汇编

前面用 `unsafe.Sizeof`/`Offsetof` 量的是**源码层面的布局**。想知道编译器最终把它编译成了什么，有两个底层工具（都在 `go tool` 下，用法见 `go tool nm -h` / `go tool objdump -h`）：

- **`go tool nm binary`**：列出二进制里的符号、地址与大小。查"某个方法有没有被内联掉""泛型实例化后的符号叫什么"时最直接；`-size` 能按大小排序，看体积大户。
- **`go tool objdump -S binary`**：反汇编，`-S` 把 Go 源码逐行混在汇编里。它是**验证"某个优化到底有没有生效"的最终手段**——比读 `-gcflags='-m'` 的报告更硬：报告说"没有逃逸"，反汇编里就不该出现分配调用。

看自己包的汇编用 `go build -gcflags='-S'` 更省事（见 `00-工具链.md` 的"go build -gcflags"）；`go tool objdump` 的价值在于**对已经编译好的二进制**做同样的事，排查线上问题时不用重新构建。

下一篇进入 [`36-优先队列与模板.md`](36-优先队列与模板.md)。
