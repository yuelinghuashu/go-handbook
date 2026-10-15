# 08 map

> Go 版本：>= 1.27.1  
> 相关标准库：maps、slices

## 声明与初始化

```go
var m map[string]int // 只是声明，此时是 nil map，还不能写入
```

必须初始化后才能写入。空 map 直接写字面量 `map[K]V{}`；`make` 用在这里只有一个场景——元素数量已知时一次给出容量提示 `make(map[K]V, n)`，省掉边写边扩容：

```go
m1 := map[string]int{} // 空 map：可以读、写、range，且不是 nil
m2 := map[string]int{ // 字面量直接给出初始内容
    "a": 1,
    "b": 2,
}
```

## map 的零值语义

map 的零值是 `nil`。"声明过但没初始化"和"初始化成空 map"是两种不同状态：

```go
var a map[string]int
b := map[string]int{}

fmt.Println(a == nil) // true
fmt.Println(b == nil) // false
fmt.Println(len(a), len(b)) // 0 0
```

两者的长度都是 0，但行为不同：

```go
var a map[string]int

fmt.Println(a["missing"]) // 0 —— nil map 可以读，读不存在的 key 同样返回零值
for k := range a {        // 合法，循环次数为 0 —— nil map 也可以 range
    _ = k
}
delete(a, "k")            // 合法，什么都没发生

// a["x"] = 1 // panic: assignment to entry in nil map
```

- **`nil` map 可以读、可以 range、可以 `delete`，唯独不能写。**
- 空 map（`map[string]int{}` 或 `make(map[string]int)`）读写都正常，它只是"长度为 0"，不是 nil。

因为"读不存在的 key"和"读 nil map"都返回零值，想判断 key 到底存不存在只能用 comma-ok（见下文"comma-ok"），不能靠返回值推断。

两者的语义差别在于意图：`nil` map 表示"还没有可用容器"，空 map 表示"容器就绪，只是暂时没有内容"。如果某个函数可能返回一个尚未初始化的 map，要么由提供方保证返回可写的 map，要么调用方先判 nil，这是 Go 里很常见的一类 bug 来源。

## 增删改查

写入和修改：

```go
m := map[string]int{}

m["a"] = 1
m["b"] = 2
m["a"] = 3

fmt.Println(m) // map[a:3 b:2]
```

读取：

```go
m := map[string]int{"a": 3, "b": 2}

v := m["a"]
fmt.Println(v) // 3
```

删除：

```go
m := map[string]int{"a": 3, "b": 2}

delete(m, "b")
fmt.Println(m) // map[a:3]
```

获取长度：

```go
m := map[string]int{"a": 3, "b": 2}
delete(m, "b")

fmt.Println(len(m)) // 1
```

## comma-ok

直接读取不存在的 key 会返回值类型的零值：

```go
m := map[string]int{"a": 1}

fmt.Println(m["missing"]) // 0
```

如果值类型的零值也有意义，就需要用 comma-ok 判断 key 是否存在：

```go
m := map[string]int{"a": 1}

v, ok := m["missing"]

fmt.Println(v)  // 0
fmt.Println(ok) // false
```

存在时 `ok` 为 `true`：

```go
m := map[string]int{"a": 1}

v, ok := m["a"]

fmt.Println(v)  // 1
fmt.Println(ok) // true
```

## 遍历

使用 `range` 遍历 map：

```go
m := map[string]int{
    "a": 1,
    "b": 2,
    "c": 3,
}

for k, v := range m {
    fmt.Println(k, v) // 顺序随机，不要依赖先打印哪一个
}
```

map 的遍历顺序是随机的，不能依赖某一种顺序。需要稳定顺序时，先把 key 取出来排序：

```go
keys := make([]string, 0, len(m))

for k := range m {
    keys = append(keys, k)
}

slices.Sort(keys) // 排序函数见 `15-slices.md`

for _, k := range keys {
    fmt.Println(k, m[k]) // a 1 / b 2 / c 3 —— 排序之后顺序才是确定的
}
```

注意别把上面 `fmt.Println(m) // map[a:3 b:2]` 那种输出当成反例：`fmt` 打印 map 时会**先把 key 排序**，所以打印结果稳定；`range` 的顺序仍然是随机的，两者不是一回事。

遍历期间增删元素的行为是明确定义的一半：**删除还没访问到的 key 是安全的**（它不会再被产出），所以 `for k := range m { delete(m, k) }` 是合法的就地清空写法；但遍历期间**新增**的 key 可能被访问到、也可能被跳过，不要依赖这一点。

## map 的值不可取地址

map 中的值不是可寻址的，`m["a"].Name = "Jerry"` 这种写法编译不过。改 map 里的值只有两条路——先取出值、改好再写回，或者让 map 直接保存指针；两条路的示例和取不到地址的原因见 `10-指针.md` 的“什么能取地址”一节。

## 删除与清空

删除单个 key：

```go
m := map[string]int{"a": 1, "b": 2}

delete(m, "a")
fmt.Println(m) // map[b:2]
```

清空所有 key 可以使用 `clear`：

```go
m := map[string]int{"a": 1, "b": 2}

clear(m)
fmt.Println(len(m)) // 0
```

`clear` 不会把 map 变成 `nil`，它只是删除所有元素。

## key 的要求

map 的 key 必须是可比较类型，例如：

- 布尔值
- 数字
- 字符串
- 指针
- channel
- 接口（但接口持有的动态值也必须可比较，否则运行时会 panic）
- 由可比较类型组成的数组和结构体

切片、map 和函数不能作为 map 的 key：

```go
// map[[]int]string{} // 非法
```

接口类型本身可以作为 key，但动态值不可比较时会 panic：

```go
m := map[any]int{}
m[[]int{1}] = 1 // panic: hash of unhashable type []int
```

浮点类型的 `NaN` 比较特殊：`NaN != NaN`，因此用 `NaN` 作为 key 时，写入后通常无法再通过相同值取回，`delete(m, math.NaN())` 同样删不掉它（键比较用的还是 `==`）——唯一可靠的做法是重建 map，或者换一个能自反比较的 key。标准库 `maps` 也不对这类非自反 key 做特殊处理。

## map 是引用语义

map 变量本身是一个描述符，赋值或传参时复制的是这个描述符，底层数据仍然共享。

```go
func add(m map[string]int) {
    m["a"] = 1
}

m := map[string]int{}
add(m)
fmt.Println(m) // map[a:1]
```

函数内部对 map 元素的修改会影响调用方。

## 并发安全

map 不是并发安全的。多个 goroutine（"同时有多个执行流"，并发本身在 `39-并发基础.md` 讲）同时读写同一个 map 会触发 `fatal error: concurrent map writes`（读写并发时是 `concurrent map read and map write`）。

注意这是**致命错误，不是普通 panic**：它由运行时直接终止整个进程，`defer` 和 `recover` 都拦不住。所以 map 的并发保护必须在编码阶段就设计好，不能指望用 `recover` 兜底。

需要并发读写时，用 `sync.RWMutex`（读多写少）或 `sync.Mutex` 把**每一次**访问都包起来，写法见 `39-并发基础.md`。另一条路是换用 `sync.Map`，但它只对两种访问模式有优势（键只写一次、之后反复读；或各 goroutine 读写互不相交的键集合），普通场景用 `map` 加锁更简单也更快，取舍见 `41-同步原语与内存模型.md`。

## 标准库 maps

改 map 之前先看一眼 `maps` 包有没有现成的：它提供了 `Clone`、`Copy`、`Equal`、`DeleteFunc`、`Keys`、`Values` 等函数，具体见 `16-maps.md`。

下一篇进入 [`09-struct.md`](09-struct.md)。
