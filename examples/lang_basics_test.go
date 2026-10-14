// Package examples 存放 docs/ 学习文档里"应当能跑出所写结果"的核心示例。
//
// 每个示例都是一个 Example 函数，靠 `// Output:` 注释让 `go test` 自动校验
// 输出是否与文档一致。函数名统一用 Example_xxx 形式（下划线后缀），
// 这样不要求包里真的存在同名标识符。
//
// 用法：go test ./...
package examples

import (
	"fmt"
	"strconv"
	"unsafe"
)

// 01 零值：各种类型的零值
func Example_variablesZeroValues() {
	var (
		n     int
		f     float64
		s     string
		ok    bool
		p     *int
		list  []int
		table map[string]int
	)

	fmt.Printf("%v %v %q %v %v %v %v\n", n, f, s, ok, p, list, table)
	// Output: 0 0 "" false <nil> [] map[]
}

// 01 整数进制字面量与下划线
func Example_integerLiterals() {
	fmt.Println(0b1010, 0o755, 0x1f, 1_000_000)
	// Output: 10 493 31 1000000
}

// 01 浮点字面量：无类型常量是精确值，所以直接打印是 0.3
func Example_untypedFloatConstantsAreExact() {
	fmt.Println(0.1 + 0.2) // 无类型常量，编译期精确计算

	a, b := 0.1, 0.2 // 运行时 float64
	fmt.Println(a + b)
	// Output:
	// 0.3
	// 0.30000000000000004
}

// 01 iota：省略表达式时复用上一行完整表达式
func Example_iotaReusesExpression() {
	const (
		// 用空白标识符把 iota=0 这行占掉，KB 才落在 iota=1 上，即 1<<10
		_  = iota
		KB = 1 << (10 * iota)
		MB
		GB
	)

	fmt.Println(KB, MB, GB)
	// Output: 1024 1048576 1073741824
}

// 01 rune 字面量
func Example_runeLiterals() {
	fmt.Println('A', '\x41', '\101', '\u4e2d')
	// Output: 65 65 65 20013
}

// 01 原始字符串与解释型字符串
func Example_stringLiteralKinds() {
	s1 := "a\nb"
	s2 := `a\nb`
	fmt.Println(len(s1), len(s2))
	// Output: 3 4
}

// 01 整数转字符串的两种含义
func Example_intToString() {
	n := 65

	// string(65) 会把 65 当成 Unicode 码点，结果是字母 A。
	// （go vet 的 stringintconv 要求这里显式写出 rune，所以写成 string(rune(n))。）
	fmt.Println(string(rune(n)))

	// 想把数字变成文本，要用 strconv.Itoa。
	fmt.Println(strconv.Itoa(n))
	// Output:
	// A
	// 65
}

// 01 %v 对浮点数走 %g：整数形式的浮点不显示小数点
func Example_floatFormatting() {
	g := float64(10)

	fmt.Println(g)
	fmt.Printf("%T %f\n", g, g)
	fmt.Println(3.0)
	fmt.Println(1e9)
	fmt.Println(1e-6)
	// Output:
	// 10
	// float64 10.000000
	// 3
	// 1e+09
	// 1e-06
}

// 01 %x 作用于字符串时输出的是每个字节
func Example_hexVerbOnString() {
	fmt.Printf("%x\n", "Go")
	// Output: 476f
}

// 02 整数除法与取余
func Example_integerDivision() {
	a, b := 7, 3

	fmt.Println(a+b, a-b, a*b, a/b, a%b)
	fmt.Println(5 / 2)
	fmt.Println(5.0 / 2)
	// Output:
	// 10 4 21 2 1
	// 2
	// 2.5
}

// 02 整数溢出按补码回绕
func Example_integerOverflow() {
	var x int8 = 127
	x++
	fmt.Println(x)
	// Output: -128
}

// 02 位运算
func Example_bitwiseOperators() {
	a, b := 6, 3 // 0110, 0011

	fmt.Println(a&b, a|b, a^b, a&^b)
	fmt.Println(1<<3, 8>>2)
	fmt.Println(^5)
	// Output:
	// 2 7 5 4
	// 8 2
	// -6
}

// 02 min/max 是内建函数，保留原类型
func Example_minMaxBuiltins() {
	fmt.Println(min(3, 1, 2), max(3, 1, 2))
	fmt.Printf("%T\n", min(3, 1, 2))
	// Output:
	// 1 3
	// int
}

// 06 数组是值类型：赋值会复制
func Example_arrayValueSemantics() {
	a := [3]int{1, 2, 3}
	b := a

	b[0] = 99
	fmt.Println(a, b)
	// Output: [1 2 3] [99 2 3]
}

// 06 切片转数组：判定依据是 len 而不是 cap
func Example_sliceToArray() {
	s := []int{1, 2, 3, 4}

	a := [3]int(s)    // 值转换：拷贝
	p := (*[3]int)(s) // 指针转换：共享底层数组

	p[0] = 99
	fmt.Println(a, s)

	a[1] = 77
	fmt.Println(a, s)
	// Output:
	// [1 2 3] [99 2 3 4]
	// [1 77 3] [99 2 3 4]
}

// 07 nil 切片可以直接 append
func Example_nilSliceAppend() {
	var s []int

	fmt.Println(len(s), cap(s), s == nil)
	s = append(s, 1)
	fmt.Println(s)
	// Output:
	// 0 0 true
	// [1]
}

// 07 append 的容量增长台阶
func Example_appendGrowth() {
	var s []int

	// 从 nil 开始 append 时，容量不够会先试一块 32 字节的定长栈缓冲（32/8 = 4 个 int），
	// 所以第一个台阶是 4 而不是 1；装不下才转交运行时按“容量不够就再翻倍”去找新数组。
	// 这个 32 是 cmd/compile 与运行时的实现细节（不是语言规范），换编译器/版本会变。
	// 容量够时 append 直接写在原数组上、cap 不变，所以这里只在容量变化的那一次打印。
	prev := cap(s)
	for i := 0; i < 40; i++ {
		s = append(s, i)
		if cap(s) != prev {
			fmt.Print(cap(s), " ")
			prev = cap(s)
		}
	}
	fmt.Println()
	// Output: 4 8 16 32 64
}

// 07 s = append(s, s...) 是安全的
func Example_appendSelf() {
	s := []int{1, 2, 3}
	s = append(s, s...)
	fmt.Println(s)
	// Output: [1 2 3 1 2 3]
}

// 07 append 会覆盖共享底层数组的其他切片
func Example_appendOverlapSharedArray() {
	a := []int{1, 2, 3, 4}
	b := a[:2] // 与 a 共享底层数组，len(b)=2、cap(b)=4

	// cap(b) 够放第三个元素，append 不扩容，直接写进 a[2]，所以 a 也被改掉了；
	// 如果容量不够（例如 b := a[3:]），append 会分配新数组，a 就不受影响。
	c := append(b, 99)
	fmt.Println(a, c)
	// Output: [1 2 99 4] [1 2 99]
}

// 07 copy 是 memmove 语义，重叠也正确
func Example_copyOverlap() {
	s := []int{1, 2, 3, 4, 5}
	copy(s[1:], s[:4])
	fmt.Println(s)

	t := []int{1, 2, 3, 4, 5}
	copy(t[:4], t[1:])
	fmt.Println(t)
	// Output:
	// [1 1 2 3 4]
	// [2 3 4 5 5]
}

// 07 用 append 删除中间元素：尾部会残留副本
func Example_deleteElementAppend() {
	s := []int{1, 2, 3, 4, 5}
	i := 2

	// append(s[:i], s[i+1:]...) 把 s[3:] 左移覆盖到 s[2:]，长度减一，底层数组没换；
	// 于是数组末尾多留了一个旧的 5（不会被清零），cap 仍然是 5。
	// 对比 slices.Delete：它会把尾部多出来的元素置零。
	s = append(s[:i], s[i+1:]...)
	fmt.Println(s, len(s), cap(s))
	// Output: [1 2 4 5] 4 5
}

// 07 完整切片表达式限制容量后，append 会分配新数组
func Example_fullSliceExpressionCapsCapacity() {
	s := []int{0, 1, 2, 3, 4}
	t := s[1:3:3]

	fmt.Println(len(t), cap(t))

	t = append(t, 99)
	fmt.Println(s, t)
	// Output:
	// 2 2
	// [0 1 2 3 4] [1 2 99]
}

// 07 nil 切片与空切片的区别只在编码时才暴露
func Example_nilVsEmptySlice() {
	var a []int
	b := []int{}

	fmt.Println(a == nil, b == nil, len(a), len(b))
	// Output: true false 0 0
}

// 07 clear 把元素置零但不改变长度
func Example_clearSlice() {
	s := []int{1, 2, 3}
	clear(s)

	fmt.Println(s, len(s))
	// Output: [0 0 0] 3
}

// 08 nil map 可以读、可以 range，但不能写
func Example_nilMapReadOnly() {
	var m map[string]int

	fmt.Println(m["missing"], len(m))

	for range m {
		fmt.Println("不会执行")
	}

	defer func() {
		fmt.Println("panic:", recover() != nil)
	}()
	m["a"] = 1
	// Output:
	// 0 0
	// panic: true
}

// 08 comma-ok 判断 key 是否存在
func Example_mapCommaOk() {
	m := map[string]int{"a": 1}

	v, ok := m["missing"]
	fmt.Println(v, ok)

	v, ok = m["a"]
	fmt.Println(v, ok)
	// Output:
	// 0 false
	// 1 true
}

// 08 delete 与 clear
func Example_mapDeleteAndClear() {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	delete(m, "a")
	fmt.Println(len(m))

	clear(m)
	fmt.Println(len(m), m == nil)
	// Output:
	// 2
	// 0 false
}

// 08 fmt 打印 map 时会先给 key 排序，所以打印结果稳定
func Example_mapPrintIsSorted() {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	fmt.Println(m)
	// Output: map[a:1 b:2 c:3]
}

// 10 指针的零值与解引用 panic
func Example_nilPointerDeref() {
	var p *int

	fmt.Println(p == nil)

	defer func() {
		fmt.Println("panic:", recover() != nil)
	}()
	fmt.Println(*p)
	// Output:
	// true
	// panic: true
}

// 10 new 分配零值并返回指针
func Example_newAllocatesZero() {
	p := new(int)

	fmt.Println(*p)
	*p = 10
	fmt.Println(*p)
	// Output:
	// 0
	// 10
}

// 10 unsafe.Sizeof 返回的是值本身的大小，不含它引用的内存
func Example_sizeofValueNotReferent() {
	var name string

	// 16：string 是（指针, 长度）两字长的头，与内容多长无关
	fmt.Println(unsafe.Sizeof(name))
	// 16：接口值同样是两个字（类型指针 + 数据指针），装 nil 也占 16 字节
	fmt.Println(unsafe.Sizeof(any(nil)))
	// 0：空结构体不占空间
	fmt.Println(unsafe.Sizeof(struct{}{}))
	// Output:
	// 16
	// 16
	// 0
}

// 35 unsafe.Sizeof 在结构体上会包含填充字节
func Example_structPadding() {
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

	// Bad 按声明顺序排：A 后面补 7 字节、C 后面再补 7 字节，共 24 字节；
	// Good 把 int64 放最前，字段本身占 10 字节、尾部补 6 字节，共 16 字节。
	// 多出来的都是对齐填充，unsafe.Sizeof 会把它算进去。
	fmt.Println(unsafe.Sizeof(Bad{}), unsafe.Sizeof(Good{}))
	// Output: 24 16
}
