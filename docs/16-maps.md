# 16 maps

> Go 版本：>= 1.27.1  
> 相关标准库：maps、slices、strings、runtime

`maps` 包为 map 提供了一组泛型工具函数（类型参数是泛型语法，见 `13-泛型与类型系统.md` 的“类型参数”一节）。和语言内建的 map 操作一样，它不处理 `NaN` 这类非自反 key 的特殊情况（行为见 `08-map.md`）。

下面**按函数逐个讲**（一个函数一个小节，方便按名字查）；如果你是按任务找，这里是索引：复制用 `Clone`、整体覆盖用 `Copy`、比较用 `Equal`、按条件删用 `DeleteFunc`、取键与值用 `Keys`/`Values`/`All`、合并进另一个 map 用 `Insert`。为什么需要它而不是手写循环——因为手写容易漏掉"深浅拷贝"和"遍历时改动"这两个坑，后面各节会点到。

## Clone

`Clone` 返回一个浅拷贝：

```go
m := map[string]int{
    "a": 1,
    "b": 2,
}

n := maps.Clone(m)

n["a"] = 99

fmt.Println(m) // map[a:1 b:2]
fmt.Println(n) // map[a:99 b:2]
```

如果 `m` 是 `nil`，`maps.Clone` 也返回 `nil`。

## Copy

`Copy` 把源 map 中的所有键值对复制到目标 map：

```go
src := map[string]int{
    "a": 1,
    "b": 2,
}

dst := map[string]int{
    "c": 3,
}

maps.Copy(dst, src)

fmt.Println(dst) // map[a:1 b:2 c:3]
```

`Copy` 会覆盖目标中已有的同名 key。

## Equal

`Equal` 比较两个 map 是否包含相同的键值对：

```go
a := map[string]int{"a": 1, "b": 2}
b := map[string]int{"a": 1, "b": 2}
c := map[string]int{"a": 1, "b": 3}

fmt.Println(maps.Equal(a, b)) // true
fmt.Println(maps.Equal(a, c)) // false
```

`EqualFunc` 使用自定义值比较函数：

```go
a := map[string]string{"name": "Go"}
b := map[string]string{"name": "GO"}

equalFold := func(x, y string) bool {
    return strings.EqualFold(x, y)
}

fmt.Println(maps.EqualFunc(a, b, equalFold)) // true
```

## DeleteFunc

`DeleteFunc` 按键值对条件删除元素：

```go
m := map[string]int{
    "a": 1,
    "b": 2,
    "c": 3,
}

maps.DeleteFunc(m, func(k string, v int) bool {
    return v%2 == 0
})

fmt.Println(m) // map[a:1 c:3]
```

## Keys、Values 和 All

### 返回的是迭代器（Go 1.23 起）

`maps.Keys` 返回 key 的迭代器：

```go
m := map[string]int{"a": 1, "b": 2}

for k := range maps.Keys(m) {
    fmt.Println(k) // 顺序不固定：map 的遍历顺序是随机的
}
```

它的签名是 `func Keys[Map ~map[K]V, K comparable, V any](m Map) iter.Seq[K]`，返回的是**迭代器**，不是切片。

这一点是常见的困惑来源：`maps.Keys` / `maps.Values` / `maps.All` 以及 `maps.Collect` / `maps.Insert` 都是 **Go 1.23** 才加入标准库的，加入时就是迭代器形态；Go 1.21 的标准库 `maps` 只有 `Clone`、`Copy`、`Equal`、`EqualFunc`、`DeleteFunc`。网上不少老教程和旧代码里写着 `keys := maps.Keys(m)` 然后直接当切片用（当年那套返回切片形态的 `Keys` 出自扩展库 `golang.org/x/exp/maps`，不随标准库发布），这样的代码在标准库下不能编译：

```go
keys := maps.Keys(m)
// keys[0] // 编译错误：cannot index keys (variable of func type iter.Seq[string])
```

### 拿切片要先 collect

要拿到切片，必须显式 collect：

```go
keys := slices.Collect(maps.Keys(m))
values := slices.Collect(maps.Values(m))
```

`maps.Values` 返回值的迭代器：

```go
for v := range maps.Values(m) {
    fmt.Println(v) // 顺序不固定：可能是 1 2，也可能是 2 1
}
```

`maps.All` 同时返回 key 和 value：

```go
for k, v := range maps.All(m) {
    fmt.Println(k, v) // 顺序不固定：a 1 和 b 2 谁先谁后每次运行都可能不同
}
```

需要稳定顺序时，可以收集 key 后排序：

```go
keys := slices.Collect(maps.Keys(m))
slices.Sort(keys)

for _, k := range keys {
    fmt.Println(k, m[k])
}
```

## Collect

`Collect` 把键值对迭代器收集成新的 map（Go 1.23+）：

```go
seq := maps.All(map[string]int{
    "a": 1,
    "b": 2,
})

m := maps.Collect(seq)
fmt.Println(m) // map[a:1 b:2]
```

## Insert

`Insert` 把迭代器中的键值对插入目标 map（Go 1.23+）：

```go
dst := map[string]int{}
seq := maps.All(map[string]int{
    "a": 1,
    "b": 2,
})

maps.Insert(dst, seq)
fmt.Println(dst) // map[a:1 b:2]
```

`Insert` 会覆盖目标 map 中已有的同名 key。

## 真删了，但内存还在

### 现象

`delete` 和 `clear` 只改变 map 的逻辑内容，**底层哈希表不会收缩**。map 一旦长到很大，删空之后仍然占着那块内存。

```go
m := make(map[int]int, 1<<20)
for i := 0; i < 1<<20; i++ {
    m[i] = i
}

for i := 0; i < 1<<20; i++ {
    delete(m, i)
}

fmt.Println(len(m)) // 0，但堆上那块表还在
```

### 自己测量

想自己看这个现象，用 `runtime.ReadMemStats` 前后对比堆占用（`runtime` 包本身在 `34-runtime与性能剖析.md` 讲；同一个进程里测量，看的是**相对变化**而不是绝对 MiB）：

```go
var ms runtime.MemStats

runtime.GC()
runtime.ReadMemStats(&ms)
base := ms.HeapAlloc

m := make(map[int]int, 1<<20)
for i := 0; i < 1<<20; i++ {
    m[i] = i
}

runtime.GC()
runtime.ReadMemStats(&ms)
afterInsert := ms.HeapAlloc

for i := 0; i < 1<<20; i++ {
    delete(m, i)
}

runtime.GC()
runtime.ReadMemStats(&ms)
afterDelete := ms.HeapAlloc

// 不变量：插入后堆占用明显上升；删空之后基本不回落（clear(m) 同理）
fmt.Println(base, afterInsert, afterDelete)
```

100 万条 `map[int]int` 在 64 位平台上是**几十 MiB 量级**（具体数字随实现、装载因子和 GC 时机变化）；关键是 `afterDelete` 和 `afterInsert` 几乎一样高。只有把 map 换成新对象、让旧对象失去引用并被 GC 回收，内存才会释放：

```go
m = nil      // 或者 m = make(map[int]int)
runtime.GC() // 之后堆回落到基线
```

这条规则在 Go 的运行时实现里是显式的：`clear` 对应的 `Map.Clear` 在清空目录后留着一句 `TODO: shrink directory?`，也就是“先不缩”。

实践建议：如果是长期存在、会周期性变大的缓存 map，定期换成新 map 比反复 `delete` 更省内存。

## 遍历期间修改 map

`range` 遍历 map 时删除元素是**安全**的：不会 panic，也不会干扰已经产出过的元素。

```go
m := map[int]int{1: 1, 2: 2, 3: 3, 4: 4}

for k := range m {
    if k%2 == 0 {
        delete(m, k)
    }
}

fmt.Println(m) // map[1:1 3:3]
```

如果被删掉的元素还没被遍历到，它就不会再被产出；已经产出过的元素不受影响。规范原文：

> If a map entry that has not yet been reached is removed during iteration, the corresponding iteration value will not be produced.

但在遍历过程中**新增**元素的行为是**未指定的**，可能被访问，也可能被跳过：

> If a map entry is created during iteration, that entry may be produced during the iteration or may be skipped. The choice may vary for each entry created and from one iteration to the next.

所以需要在遍历中插入新键值对时，先把新数据收集到另一个容器，遍历结束后再合并，不要依赖当次遍历能否看到它们。

`maps.DeleteFunc` 同理：它只是把 `delete` 包了一层，边遍历边删是安全的。

## 浅拷贝的含义

`Clone` 和 `Copy` 都是浅拷贝。如果 value 是切片、map 或指针，复制的是这些引用值，而不是底层数据。

```go
src := map[string][]int{
    "a": {1, 2},
}

dst := maps.Clone(src)

dst["a"][0] = 99

fmt.Println(src["a"]) // [99 2]
```

需要深拷贝时，需要自己复制 value 内部的数据。

下一篇进入 [`17-cmp.md`](17-cmp.md)。
