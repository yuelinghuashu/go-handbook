# 15 slices

> Go 版本：>= 1.27.1  
> 相关标准库：slices、cmp、strings、bytes、iter

`slices` 包为切片提供了一组泛型工具函数。大多数函数不会隐式分配，除非函数本身需要创建新切片。

函数签名里的类型参数是泛型语法（见 `13-泛型与类型系统.md` 的“类型参数”一节）；调用时类型实参通常由编译器从切片元素推断出来，不用手写。

这个包是分三批长起来的，看文档时容易记混，先给一张对照表：

- **Go 1.21**（随标准库泛型化一起引入，泛型本身是 Go 1.18）：`Clone`、`Grow`、`Clip`、`Contains`、`Equal`、`Sort`、`SortFunc`、`BinarySearch` 等基础函数。
- **Go 1.22**：`Concat`。
- **Go 1.23**：`Repeat`、`Chunk`、`Collect`、`Sorted`、`SortedFunc`、`SortedStableFunc`、`AppendSeq`、`Values`、`All`、`Backward` 等。

下面各节里，属于较新批次的函数会在原处标注引入版本。

第三批里有一半是**迭代器函数**（`All`、`Values`、`Backward`、`Chunk`、`Collect`、`Sorted`、`AppendSeq`），它们涉及 `range` 遍历函数、`iter.Seq`、以及"迭代器能不能重复遍历"这套约定，统一放在 `14-迭代器.md` 讲；本篇只讲直接在切片上操作的函数。

## 查找

`Contains` 判断元素是否存在：

```go
s := []int{1, 2, 3}

fmt.Println(slices.Contains(s, 2)) // true
fmt.Println(slices.Contains(s, 4)) // false
```

`Index` 返回元素第一次出现的位置，不存在时返回 `-1`：

```go
fmt.Println(slices.Index(s, 2)) // 1
fmt.Println(slices.Index(s, 4)) // -1
```

`ContainsFunc` 和 `IndexFunc` 使用自定义条件：

```go
s := []int{1, 2, 3, 4}

isEven := func(v int) bool {
    return v%2 == 0
}

fmt.Println(slices.ContainsFunc(s, isEven)) // true
fmt.Println(slices.IndexFunc(s, isEven))    // 1
```

## 比较

`Equal` 比较两个切片是否长度相同、元素逐一相等：

```go
a := []int{1, 2, 3}
b := []int{1, 2, 3}
c := []int{1, 2}

fmt.Println(slices.Equal(a, b)) // true
fmt.Println(slices.Equal(a, c)) // false
```

`EqualFunc` 使用自定义相等函数：

```go
a := []string{"Go", "Lang"}
b := []string{"go", "lang"}

equalFold := func(x, y string) bool {
    return strings.EqualFold(x, y)
}

fmt.Println(slices.EqualFunc(a, b, equalFold)) // true
```

`Compare` 按字典序比较有序类型的切片：

```go
fmt.Println(slices.Compare([]int{1, 2}, []int{1, 3})) // -1
fmt.Println(slices.Compare([]int{1, 2}, []int{1, 2})) // 0
fmt.Println(slices.Compare([]int{1, 3}, []int{1, 2})) // 1
```

`CompareFunc` 使用自定义比较函数。

## 复制与拼接

`Clone` 返回一个浅拷贝：

```go
a := []int{1, 2, 3}
b := slices.Clone(a)

b[0] = 99

fmt.Println(a) // [1 2 3]
fmt.Println(b) // [99 2 3]
```

`Clone` 保留 nil：`slices.Clone([]int(nil))` 返回 `nil`，不是空切片。文档的措辞是 “The result preserves the nilness of s.”。

`Concat` 把多个切片拼接成新切片（Go 1.22 起）：

```go
a := []int{1, 2}
b := []int{3, 4}

c := slices.Concat(a, b)
fmt.Println(c) // [1 2 3 4]
```

`Repeat` 重复同一个切片若干次（Go 1.23 起）：

```go
s := slices.Repeat([]int{1, 2}, 3)
fmt.Println(s) // [1 2 1 2 1 2]
```

`Repeat` 的结果**永远不是 nil**：`slices.Repeat([]int(nil), 0)` 返回的是非 nil 的空切片（文档：“The result is never nil.”）。它要求 `count >= 0`，负数会 panic。

## 删除与插入

下面这些函数都是**就地修改**传入切片的底层数组，共用该数组的其他切片会一起看到改动，详见后面的“原地修改与共享底层数组”。

`Delete` 删除 `[i, j)` 区间内的元素，并返回修改后的切片：

```go
s := []int{1, 2, 3, 4, 5}
s = slices.Delete(s, 1, 3)

fmt.Println(s) // [1 4 5]
```

`DeleteFunc` 按条件删除元素：

```go
s := []int{1, 2, 3, 4, 5}
s = slices.DeleteFunc(s, func(v int) bool {
    return v%2 == 0
})

fmt.Println(s) // [1 3 5]
```

`Insert` 在指定位置插入元素：

```go
s := []int{1, 4, 5}
s = slices.Insert(s, 1, 2, 3)

fmt.Println(s) // [1 2 3 4 5]
```

`Replace` 替换一段区间：

```go
s := []int{1, 2, 3, 4}
s = slices.Replace(s, 1, 3, 8, 9)

fmt.Println(s) // [1 8 9 4]
```

## 去重与反转

`Compact` 去除相邻重复元素：

```go
s := []int{1, 1, 2, 2, 2, 3, 1}
s = slices.Compact(s)

fmt.Println(s) // [1 2 3 1]
```

注意它只处理相邻重复项。如果需要全局去重，通常先用 map 或先排序。

`CompactFunc` 使用自定义相等函数。

`Reverse` 原地反转切片：

```go
s := []int{1, 2, 3}
slices.Reverse(s)

fmt.Println(s) // [3 2 1]
```

## 原地修改与共享底层数组

`Delete`、`Insert`、`Replace`、`DeleteFunc`、`Compact`、`Reverse` 以及排序系列（`Sort`/`SortFunc`/`SortStableFunc`）都**就地修改传入切片的底层数组**；共享底层数组的完整规则见 `07-切片.md`。它们返回的切片通常仍然和原切片共用这段数组，所以任何其他指向同一数组的切片都会看到改动。

```go
s := []int{1, 2, 3, 4, 5}
other := s[:5] // 与 s 共享底层数组

t := slices.Delete(s, 1, 3)

fmt.Println(t)     // [1 4 5]
fmt.Println(other) // [1 4 5 0 0]，other 也被改了
fmt.Println(s[:5]) // [1 4 5 0 0]
```

`Delete` 不只是搬移元素，它还会**把腾空的尾部元素置零**。`go doc slices.Delete` 的原文是：

> Delete zeroes the elements s[len(s)-(j-i):len(s)].

`Replace` 在替换区间变短时同样会置零，`go doc slices.Replace` 的原文是：

> When len(v) < (j-i), Replace zeroes the elements between the new length and the original length.

```go
r := []int{1, 2, 3, 4, 5}
r2 := slices.Replace(r, 1, 4, 9)

fmt.Println(r2)  // [1 9 5]
fmt.Println(r)   // [1 9 5 0 0]，尾部被置零
```

`DeleteFunc` 与 `Compact` 的文档里也有同样的说明（“zeroes the elements between the new length and the original length”）。置零是为了让被删掉的元素不再被底层数组引用，从而能被 GC 回收。

`Insert` 在容量足够时同样就地搬移，不分配新数组：

```go
s := make([]int, 3, 10)
copy(s, []int{1, 4, 5})

s2 := slices.Insert(s, 1, 2, 3)

fmt.Println(s2, cap(s2)) // [1 2 3 4 5] 10
```

如果不想影响其他视图，先 `slices.Clone`，或者把结果写进一个新切片。

## 排序与搜索

`Sort` 原地排序有序类型：

```go
s := []int{3, 1, 2}
slices.Sort(s)

fmt.Println(s) // [1 2 3]
```

`SortFunc` 使用自定义比较函数（比较函数里常见的 `cmp.Compare` 来自 `cmp` 包，见 `17-cmp.md`）：

```go
type User struct {
    Name string
    Age  int
}

users := []User{
    {Name: "Tom", Age: 30},
    {Name: "Jerry", Age: 20},
}

slices.SortFunc(users, func(a, b User) int {
    return cmp.Compare(a.Age, b.Age)
})

fmt.Println(users) // [{Jerry 20} {Tom 30}] —— 按 Age 升序，Jerry 排到了前面
```

`SortStableFunc` 在比较结果相等时保留原顺序：

```go
same := []User{
    {Name: "Tom", Age: 30},
    {Name: "Jerry", Age: 30}, // 年龄相同
}

slices.SortStableFunc(same, func(a, b User) int {
    return cmp.Compare(a.Age, b.Age)
})

fmt.Println(same) // [{Tom 30} {Jerry 30}]，年龄相同时保留原始先后
```

比较函数必须满足**严格弱序**（strict weak ordering），这是 `go doc slices.SortFunc` 的明确要求：

> SortFunc requires that cmp is a strict weak ordering. ... The function should return 0 for incomparable items.

也就是说 `cmp` 要一致、可传递，并且对“无法比较”的元素返回 `0`。不满足时排序结果**未定义**——注意这里的“未定义”是字面意思：不仅“可能没排好”，连“有没有动过元素”都不确定，而且写出来的具体顺序会随实现版本变化。

```go
s := []int{3, 1, 2, 5, 4}

// 永远返回 1：不满足严格弱序
slices.SortFunc(s, func(a, b int) int { return 1 })

fmt.Println(s) // 结果未定义：不要依赖这里打印出的任何具体顺序
```

更隐蔽的是“看似合理”但不传递的比较函数：

```go
t := []int{3, 1, 2, 5, 4}

slices.SortFunc(t, func(a, b int) int {
    if a%2 != b%2 {
        if a%2 == 1 {
            return -1 // 奇数“小于”偶数
        }
        return 1
    }
    return 0 // 同奇偶时视为相等，破坏了传递性
})

fmt.Println(t) // 同样未定义：奇数在前只是“通常看起来对”，不能作为保证
```

> **这两个例子的重点是认识错误前提，不是观察输出。** 真正的结论是"比较函数必须满足严格弱序"：违反之后调用方拿到的切片处在**没有任何保证**的状态——不仅"可能没排好"，连"有没有被移动过元素"都不确定，写出来的具体顺序还会随实现版本变化。生产代码里出现这种比较函数，等于埋了一个随时会变行为的 bug。

`IsSorted` 判断切片是否已经有序：

```go
fmt.Println(slices.IsSorted([]int{1, 2, 3})) // true
fmt.Println(slices.IsSorted([]int{1, 3, 2})) // false
```

`IsSorted` 内部用 `cmp.Less` 逐对判断，所以也能用来检查上面那种“排了但没完全排”的结果（`slices.IsSorted(t)` 会返回 `false`）。更多关于比较函数的约定见 `17-cmp.md`。

### 比较函数别用减法

数值类型上很容易顺手写成 `b - a`（想要降序）或 `a - b`，但**减法会溢出**，溢出之后符号翻转，排序结果直接错掉——不报错、不 panic，只是排错：

```go
s := []int{1 << 62, -(1 << 62)}

// 想要降序，但 1<<62 - (-(1<<62)) 超出 int64，回绕成负数
slices.SortFunc(s, func(x, y int) int { return y - x })

fmt.Println(s) // [-4611686018427387904 4611686018427387904] —— 变成了升序，与意图相反
```

用 `cmp.Compare` 没有这个问题，它按 `<`/`>` 返回 `-1`/`0`/`1`，不做算术：

```go
t := []int{1 << 62, -(1 << 62)}

slices.SortFunc(t, func(x, y int) int { return cmp.Compare(y, x) })

fmt.Println(t) // [4611686018427387904 -4611686018427387904] —— 降序正确
```

（上面两行的期望输出是**十进制**：`1 << 62` 只是源码里的写法，`fmt.Println` 打印切片时不会保留表达式形式。）

所以**比较函数里出现减法就该警惕**，除非能保证差值一定落在类型范围内（例如元素都是 `int8` 这类小范围类型）。

`BinarySearch` 在有序切片中二分查找：

```go
s := []int{1, 3, 5, 7}

index, found := slices.BinarySearch(s, 5)
fmt.Println(index, found) // 2 true
```

`BinarySearchFunc` 使用自定义比较函数。

## 极值

> **`slices.Min`、`slices.Max`、`slices.MinFunc`、`slices.MaxFunc` 在切片为空时都会 panic。** `go doc` 的原文是 “Min returns the minimal value in x. It panics if x is empty.”，`MinFunc` / `MaxFunc` 写的是 “It panics if x is empty.”。调用前先用 `len(s) == 0` 兜底。

`Min` / `Max` 对浮点数还有一个额外约定：只要 `x` 里有 `NaN`，结果就是 `NaN`（“Min propagates NaNs”）。

```go
s := []int{3, 1, 4, 2}

fmt.Println(slices.Min(s)) // 1
fmt.Println(slices.Max(s)) // 4
```

`MinFunc` 和 `MaxFunc` 使用自定义比较函数，空切片同样会 panic，比较函数也应当满足严格弱序：

```go
s := []string{"go", "golang", "c"}

longest := slices.MaxFunc(s, func(a, b string) int {
    return cmp.Compare(len(a), len(b))
})

fmt.Println(longest) // golang
```

## 容量管理

`Grow` 增加切片的容量（什么时候需要手动管容量：已经知道最终要装多少元素、又不想让 `append` 反复扩容复制，就提前 `Grow`；反之不要为了"省一点"到处调它）：

```go
s := make([]int, 2, 2)
s = slices.Grow(s, 10)
```

`Grow` 不改变长度，只保证至少有足够的容量。

`Clip` 把切片的容量裁剪到长度：

```go
s := []int{1, 2, 3, 4}
s = s[:2]

s = slices.Clip(s)
fmt.Println(len(s), cap(s)) // 2 2
```

`Clip` 可以减少底层数组被长期保留的问题。

## nil 与空切片

`nil` 切片和空切片在 `slices` 里的处理并不一致（以下都实测过，结论与 `go doc` 一致）：

| 调用                                     | 结果            | 依据                                                            |
| ---------------------------------------- | --------------- | --------------------------------------------------------------- |
| `slices.Clone([]int(nil))`               | `nil`           | 文档：“The result preserves the nilness of s.”                  |
| `slices.Collect(空迭代器)`               | `nil`           | 文档：“If seq is empty, the result is nil.”                     |
| `slices.Sorted(空迭代器)`                | `nil`           | 同上                                                            |
| `slices.AppendSeq([]int(nil), 空迭代器)` | `nil`           | 文档：“If seq is empty, the result preserves the nilness of s.” |
| `slices.Repeat([]int(nil), 0)`           | 非 nil 空切片   | 文档：“The result is never nil.”                                |
| `slices.Grow([]int(nil), 4)`             | 非 nil（cap=4） | 需要空间时必须分配，见下方说明                                  |
| `slices.Clip([]int(nil))`                | `nil`           | 文档：“The result preserves the nilness of s.”                  |
| `slices.Delete([]int(nil), 0, 0)`        | `nil`           | 文档：“If the result is empty, it has the same nilness as s.”   |
| `slices.Compact([]int(nil))`             | `nil`           | 文档：“The result preserves the nilness of s.”                  |

表里统一写 `[]int(nil)` 而不是 `nil`：这些函数都是泛型函数，参数带 `~[]E` 约束，直接传无类型 `nil` 无法推断出类型参数，编译器会报 `cannot infer S`（标准库自己在这种情况下也写成 `Grow[S](nil, size)`）。

`Grow` 那一行和文档略有出入：文档写着 “The result preserves the nilness of s.”，但只要 `n > 0` 真的分配了空间，结果就不可能是 nil（实测 `slices.Grow([]int(nil), 4)` 的 `len` 是 0、`cap` 是 4）；`n == 0` 时才会原样返回 nil。

表里那几行涉及迭代器的调用（`Collect`、`Sorted`、`AppendSeq`）所说的"空迭代器"指的是一段不会产出任何元素的 `iter.Seq`。注意"没有元素"和"迭代器本身是 `nil`"是两回事，后者会直接 panic，判空规则见 `14-迭代器.md` 的"空迭代器与 nil 迭代器"一节。

下一篇进入 [`16-maps.md`](16-maps.md)。
