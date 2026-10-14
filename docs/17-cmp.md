# 17 cmp

> Go 版本：>= 1.27.1  
> 相关标准库：cmp、slices

`cmp` 包提供有序类型的比较函数和 `cmp.Ordered` 约束。它本身是 **Go 1.21** 随泛型和 `slices`/`maps` 一起引入的：Go 1.21 带来 `cmp.Compare`、`cmp.Less`、`cmp.Ordered`，**Go 1.22** 补上 `cmp.Or`（对照 GOROOT 的 `api/go1.21.txt`、`api/go1.22.txt`）。下面提到具体符号时会标出引入版本。

**为什么需要它**：排序和搜索要的是"小于、等于、大于"这个三态结果，而不是一个 `bool`——`slices.SortFunc`、`slices.BinarySearchFunc` 都按这个约定工作，自己手写 `<` 还得处理 NaN 这类特殊情况。按任务查：要三态用 `Compare`、只要布尔用 `Less`、给可能为零的值兜底用 `Or`。

## Ordered

`cmp.Ordered` 是一个类型约束，包含所有支持 `<`、`<=`、`>`、`>=` 的类型（整数、浮点、字符串，都带 `~`）。它的完整定义、`~` 的含义以及"什么时候该用它"见 `13-泛型与类型系统.md` 的"cmp.Ordered"一节。

## Compare

`cmp.Compare` 返回三个可能的值：

- `-1`：`x < y`
- `0`：`x == y`
- `1`：`x > y`

```go
fmt.Println(cmp.Compare(1, 2)) // -1
fmt.Println(cmp.Compare(2, 2)) // 0
fmt.Println(cmp.Compare(3, 2)) // 1
```

也适用于字符串：

```go
fmt.Println(cmp.Compare("a", "b")) // -1
fmt.Println(cmp.Compare("b", "a")) // 1
```

## Less

`cmp.Less` 返回 `x < y` 的结果：

```go
fmt.Println(cmp.Less(1, 2)) // true
fmt.Println(cmp.Less(2, 1)) // false
```

它适合直接用于判断，不需要关心返回的 `-1`、`0`、`1`。

## NaN 的处理

浮点数的 `NaN` 不满足任何比较运算，`NaN < 1`、`NaN > 1`、`NaN == NaN` 全是 `false`。`cmp` 的函数为了让排序可用，给 `NaN` 规定了一个**一致**的顺序：`go doc cmp.Compare` 写的是

> For floating-point types, a NaN is considered less than any non-NaN, a NaN is considered equal to a NaN, and -0.0 is equal to 0.0.

实测行为与文档一致：

```go
// NaN 是"不是数字"，要用 math.NaN 构造，自己写 0/0 常量会编译报错
nan := math.NaN()

// Compare 返回 -1 / 0 / 1，可以把结果直接当成排序所需的符号
fmt.Println(cmp.Compare(nan, 1))            // -1
fmt.Println(cmp.Compare(1, nan))            // 1
fmt.Println(cmp.Compare(nan, nan))          // 0
fmt.Println(cmp.Compare(nan, math.Inf(-1))) // -1，NaN 连 -Inf 都排在前面
fmt.Println(cmp.Compare(math.Copysign(0, -1), 0)) // 0，-0.0 等于 +0.0
```

注意 `-0.0` 这种写法在 Go 里是常量，等于 `+0.0`；要构造负零得用 `math.Copysign(0, -1)`。

`cmp.Less` 也遵循同一套规则，所以 **`cmp.Less(NaN, x)` 对任何非 NaN 的 `x` 都返回 `true`**，反过来 `cmp.Less(x, NaN)` 返回 `false`：

```go
fmt.Println(cmp.Less(nan, 1)) // true
fmt.Println(cmp.Less(1, nan)) // false
```

注意这和裸运算符不同：`nan < 1` 是 `false`，而 `cmp.Less(nan, 1)` 是 `true`。`slices.SortFunc` 里用 `cmp.Compare` 会得到“NaN 排在最前面”的确定结果；如果直接写 `if a < b { return -1 }`，NaN 会让比较函数失去一致性，排序结果就不可靠了。

## Or

`cmp.Or` 返回第一个非零值；如果所有值都是零值，就返回该类型的零值。注意负零（如 `math.Copysign(0, -1)`）也视为零值，最终得到的是 `+0`。

```go
fmt.Println(cmp.Or(0, 0, 3)) // 3
fmt.Println(cmp.Or("", "go")) // go
fmt.Println(cmp.Or(0, 0))    // 0
```

常见用途是提供默认值：

```go
name := cmp.Or(userName, defaultName)
```

如果 `userName` 不是空字符串，就返回 `userName`；否则返回 `defaultName`。

## 结构体、time.Time 等类型

`cmp.Compare` 的约束是 `cmp.Ordered`，只覆盖整数、浮点、字符串。结构体、`time.Time` 这些类型都不在其中，直接传进去是**编译错误**：

```go
// cmp.Compare(User{}, User{})
// 编译错误：User does not satisfy cmp.Ordered (User missing in ~int | ~int8 | ... | ~string)

// cmp.Compare(time.Now(), time.Now())
// 编译错误：time.Time does not satisfy cmp.Ordered (time.Time missing in ~int | ~int8 | ... | ~string)
```

这类类型要用它们自带的方法或自己写的比较函数：

```go
t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
t2 := t1.Add(time.Hour)

// time.Time 自带 Compare（返回 -1/0/1），也有 Before / After
fmt.Println(t1.Compare(t2)) // -1
fmt.Println(t1.Before(t2))  // true
fmt.Println(t1.After(t2))   // false
```

结构体则要自己写比较函数，把每个字段依次交给 `cmp.Compare`——见下文"自定义比较函数"。

如果自己实现 `Compare` 方法，返回值只需满足负/零/正的约定，不必一定是 `-1`/`0`/`1`。

## 配合 slices.SortFunc

`slices` 里接受比较函数的 `*Func` 系列要的都是"返回 `int` 的三路比较器"，`cmp.Compare` 正好符合，可以直接当函数值传进去：

```go
nums := []int{3, 1, 4, 2}

slices.SortFunc(nums, cmp.Compare)
fmt.Println(nums) // [1 2 3 4]

// 也可以写成显式的比较函数
slices.SortFunc(nums, func(a, b int) int {
    return cmp.Compare(a, b)
})
```

结构体字段的排序见下文"自定义比较函数"。

## 配合 slices.BinarySearchFunc

`cmp.Compare` 也可以用于自定义二分查找：

```go
type User struct {
    Name string
    Age  int
}

users := []User{
    {Name: "Jerry", Age: 20},
    {Name: "Spike", Age: 25},
    {Name: "Tom", Age: 30},
}

target := User{Age: 25}

index, found := slices.BinarySearchFunc(users, target, func(a, b User) int {
    return cmp.Compare(a.Age, b.Age)
})

fmt.Println(index, found) // 1 true
```

## 配合 slices 的其他函数

上面说过 `cmp.Compare` 可以直接当函数值传给 `*Func` 系列。这些函数的签名分两类：`SortFunc`、`SortStableFunc`、`IsSortedFunc`、`MinFunc`、`MaxFunc` 要 `func(a, b E) int`；`BinarySearchFunc` 要 `func(E, T) int`、`CompareFunc` 要 `func(E1, E2) int`——后两者的两个类型参数可以不同，`E1 == E2` 时 `cmp.Compare` 同样能直接用。

```go
s := []int{3, 1, 4, 2}

// MinFunc / MaxFunc：用自定义比较函数取最小 / 最大值
fmt.Println(slices.MinFunc(s, cmp.Compare)) // 1
fmt.Println(slices.MaxFunc(s, cmp.Compare)) // 4

// SortFunc 原地排序；IsSortedFunc 判断是否已按同一规则有序
// SortStableFunc / IsSortedFunc 同理；BinarySearchFunc 在 E == T 时也同理
slices.SortFunc(s, cmp.Compare)
fmt.Println(slices.IsSorted(s))                          // true
fmt.Println(slices.IsSortedFunc(s, cmp.Compare))         // true
// BinarySearchFunc：有序切片上的二分查找，返回下标和"是否找到"
fmt.Println(slices.BinarySearchFunc(s, 4, cmp.Compare))  // 3 true
```

注意 `cmp.Less` 不能这样用：它的签名是 `func(x, y T) bool`，和比较函数要求的 `func(a, b E) int` 不匹配。

```go
// slices.SortFunc(s, cmp.Less)
// 编译错误：type func[T cmp.Ordered](x T, y T) bool of cmp.Less does not match func(a E, b E) int
```

需要特别注意的两点：

- `slices.MinFunc` / `slices.MaxFunc` 在切片为空时 panic，`cmp.Compare` 本身无法帮你兜底。
- `SortFunc` / `SortStableFunc` 明确要求比较函数满足严格弱序；这里用 `cmp.Compare` 恰好满足，包括 `NaN` 的情况。自己手写比较逻辑时容易破坏这一点，见 `15-slices.md`。

## 自定义比较函数

`cmp.Compare` 只比较单个有序值。要按结构体的某个字段排序，就把该字段交给它：

```go
slices.SortFunc(users, func(a, b User) int {
    return cmp.Compare(a.Age, b.Age)
})
```

要"多字段排序"，就在比较函数里**依次比较**：前一个字段相等（返回 0）才继续比下一个。

```go
slices.SortFunc(users, func(a, b User) int {
    if c := cmp.Compare(a.Age, b.Age); c != 0 {
        return c
    }
    return cmp.Compare(a.Name, b.Name)
})
```

上面这段先按 `Age` 排序，年龄相同时再按 `Name` 排序。

比较函数的三路约定和严格弱序要求见 `15-slices.md` 的"排序与搜索"一节。这里只强调与 `cmp` 相关的两点：

- 用 `cmp.Compare` 拼出来的比较函数天然满足严格弱序（包括 `NaN` 的情形），手写 `if a < b { return -1 }` 则容易破坏一致性。
- 数值类型上**不要用减法**代替 `cmp.Compare`，减法会溢出，具体反例见 `15-slices.md`。

## 与 sort 包的关系

标准库还有 `sort` 包，它更早出现，主要面对不支持泛型的代码。新代码中，切片排序优先使用 `slices.Sort`、`slices.SortFunc`，比较函数优先使用 `cmp.Compare`。

下一篇进入 [`18-包与工程组织.md`](18-包与工程组织.md)。
