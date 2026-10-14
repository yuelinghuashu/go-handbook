package examples

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// ---------- 09 struct ----------

type user struct {
	Name string
	Age  int
}

// 09 结构体是值类型：赋值会复制
func Example_structCopy() {
	a := user{Name: "Tom", Age: 18}
	b := a

	b.Name = "Jerry"
	fmt.Println(a.Name, b.Name)
	// Output: Tom Jerry
}

// 09 所有字段可比较时结构体才能用 ==
func Example_structComparable() {
	a := user{Name: "Tom", Age: 18}
	b := user{Name: "Tom", Age: 18}

	fmt.Println(a == b)

	// 含切片/map/函数字段的结构体不可比较，下面这行会编译失败：
	//   withSlice{} == withSlice{} // invalid operation
	type withSlice struct{ V []int }
	_ = withSlice{}

	// Output: true
}

// 09 匿名结构体：字段序列与标签相同就属于同一个类型
func Example_anonymousStructSameType() {
	a := struct{ X, Y int }{1, 2}
	b := struct{ X, Y int }{X: 1, Y: 2}

	fmt.Println(a == b)

	var c struct{ X, Y int } = b
	fmt.Println(c)
	// Output:
	// true
	// {1 2}
}

// 09 嵌入 + 提升字段（含 Go 1.27 的提升字段字面量）
func Example_embeddedPromotedFields() {
	type Person struct{ Name string }
	type Employee struct {
		Person
		ID int
	}

	// Go 1.27 起可以直接用提升字段作为字面量 key
	e := Employee{Name: "Tom", ID: 1}

	fmt.Println(e.Name, e.ID, e.Person.Name)
	// Output: Tom 1 Tom
}

// 09 空结构体不占空间
func Example_emptyStructSize() {
	type Set map[string]struct{}

	s := Set{}
	s["a"] = struct{}{}

	_, ok := s["a"]
	fmt.Println(ok)
	// Output: true
}

// ---------- 10 指针 ----------

// 10 通过指针修改原变量
func Example_pointerMutation() {
	x := 10
	p := &x

	*p = 20
	fmt.Println(x)
	// Output: 20
}

// 10 指向结构体的指针可以直接访问字段
func Example_pointerToStruct() {
	u := user{Name: "Tom", Age: 18}
	p := &u

	p.Age = 19
	fmt.Println(u.Age)
	// Output: 19
}

// 10 函数接收数组指针才能修改原数组
func Example_arrayPointerParam() {
	change := func(a *[3]int) { a[0] = 99 }

	a := [3]int{1, 2, 3}
	change(&a)
	fmt.Println(a)
	// Output: [99 2 3]
}

// 10 map 元素不可寻址
func Example_mapElementNotAddressable() {
	m := map[string]int{"a": 1}

	// 下面这行编译不过：invalid operation: cannot take address of m["a"]
	// p := &m["a"]

	v := m["a"]
	v = 2
	m["a"] = v
	fmt.Println(m["a"])
	// Output: 2
}

// 10 什么能取地址：变量、字段、数组/切片元素、复合字面量
func Example_addressability() {
	x := 10
	p1 := &x

	type pt struct{ X, Y int }
	v := pt{X: 1, Y: 2}
	p2 := &v.X
	p3 := &v

	arr := [3]int{1, 2, 3}
	p4 := &arr[0]

	s := []int{1, 2, 3}
	p5 := &s[0]
	*p5 = 99 // 切片元素可寻址，改它影响切片

	p6 := &pt{X: 1} // 复合字面量是唯一"例外"，但合法

	_, _, _, _ = p1, p2, p3, p4
	fmt.Println(s, p6.X)
	// Output: [99 2 3] 1
}

// 10 修改 map 里的值：取出来写回，或让 map 存指针
func Example_modifyMapValueThroughPointer() {
	type pt struct{ X int }

	m := map[string]pt{"a": {X: 1}}
	v := m["a"] // 取出来
	v.X = 2
	m["a"] = v // 写回去
	fmt.Println(m["a"])

	mp := map[string]*pt{"a": {X: 1}}
	mp["a"].X = 3 // 存指针就能直接改
	fmt.Println(mp["a"].X)
	// Output:
	// {2}
	// 3
}

// 10 new(expr)：Go 1.26 起 new 也接受值表达式
func Example_newExpr() {
	p := new(42)
	fmt.Println(*p)
	fmt.Printf("%T\n", p)

	// 分配的是新变量，不是 x 的地址
	x := 7
	q := new(x)
	*q = 99
	fmt.Println(x, *q)
	// Output:
	// 42
	// *int
	// 7 99
}

// 10 传指针改调用方的值，传值只改副本
func Example_pointerModifiesCaller() {
	type u struct{ Age int }

	withPtr := func(v *u) { v.Age++ }
	byValue := func(v u) { v.Age++ }

	a := u{Age: 18}
	withPtr(&a)
	fmt.Println(a.Age)

	b := u{Age: 18}
	byValue(b)
	fmt.Println(b.Age)
	// Output:
	// 19
	// 18
}

// 10 改元素不需要指针，改切片描述符本身才需要
func Example_pointerToSliceHeader() {
	double := func(s []int) {
		for i := range s {
			s[i] *= 2
		}
	}
	addOne := func(s *[]int) { *s = append(*s, 4) }

	nums := []int{1, 2, 3}
	double(nums)
	fmt.Println(nums)

	addOne(&nums)
	fmt.Println(nums)
	// Output:
	// [2 4 6]
	// [2 4 6 4]
}

// 10 指针比较的是地址，不是指向的值
func Example_pointerComparison() {
	x, y := 10, 10
	px, py := &x, &y

	fmt.Println(px == py)
	fmt.Println(*px == *py)

	q := &x
	fmt.Println(px == q)
	fmt.Println(px == nil)
	// Output:
	// false
	// true
	// true
	// false
}

// 10 循环变量：Go 1.22 起每次迭代都是独立的变量
func Example_loopVarAddress() {
	var ptrs []*int
	// Go 1.22 起 range 每次迭代都新建循环变量 v，三个 &v 互不相同，输出才是 1 2 3；
	// 1.21 及以前共用同一个变量，同样的代码会打印 3 3 3。
	for _, v := range []int{1, 2, 3} {
		ptrs = append(ptrs, &v)
	}

	for _, p := range ptrs {
		fmt.Print(*p, " ")
	}
	fmt.Println()
	// Output: 1 2 3
}

// ---------- 11 方法 ----------

type counter int

func (c counter) Value() int { return int(c) }
func (c *counter) Inc()      { *c++ }

// 11 值接收者拿到的是副本，改不到原值
func Example_valueReceiverCopies() {
	var c counter = 1
	c.Inc() // 自动变成 (&c).Inc()，因为 c 可寻址
	fmt.Println(c.Value())

	d := c
	change := func(x counter) { x++ }
	change(d)
	fmt.Println(d)
	// Output:
	// 2
	// 2
}

// 11 方法值与方法表达式
func Example_methodValueAndExpression() {
	c := counter(5)

	mv := c.Value       // 方法值：绑定到 c
	me := counter.Value // 方法表达式：接收者变成第一个参数

	fmt.Println(mv(), me(c))
	// Output: 5 5
}

// 11 方法集决定谁满足接口
func Example_methodSet() {
	type incer interface{ Inc() }

	var c counter
	// var i incer = c   // 编译错误：counter 的方法集里没有 Inc
	var i incer = &c // *counter 有 Inc
	i.Inc()
	fmt.Println(c.Value())
	// Output: 1
}

// ---------- 12 接口 ----------

type speaker interface{ Speak() string }

type greeter struct{ Name string }

func (g greeter) Speak() string { return "hello, " + g.Name }

// 12 隐式实现 + 类型断言
func Example_interfaceAndTypeAssertion() {
	var s speaker = greeter{Name: "Tom"}

	fmt.Println(s.Speak())

	// comma-ok 形式不会 panic
	if g, ok := s.(greeter); ok {
		fmt.Println(g.Name)
	}

	// 断言到一个"不可能实现该接口"的类型是编译错误（int 没有 Speak），
	// 所以这里用 any 变量演示失败的情况
	var v any = s
	if _, ok := v.(int); !ok {
		fmt.Println("不是 int")
	}
	// Output:
	// hello, Tom
	// Tom
	// 不是 int
}

// 12 类型 switch
func Example_typeSwitch() {
	describe := func(v any) string {
		switch x := v.(type) {
		case string:
			return "string:" + x
		case int:
			return fmt.Sprintf("int:%d", x)
		case nil:
			return "nil"
		default:
			return "other"
		}
	}

	fmt.Println(describe("a"), describe(1), describe(nil), describe(1.5))
	// Output: string:a int:1 nil other
}

type pathErr struct{ Path string }

func (e *pathErr) Error() string { return "path error: " + e.Path }

// 12 nil 接口陷阱：装了 nil 指针的接口不等于 nil
func Example_nilInterfaceTrap() {
	type myErr struct{}

	// Error 必须定义在指针接收者上，*myErr 才实现 error
	// （方法要定义在包级类型上，这里用已有类型演示同样的语义）

	var p *pathErr
	var err error = p

	fmt.Println(err == nil)
	fmt.Println(err != nil && p == nil)
	// Output:
	// false
	// true
}

// 12 动态类型不可比较时，接口比较会 panic
func Example_interfaceComparisonPanics() {
	var a any = []int{1}
	var b any = []int{1}

	defer func() {
		fmt.Println("panic:", recover() != nil)
	}()
	// a、b 的动态类型都是 []int：接口比较要比动态值，而切片不可比较，运行到这行就 panic。
	// defer 里的 recover 把它接住，示例才不会真的崩掉。
	fmt.Println(a == b)
	// Output: panic: true
}

// 12 编译期接口断言
func Example_compileTimeInterfaceAssertion() {
	var _ speaker = greeter{} // 值接收者：值类型和指针都满足
	var _ speaker = (*greeter)(nil)
	fmt.Println("编译期已确认 greeter 实现 speaker")
	// Output: 编译期已确认 greeter 实现 speaker
}

// ---------- 13 泛型 ----------

// 泛型函数必须声明在包级别：Go 不允许带类型参数的函数字面量
// （写 `f := func[T any](v T) T {...}` 会报 "function type must have no type parameters"）。
func identity[T any](v T) T { return v }

type number interface {
	~int | ~int64 | ~float64
}

func add[T number](a, b T) T { return a + b }

func maxOf[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

type tildeInt interface{ ~int }

func double[T tildeInt](v T) T { return v * 2 }

func sumAll[T ~int | ~float64](xs []T) T {
	var total T
	for _, x := range xs {
		total += x
	}
	return total
}

type stack[T any] struct{ items []T }

func (s *stack[T]) push(v T) { s.items = append(s.items, v) }

func (s *stack[T]) pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	last := len(s.items) - 1
	v := s.items[last]
	// 取走末尾元素并把它从 items 里摘掉，调用方看到的是缩短后的栈
	s.items = s.items[:last]
	return v, true
}

// 13 类型参数与类型推断
func Example_genericsTypeInference() {
	fmt.Println(identity(1))
	fmt.Println(identity("hello"))
	fmt.Printf("%T\n", identity(1.5))
	// Output:
	// 1
	// hello
	// float64
}

// 13 联合约束：运算符必须对集合里每个类型都成立
func Example_genericsUnionConstraint() {
	fmt.Println(add(1, 2))
	fmt.Println(add(1.5, 2.5))
	// Output:
	// 3
	// 4
}

// 13 cmp.Ordered 约束
func Example_genericsCmpOrdered() {
	fmt.Println(maxOf(1, 2), maxOf("a", "b"))
	// Output: 2 b
}

// 13 ~T 让底层类型相同的自定义类型也满足约束
func Example_genericsTilde() {
	type MyInt int

	var n MyInt = 3
	fmt.Println(double(n))
	fmt.Printf("%T\n", double(n))
	// Output:
	// 6
	// examples.MyInt
}

// 13 泛型类型：必须先实例化才能使用
func Example_genericsType() {
	s := &stack[int]{}
	s.push(1)
	s.push(2)

	v, ok := s.pop()
	fmt.Println(v, ok, len(s.items))
	// Output: 2 true 1
}

// 13 无类型常量的默认类型决定推断结果
func Example_genericsUntypedConstantKind() {
	fmt.Printf("%T\n", maxOf(1, 2))
	fmt.Printf("%T %v\n", maxOf(1, 2.5), maxOf(1, 2.5))
	// Output:
	// int
	// float64 2.5
}

// 12/13 泛型与接口的取舍：约束只在编译期起作用，实例化后没有装箱
func Example_genericsVsInterface() {
	fmt.Println(sumAll([]int{1, 2, 3}))
	fmt.Println(sumAll([]float64{1.5, 2.5}))
	// Output:
	// 6
	// 4
}

// 13 标准库泛型：先查标准库再自己写
func Example_stdlibGenericBeforeCustom() {
	users := []user{{"Tom", 30}, {"Jerry", 20}}

	fmt.Println(slices.Contains(users, user{"Tom", 30}))
	fmt.Println(slices.IndexFunc(users, func(u user) bool { return u.Age < 25 }))

	slices.SortFunc(users, func(a, b user) int { return cmp.Compare(a.Age, b.Age) })
	fmt.Println(users)
	// Output:
	// true
	// 1
	// [{Jerry 20} {Tom 30}]
}

// 05 errors.Is 能穿透包装
func Example_errorsIsThroughWrapping() {
	base := errors.New("base")
	wrapped := fmt.Errorf("outer: %w", wrapMiddle(base))

	fmt.Println(errors.Is(wrapped, base))
	fmt.Println(wrapped)
	// Output:
	// true
	// outer: middle: base
}

func wrapMiddle(err error) error { return fmt.Errorf("middle: %w", err) }

// 05 errors.Join 与 errors.Unwrap 的差异
func Example_errorsJoinAndUnwrap() {
	a := errors.New("a")
	joined := errors.Join(a, errors.New("b"))

	// 多错误的 Unwrap() 返回 []error，errors.Unwrap 只认 Unwrap() error
	fmt.Println(errors.Unwrap(joined) == nil)
	fmt.Println(errors.Is(joined, a))
	// Output:
	// true
	// true
}
