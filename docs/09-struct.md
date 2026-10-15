# 09 struct

> Go 版本：>= 1.27.1  
> 相关标准库：reflect、unsafe

## 定义结构体

Go 没有类，也没有继承：**用结构体组合数据、用嵌入复用行为**（方法见 `11-方法.md`，嵌入接口见下文），这也是"结构体为什么值得单独一篇"的原因。

结构体把多个字段组合成一个类型：

```go
type User struct {
    Name string
    Age  int
}
```

字段名首字母大写表示导出，其他包可以访问；小写表示只在当前包内可见。

```go
type user struct {
    name string
    age  int
}
```

同一个包内可以访问未导出字段。

## 零值

结构体变量的零值是每个字段各自类型的零值（零值清单见 `01-变量与基础类型.md` 的“零值”一节）：

```go
type User struct {
    Name string
    Age  int
}

var u User

fmt.Println(u.Name) // 空字符串
fmt.Println(u.Age)  // 0
fmt.Println(u)      // { 0}
```

## 结构体字面量

使用键值方式初始化：

```go
u := User{
    Name: "Tom",
    Age:  18,
}
```

也可以按字段顺序初始化：

```go
u := User{"Tom", 18}
```

字段很多时，推荐使用键值方式，字段顺序变化时更不容易出错。键值方式和位置方式不能混用。

只初始化部分字段：

```go
type User struct {
    Name string
    Age  int
}

u := User{Name: "Tom"}

fmt.Println(u.Age) // 0
```

未指定的字段自动取得零值。

## 赋值与比较

### 复制

结构体是值类型，赋值或传参时会复制整个结构体：

```go
a := User{Name: "Tom", Age: 18}
b := a

b.Name = "Jerry"

fmt.Println(a.Name) // Tom
fmt.Println(b.Name) // Jerry
```

### 比较

如果结构体的所有字段都是可比较类型，那么结构体可以使用 `==` 和 `!=`：

```go
type User struct {
    Name string
    Age  int
}

a := User{Name: "Tom", Age: 18}
b := User{Name: "Tom", Age: 18}

fmt.Println(a == b) // true
```

包含切片、map 或函数字段的结构体不可比较：

```go
type Data struct {
    Values []int
}

// a == b // 非法
```

同样的条件也让结构体可以作为 map 的 key（map 对 key 的要求见 `08-map.md`）：

```go
type Key struct {
    Name string
    ID   int
}

m := map[Key]string{
    {Name: "a", ID: 1}: "one",
}

fmt.Println(m[Key{Name: "a", ID: 1}]) // one
```

字段里出现切片、map 或函数时，结构体也不能作为 map 的 key。

## 匿名结构体

没有名字的结构体可以直接使用：

```go
point := struct {
    X int
    Y int
}{
    X: 1,
    Y: 2,
}

fmt.Println(point) // {1 2}
```

匿名结构体适合临时的数据结构，例如测试数据、配置项或一次性 JSON 结构。

匿名结构体没有类型名，但两个匿名结构体只要字段序列、字段名、字段类型、标签以及是否嵌入全部相同，就属于同一个类型，可以直接比较和互相赋值：

```go
a := struct{ X, Y int }{1, 2}
b := struct{ X, Y int }{X: 1, Y: 2}

fmt.Println(a == b) // true

var c struct{ X, Y int } = b
fmt.Println(c) // {1 2}
```

只要字段名或标签不同，就是两个不同的类型，不能比较也不能赋值：

```go
// a == struct{ A, B int }{1, 2} // 非法：mismatched types
```

## 嵌套与嵌入

### 嵌套：命名字段

一个结构体可以作为另一个结构体的命名字段：

```go
type Address struct {
    City string
}

type User struct {
    Name    string
    Address Address
}
```

访问嵌套字段：

```go
u := User{
    Name:    "Tom",
    Address: Address{City: "Beijing"},
}

fmt.Println(u.Address.City) // Beijing
```

### 嵌入：不写字段名

嵌入是不写字段名的写法：

```go
type Person struct {
    Name string
}

type Employee struct {
    Person
    ID int
}
```

### 提升字段

嵌入后，外层结构体可以直接访问被嵌入类型的字段，这称为提升字段：

```go
type Person struct {
    Name string
}

type Employee struct {
    Person
    ID int
}

e := Employee{
    Person: Person{Name: "Tom"},
    ID:     1,
}

fmt.Println(e.Name) // Tom —— 提升字段，等于 e.Person.Name
fmt.Println(e.ID)   // 1

fmt.Println(e.Person.Name) // Tom —— 完整写法始终可用
```

### 提升字段字面量（Go 1.27）

Go 1.27 起，结构体**字面量的 key** 也可以是提升字段，不必先写嵌入字段再往里套一层。上面那个 `e` 等价于：

```go
e := Employee{
    Name: "Tom", // 直接给提升字段赋值
    ID:   1,
}

fmt.Println(e.Person.Name) // Tom
```

一条限制和一条冲突规则：

- 到被选字段路径上经过的嵌入字段**不能是指针类型**，`*Person` 这种嵌入无法通过提升字段直接初始化。

```go
type EmployeePtr struct {
    *Person
    ID int
}

// e := EmployeePtr{Name: "Tom", ID: 1}
// 编译错误：invalid implicit pointer indirection to reach Name
```

- 如果同时使用嵌入字段和它内部的提升字段，就会产生冲突——两者指向同一个字段：

```go
// e := Employee{
//     Person: Person{Name: "Tom"},
//     Name:   "Jerry",
// }
// 编译错误：cannot specify promoted field Name and enclosing embedded field Person
```

## 嵌入接口

结构体也可以嵌入接口类型——接口在这里只是"当作字段用"的一种类型，语法上和嵌入结构体没有区别：

```go
type Speaker interface {
    Speak() string
}

type Person struct {
    Speaker // 嵌入的字段名就是类型名 Speaker
    Age     int
}
```

这个字段同样可以用完整写法访问，并且**没赋值时它的值是 `nil`**：

```go
type Speaker interface {
    Speak() string
}

type Person struct {
    Speaker // 嵌入的字段名就是类型名 Speaker
    Age     int
}

var q Person

fmt.Println(q.Speaker == nil) // true
```

嵌入接口的常见用途是把接口当作字段，同时省掉一层手写的转发方法（让外层类型自动获得接口里的方法）。方法提升的规则、外层类型因此实现了哪些接口、以及 nil 接口字段调用方法会 panic 这些细节，都涉及方法和接口，分别见 `11-方法.md` 的"嵌入与方法提升"和 `12-接口.md`。

如果只是想组合出一个更大的接口，直接用接口嵌入即可，见 `12-接口.md` 的"接口嵌入"一节。

## 标签

结构体字段可以附加标签，常用于 JSON、数据库和反射：

```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}
```

标签本身不改变程序行为，由标准库或第三方库解析——`encoding/json` 读 `json` 键（见 `26-JSON与序列化.md`），`encoding/xml` 读 `xml` 键（见 `32-编码与序列化.md`）。

标签的格式按约定是一串可选的、以空格分隔的 `key:"value"` 对：

```go
type Config struct {
    Host string `json:"host" xml:"host,attr"`
}
```

- `key` 是非空字符串，由非控制字符组成，不能包含空格、双引号和冒号；
- `value` 用双引号括起来，内部遵循 Go 字符串字面量语法。

**格式不合约定时程序不会报错**：标签内容不合法不是编译错误，编译器完全不检查标签，只是 `reflect.StructTag.Get` 的返回值变成未定义的（当前实现返回空字符串）。这类错误只能靠工具发现：`go vet` 的 `structtag` 检查专门报告不符合 `reflect.StructTag.Get` 约定的字段标签——坏标签的例子与报错原文见 `00-工具链.md` 的“go vet：静态检查”一节。用 `reflect.StructTag` 的 `Get` / `Lookup` 在运行时读取标签（以及两者的区别）见 `36-reflect与unsafe.md` 的“读取结构体标签”一节。

## 空结构体

空结构体不占空间，常用作集合中的占位值：

```go
type Set map[string]struct{}

s := Set{}

s["a"] = struct{}{}
s["b"] = struct{}{}

if _, ok := s["a"]; ok {
    fmt.Println("exists")
}
```

## 内存布局

结构体的大小**不等于**各字段大小之和：每个字段的起始地址必须满足它自己的对齐要求，编译器会在字段之间和结构体末尾插入填充字节（padding），所以**字段顺序会直接改变结构体大小**。按"宽度从大到小"排列通常最省内存，但可读性更重要——只有结构体实例非常多、或者需要和外部二进制格式对齐时，才值得为布局专门调整字段顺序。

这一块的具体规则（填充位置、`unsafe.Sizeof`/`Offsetof`/`Alignof` 的实测字节数、`structs.HostLayout` 与 C ABI 布局）都要用 `unsafe` 才能观察，涉及指针（`10-指针.md`）和 `unsafe` 包，统一放在 `36-reflect与unsafe.md` 的“内存布局与 unsafe”一节。

## 结构体与指针

结构体通常通过指针修改原值——把 `&u` 传进函数，函数里改的是同一个对象；传值则是改副本，调用方看不到变化。完整的参数传递规则、`deposit` 与 `depositCopy` 的两种写法和对比见 `10-指针.md` 的"修改调用方的值"一节。

指针本身的语法、可寻址性与使用时机见 `10-指针.md`，方法接收者的选择见 `11-方法.md`。

下一篇进入 [`10-指针.md`](10-指针.md)。
