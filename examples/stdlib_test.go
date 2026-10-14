package examples

import (
	"archive/tar"
	"bytes"
	"cmp"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"text/template"
	"time"
	"unicode/utf8"
)

// ---------- 19 测试 ----------

// Div 是被 19 篇举例用的最小被测函数：除零返回 error（返回值与文档里 ExampleDiv 的期望一致）。
func Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

// 19 模糊测试：目标与不变量都抄自文档那一段，所以文档改了这里必须跟着改。
//
// 种子语料里特意放了 b == 0 和两个极端值，让 `go test`（不 fuzz）就能覆盖：
//   - b == 0：把"除零必须返回错误"这一支真的执行一遍（文档强调的分支顺序就是这个意思）
//   - math.MinInt / -1：验证不变量本身不会因为回绕而误报
//
// 注意不变量写成 `got*b == a-a%b`，不能写成"商 × 除数 + 余数 == 被除数"：
// MinInt / -1 的结果超出 int 范围（Go 规定回绕），加法验算会溢出。
func FuzzDiv(f *testing.F) {
	f.Add(6, 3)
	f.Add(0, 1)
	f.Add(5, 0)
	f.Add(math.MinInt, -1)

	f.Fuzz(func(t *testing.T, a, b int) {
		got, err := Div(a, b)

		if b == 0 {
			if err == nil {
				t.Fatalf("Div(%d, 0) 应返回错误，实际返回了 %d", a, got)
			}
			return
		}
		if err != nil {
			t.Fatalf("Div(%d, %d) 返回了意外错误：%v", a, b, err)
		}
		if got*b != a-a%b {
			t.Fatalf("Div(%d, %d) = %d，乘法验算不回去", a, b, got)
		}
	})
}

// ---------- 14 迭代器 ----------

func repeat(s string, n int) iter.Seq[string] {
	return func(yield func(string) bool) {
		for range n {
			if !yield(s) {
				return
			}
		}
	}
}

// 14 自定义迭代器与 range
func Example_iteratorCustom() {
	for s := range repeat("go", 3) {
		fmt.Print(s, " ")
	}
	fmt.Println()
	// Output: go go go
}

// 14 yield 返回 false 时迭代器应立即停止
func Example_iteratorStopsEarly() {
	count := 0
	for s := range repeat("x", 100) {
		count++
		fmt.Print(s)
		if count == 2 {
			break // break 会让 yield 返回 false，repeat 里的循环随即 return
		}
	}
	// 循环里用的是 Print（不换行），所以 count 的 2 紧接在 "xx" 之后，合起来是 xx2
	fmt.Println(count)
	// Output: xx2
}

// 14 iter.Pull 把推风格转成拉风格
func Example_iteratorPull() {
	next, stop := iter.Pull(repeat("go", 3))
	defer stop()

	var got []string
	for {
		s, ok := next()
		if !ok {
			break
		}
		got = append(got, s)
	}
	fmt.Println(got)
	// Output: [go go go]
}

// 14 把迭代器收成切片
func Example_iteratorCollect() {
	nums := slices.Collect(slices.Values([]int{1, 2, 3}))
	fmt.Println(nums)

	keys := slices.Collect(maps.Keys(map[string]int{"b": 2, "a": 1}))
	slices.Sort(keys)
	fmt.Println(keys)
	// Output:
	// [1 2 3]
	// [a b]
}

// 14 Chunk 分块
func Example_iteratorChunk() {
	for chunk := range slices.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		fmt.Println(chunk)
	}
	// Output:
	// [1 2]
	// [3 4]
	// [5]
}

// 14 大多数迭代器可以重复遍历（与 strings.SplitSeq 这类单次迭代器不同）
func Example_iteratorReusable() {
	seq := slices.Values([]int{1, 2, 3})

	var a, b []int
	for v := range seq {
		a = append(a, v)
	}
	for v := range seq {
		b = append(b, v)
	}
	fmt.Println(a, b)
	// Output: [1 2 3] [1 2 3]
}

// ---------- 15 slices ----------

// 15 slices 的查找与比较
func Example_slicesFindAndCompare() {
	s := []int{1, 2, 3, 4}

	fmt.Println(slices.Contains(s, 3), slices.Index(s, 4), slices.Index(s, 9))
	fmt.Println(slices.Equal([]int{1, 2}, []int{1, 2}))
	fmt.Println(slices.Compare([]int{1, 2}, []int{1, 3}))
	// Output:
	// true 3 -1
	// true
	// -1
}

// 15 Delete 会就地修改并置零尾部
func Example_slicesDeleteZeroesTail() {
	s := []int{1, 2, 3, 4, 5}
	other := s[:5] // 与 s 共享底层数组

	t := slices.Delete(s, 1, 3)
	fmt.Println(t)
	fmt.Println(other)
	// Output:
	// [1 4 5]
	// [1 4 5 0 0]
}

// 15 Insert 在容量足够时就地搬移
func Example_slicesInsertInPlace() {
	s := make([]int, 3, 10) // 预留容量，slices.Insert 才能在同一个底层数组里就地搬移
	copy(s, []int{1, 4, 5})

	s2 := slices.Insert(s, 1, 2, 3)
	// 容量够：Insert 原地把 [4 5] 后移再写入 2、3，返回更长的切片，cap 仍是 10，
	// 说明没有分配新数组——s2 与 s 其实指向同一块内存。
	fmt.Println(s2, cap(s2))
	// Output: [1 2 3 4 5] 10
}

// 15 Compact 只去掉相邻重复
func Example_slicesCompact() {
	s := slices.Compact([]int{1, 1, 2, 2, 2, 3, 1})
	fmt.Println(s)
	// Output: [1 2 3 1]
}

// 15 Clone 保留 nil
func Example_slicesCloneKeepsNil() {
	fmt.Println(slices.Clone([]int(nil)) == nil)
	fmt.Println(slices.Clone([]int{}) == nil)
	// Output:
	// true
	// false
}

// 15 排序、二分查找与极值
func Example_slicesSortAndSearch() {
	s := []int{3, 1, 2}
	slices.Sort(s)

	idx, found := slices.BinarySearch(s, 2)
	fmt.Println(s, idx, found)
	fmt.Println(slices.Min(s), slices.Max(s), slices.IsSorted(s))
	// Output:
	// [1 2 3] 1 true
	// 1 3 true
}

// 15 比较函数别用减法：会溢出
func Example_slicesCompareFuncOverflow() {
	s := []int{1 << 62, -(1 << 62)}
	slices.SortFunc(s, func(x, y int) int { return y - x })
	fmt.Println(s)

	t := []int{1 << 62, -(1 << 62)}
	slices.SortFunc(t, func(x, y int) int { return cmp.Compare(y, x) })
	fmt.Println(t)
	// Output:
	// [-4611686018427387904 4611686018427387904]
	// [4611686018427387904 -4611686018427387904]
}

// 15 MinFunc 在空切片上 panic
func Example_slicesMinFuncPanicsWhenEmpty() {
	defer func() {
		fmt.Println("panic:", recover() != nil)
	}()
	_ = slices.MinFunc([]int{}, cmp.Compare)
	// Output: panic: true
}

// ---------- 16 maps ----------

// 16 maps.Clone 与 maps.Copy
func Example_mapsCloneAndCopy() {
	src := map[string]int{"a": 1, "b": 2}
	clone := maps.Clone(src)
	clone["a"] = 99
	fmt.Println(src["a"], clone["a"])

	dst := map[string]int{"c": 3}
	maps.Copy(dst, src)
	fmt.Println(len(dst))
	// Output:
	// 1 99
	// 3
}

// 16 maps.Equal 与 DeleteFunc
func Example_mapsEqualAndDeleteFunc() {
	a := map[string]int{"a": 1, "b": 2}
	b := map[string]int{"a": 1, "b": 2}
	fmt.Println(maps.Equal(a, b))

	maps.DeleteFunc(b, func(k string, v int) bool { return v%2 == 0 })
	fmt.Println(b)
	// Output:
	// true
	// map[a:1]
}

// 16 maps.Keys 返回的是迭代器，不是切片
func Example_mapsKeysIsIterator() {
	m := map[string]int{"a": 1, "b": 2}
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)
	fmt.Println(keys)
	// Output: [a b]
}

// ---------- 17 cmp ----------

// 17 cmp.Compare / Less / Or
func Example_cmpCompareLessOr() {
	fmt.Println(cmp.Compare(1, 2), cmp.Compare(2, 2), cmp.Compare(3, 2))
	fmt.Println(cmp.Less(1, 2), cmp.Less(2, 1))
	fmt.Println(cmp.Or("", "go"), cmp.Or(0, 0, 3))
	// Output:
	// -1 0 1
	// true false
	// go 3
}

// 17 cmp 给 NaN 规定了一致的顺序
func Example_cmpNaNOrdering() {
	nan := math.NaN()

	fmt.Println(cmp.Compare(nan, 1))
	fmt.Println(cmp.Compare(nan, nan))
	fmt.Println(cmp.Less(nan, 1))
	fmt.Println(nan < 1) // 裸运算符是另一回事
	// Output:
	// -1
	// 0
	// true
	// false
}

// ---------- 20 字符串与字节 ----------

// 20 字符串是字节序列：len 是字节数，下标取到字节
func Example_stringIsBytes() {
	s := "Go语言"

	fmt.Println(len(s))
	fmt.Println(s[0])
	fmt.Printf("%T %c\n", rune(s[0]), rune(s[0]))
	// Output:
	// 8
	// 71
	// int32 G
}

// 20 按 rune 统计与遍历
func Example_stringRunes() {
	s := "Go语言"

	fmt.Println(utf8.RuneCountInString(s))

	var out []string
	for i, r := range s {
		if r > 127 { // i 是字节下标，不是第几个字符
			out = append(out, fmt.Sprintf("%d:%c", i, r))
		}
	}
	// 输出第二行的 2:语 5:言：G(0) o(1) 语(2..4) 言(5..7)，数字是字节下标
	fmt.Println(strings.Join(out, " "))
	fmt.Println(utf8.ValidString(s))
	// Output:
	// 4
	// 2:语 5:言
	// true
}

// 20 Cut 系列
func Example_stringsCut() {
	before, after, found := strings.Cut("key=value", "=")
	fmt.Println(before, after, found)

	rest, ok := strings.CutPrefix("go:lang", "go:")
	fmt.Println(rest, ok)

	head, tail, ok := strings.CutLast("a-b-c", "-")
	fmt.Println(head, tail, ok)
	// Output:
	// key value true
	// lang true
	// a-b c true
}

// 20 strings.Builder 与 + 的差别
func Example_stringsBuilder() {
	var b strings.Builder
	b.Grow(64) // 预分配缓冲区：之后的 WriteXxx 都往它上面追加，不产生中间字符串
	b.WriteString("go")
	b.WriteByte('-')
	b.WriteRune('语')
	// Len 是已写入的字节数而不是字符数："语" 占 3 字节，所以是 2+1+3 = 6
	fmt.Println(b.String(), b.Len())
	// Output: go-语 6
}

// 20 NewReplacer 一趟完成多处替换
func Example_stringsNewReplacer() {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;")
	fmt.Println(r.Replace("<a & b>"))
	// Output: &lt;a &amp; b&gt;
}

// 20 链式 ReplaceAll 会把前一步插入的 & 再转义一遍
func Example_stringsChainedReplaceAll() {
	s := strings.ReplaceAll("<a & b>", "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "&", "&amp;")
	fmt.Println(s)
	// Output: &amp;lt;a &amp; b&amp;gt;
}

// 20 SplitSeq 是单次迭代器
func Example_stringsSplitSeqSingleUse() {
	// SplitSeq 是单次迭代器：遍历状态留在闭包里，第二次 range 不会从头开始
	// （只剩最后一段，会再打印一个 "c"），也不会报错。
	for part := range strings.SplitSeq("a,b,c", ",") {
		fmt.Print(part, " ")
	}
	fmt.Println()
	// Output: a b c
}

// ---------- 23 时间 ----------

// 24 time.Duration 的运算与单位
func Example_timeDuration() {
	d := 2*time.Hour + 30*time.Minute

	fmt.Println(d)
	fmt.Println(d.Minutes())
	fmt.Println(time.Second)
	// Output:
	// 2h30m0s
	// 150
	// 1s
}

// 24 解析与格式化：layout 是参考时间本身
func Example_timeLayoutIsReferenceTime() {
	t, err := time.Parse("2006-01-02", "2026-08-28")
	fmt.Println(err)

	fmt.Println(t.Format(time.DateOnly))
	fmt.Println(t.Format("2006/01/02 15:04"))

	// 看起来像格式的写法不成立
	bad, badErr := time.Parse("YYYY-MM-DD", "2026-08-28")
	fmt.Println(bad.IsZero(), badErr != nil)
	// Output:
	// <nil>
	// 2026-08-28
	// 2026/08/28 00:00
	// true true
}

// 24 为什么不要用 == 比较 time.Time
func Example_timeDoNotUseEqualOperator() {
	a, _ := time.Parse(time.RFC3339, "2026-08-28T12:00:00+08:00")
	b, _ := time.Parse(time.RFC3339, "2026-08-28T04:00:00Z")

	fmt.Println(a == b)
	fmt.Println(a.Equal(b))
	fmt.Println(a.Compare(b))
	// Output:
	// false
	// true
	// 0
}

// 24 越界字段会被规范化；Truncate 是绝对偏移
func Example_timeNormalization() {
	t := time.Date(2026, 10, 32, 0, 0, 0, 0, time.UTC)
	// 10 月 32 日越界 → 规范化成 11 月 1 日
	fmt.Println(t.Format(time.DateOnly))

	d := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	// AddDate 先加月份再规范化：11 月没有 31 号 → 落到 12 月 1 日
	fmt.Println(d.AddDate(0, 1, 0).Format(time.DateOnly))

	// Truncate 按绝对时间点向下取整，与 timezone 无关
	fmt.Println(time.Date(2026, 8, 28, 13, 45, 30, 0, time.UTC).
		Truncate(time.Hour).Format(time.DateTime))
	// Output:
	// 2026-11-01
	// 2026-12-01
	// 2026-08-28 13:00:00
}

// 24 ParseDuration 只认到小时
func Example_timeParseDuration() {
	d, err := time.ParseDuration("1h30m")
	fmt.Println(d, d.Minutes(), err)

	_, err = time.ParseDuration("1d")
	fmt.Println(err != nil)
	// Output:
	// 1h30m0s 90 <nil>
	// true
}

// ---------- 24 数学与随机数 ----------

// 25 用固定种子的来源做可复现的随机
func Example_randReproducibleSource() {
	r := rand.New(rand.NewPCG(1, 2))
	fmt.Println(r.IntN(100), r.IntN(100))

	// 同一个种子得到同一串
	r2 := rand.New(rand.NewPCG(1, 2))
	fmt.Println(r2.IntN(100), r2.IntN(100))
	// Output:
	// 76 61
	// 76 61
}

// 25 r.N 是泛型版本，可指定任意整数类型
func Example_randGenericN() {
	r := rand.New(rand.NewPCG(1, 2))

	v := r.N(uint8(10))
	fmt.Printf("%v %T\n", v, v)
	// Output: 7 uint8
}

// 25 用固定来源做可复现的洗牌
func Example_randShuffleDeterministic() {
	r := rand.New(rand.NewPCG(7, 8))
	s := []int{1, 2, 3, 4, 5}
	r.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
	fmt.Println(s)
	// Output: [1 5 2 4 3]
}

// 25 math 的取整函数
func Example_mathRounding() {
	fmt.Println(math.Floor(3.7), math.Ceil(3.2), math.Round(3.5), math.Trunc(-3.7))
	fmt.Println(math.Mod(5.5, 2))
	// Output:
	// 3 4 4 -3
	// 1.5
}

// ---------- 25 JSON ----------

type jsonUser struct {
	Name    string        `json:"name"`
	Age     int           `json:"age,omitempty"`
	Address string        `json:"address,omitempty"`
	At      time.Time     `json:"at,omitzero"`
	D       time.Duration `json:"d,omitzero"`
}

// 26 Marshal 与 struct tag
func Example_jsonMarshal() {
	b, _ := json.Marshal(jsonUser{Name: "Tom", Age: 18})
	fmt.Println(string(b))
	// Output: {"name":"Tom","age":18}
}

// 26 omitempty 对 time.Time 无效，omitzero 才有效
func Example_jsonOmitzeroVsOmitempty() {
	type withEmpty struct {
		At time.Time `json:"at,omitempty"`
	}
	b, _ := json.Marshal(withEmpty{})
	fmt.Println(string(b))

	type withZero struct {
		At time.Time `json:"at,omitzero"`
	}
	b, _ = json.Marshal(withZero{})
	fmt.Println(string(b))
	// Output:
	// {"at":"0001-01-01T00:00:00Z"}
	// {}
}

// 26 nil 切片编码成 null，空切片编码成 []
func Example_jsonNilVsEmptySlice() {
	var a []int
	b := []int{}

	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	fmt.Println(string(x), string(y))
	// Output: null []
}

// 26 v1 默认转义 HTML 特殊字符
func Example_jsonHTMLEscaping() {
	b, _ := json.Marshal(map[string]string{"u": "a<b>&c"})
	fmt.Println(string(b))
	// Output: {"u":"a\u003cb\u003e\u0026c"}
}

// 26 Unmarshal 与 DisallowUnknownFields
func Example_jsonUnmarshal() {
	var u jsonUser
	err := json.Unmarshal([]byte(`{"name":"Tom","age":18,"extra":1}`), &u)
	fmt.Println(u.Name, u.Age, err)

	dec := json.NewDecoder(strings.NewReader(`{"name":"Tom","extra":1}`))
	dec.DisallowUnknownFields()
	err = dec.Decode(&u)
	fmt.Println(err != nil)
	// Output:
	// Tom 18 <nil>
	// true
}

// 26 Duration 在 v1 里是纳秒数
func Example_jsonDurationAsNanos() {
	type job struct {
		Timeout time.Duration `json:"timeout"`
	}
	b, _ := json.Marshal(job{Timeout: 90 * time.Second})
	fmt.Println(string(b))
	// Output: {"timeout":90000000000}
}

// ---------- 29 网络与 HTTP ----------

// 30 Go 1.22 起 ServeMux 支持方法匹配与路径通配符
func Example_serveMuxEnhancedRouting() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "id=%s", r.PathValue("id"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	call := func(method, path string) string {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return fmt.Sprintf("%d %s", w.Code, strings.TrimSpace(w.Body.String()))
	}

	fmt.Println(call("GET", "/users/42"))
	fmt.Println(call("POST", "/users/42"))
	fmt.Println(call("GET", "/health"))
	// Output:
	// 200 id=42
	// 405 Method Not Allowed
	// 200 ok
}

// 30 netip 是不可变值类型，可以直接比较、当 map key
func Example_netipAddr() {
	a := netip.MustParseAddr("192.0.2.1")

	fmt.Println(a.Is4(), a.IsPrivate(), a.Next())
	fmt.Println(a == netip.MustParseAddr("192.0.2.1"))

	p := netip.MustParsePrefix("192.0.2.0/24")
	fmt.Println(p.Contains(a), p.Masked())

	ap, _ := netip.ParseAddrPort("[2001:db8::1]:8080")
	fmt.Println(ap.Addr(), ap.Port())
	// Output:
	// true false 192.0.2.2
	// true
	// true 192.0.2.0/24
	// 2001:db8::1 8080
}

// ---------- 28 命令行与正则 ----------

// 29 正则的 ASCII 语义与 \p{...}
func Example_regexpASCIIvsUnicode() {
	re := regexp.MustCompile(`^\d+$`)
	fmt.Println(re.MatchString("٣"))

	re = regexp.MustCompile(`^\p{Nd}+$`)
	fmt.Println(re.MatchString("٣"))
	// Output:
	// false
	// true
}

// 29 替换串里的 $1 会被展开，用户输入要用 Literal
func Example_regexpReplaceLiteral() {
	re := regexp.MustCompile(`(\w+)=(\w+)`)
	userRepl := "$1"

	fmt.Println(re.ReplaceAllString("name=tom", userRepl))
	fmt.Println(re.ReplaceAllLiteralString("name=tom", userRepl))
	// Output:
	// name
	// $1
}

// 29 QuoteMeta 把用户输入拼进正则
func Example_regexpQuoteMeta() {
	user := "a.b(c)"
	re := regexp.MustCompile(regexp.QuoteMeta(user))

	fmt.Println(regexp.QuoteMeta(user))
	fmt.Println(re.MatchString("xa.b(c)y"))
	// Output:
	// a\.b\(c\)
	// true
}

// ---------- 35 container/heap ----------

type Task struct {
	Name     string
	Priority int
}

type TaskQueue []Task

func (q TaskQueue) Len() int           { return len(q) }
func (q TaskQueue) Less(i, j int) bool { return q[i].Priority < q[j].Priority }
func (q TaskQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }

func (q *TaskQueue) Push(x any) { *q = append(*q, x.(Task)) }

func (q *TaskQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

// 36 优先队列：Init 整理切片、Push 上浮、Pop 取出当前最小；类型与方法抄自文档那一段
func Example_heapPriorityQueue() {
	q := TaskQueue{
		{Name: "write", Priority: 2},
		{Name: "deploy", Priority: 3},
		{Name: "fix", Priority: 1},
	}

	heap.Init(&q)

	heap.Push(&q, Task{Name: "review", Priority: 4})

	first := heap.Pop(&q).(Task)
	second := heap.Pop(&q).(Task)

	fmt.Println(first.Priority, first.Name)
	fmt.Println(second.Priority, second.Name)
	// Output:
	// 1 fix
	// 2 write
}

// 36 缺键默认静默、missingkey=error 才报错：把这条语义钉住，免得以后有人以为模板会替你把关。
// 错误文案里带模板名与位置（template: row:1:10: executing "row" at <.Missing>: …），
// 只断言"必须报错 + 提到缺的键"这两件不随版本漂移的事。
func TestTemplateMissingKey(t *testing.T) {
	run := func(t *testing.T, tmpl *template.Template, data any) (string, error) {
		t.Helper()
		var b strings.Builder
		err := tmpl.Execute(&b, data)
		return b.String(), err
	}

	type Row map[string]string

	def := template.Must(template.New("row").Parse("[{{.Missing}}]"))
	got, err := run(t, def, Row{})
	if err != nil {
		t.Fatalf("默认行为不该报错: %v", err)
	}
	if got != "[<no value>]" {
		t.Fatalf("默认渲染 = %q，期望 [<no value>]", got)
	}

	strict := template.Must(template.New("row").Option("missingkey=error").Parse("[{{.Missing}}]"))
	_, err = run(t, strict, Row{})
	if err == nil {
		t.Fatal("missingkey=error 应当报错")
	}
	if !strings.Contains(err.Error(), "Missing") {
		t.Fatalf("错误里应当提到缺的键，实际: %v", err)
	}
}

// ---------- 31 archive/tar ----------

// 32 tar 的目录条目靠 Typeflag 表达：Name 尾斜杠会被提升成 TypeDir，
// 显式写 TypeDir 则不必带斜杠；解压侧只有 Typeflag 能区分目录与空文件。
func Example_tarDirectoryTypeflag() {
	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	if err := tw.WriteHeader(&tar.Header{Name: "dir", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		panic(err)
	}

	body := []byte("hello")
	if err := tw.WriteHeader(&tar.Header{Name: "dir/file.txt", Mode: 0o644, Size: int64(len(body))}); err != nil {
		panic(err)
	}
	if _, err := tw.Write(body); err != nil {
		panic(err)
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}

	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Println(hdr.Name, hdr.Typeflag == tar.TypeDir)
	}
	// Output:
	// dir true
	// dir/file.txt false
}

// ---------- 37 并发基础 ----------

// 38 无缓冲 channel 的发送接收
func Example_channelUnbuffered() {
	ch := make(chan int)

	// 无缓冲 channel 的发送会一直阻塞到有人接收，所以在同一个 goroutine 里“先发后收”会死锁；
	// 放进 goroutine 发送、主 goroutine 接收，就完成了一次交接。
	go func() { ch <- 10 }()

	fmt.Println(<-ch)
	// Output: 10
}

// 38 关闭 channel 后用 range 读完剩余数据
func Example_channelCloseAndRange() {
	ch := make(chan int)

	go func() {
		defer close(ch)
		for i := 1; i <= 3; i++ {
			ch <- i
		}
	}()

	var got []int
	for v := range ch {
		got = append(got, v)
	}
	fmt.Println(got)
	// Output: [1 2 3]
}

// 38 closed channel 读取立刻返回零值，ok 为 false
func Example_channelClosedReads() {
	ch := make(chan int, 1)
	ch <- 1
	close(ch)

	v, ok := <-ch
	fmt.Println(v, ok)

	v, ok = <-ch
	fmt.Println(v, ok)
	// Output:
	// 1 true
	// 0 false
}

// 38 select 的 default 让它不阻塞
func Example_selectDefault() {
	ch := make(chan int)

	select {
	case v := <-ch:
		fmt.Println("收到", v)
	default:
		fmt.Println("没数据")
	}
	// Output: 没数据
}

// 38 nil channel 上的收发永久阻塞，可用来关掉一个 case
func Example_selectNilChannelDisablesCase() {
	chA := make(chan int)
	chB := make(chan int)

	go func() { chA <- 1; close(chA) }()
	go func() { chB <- 2; close(chB) }()

	var gotA, gotB []int
	// 两个 channel 各发一个值后被关闭；把“已关闭”记成 nil，就能让对应 case 永久失效
	// （nil channel 上的收发永远阻塞，select 不会选中它），循环条件随之结束。
	// 不置 nil 的话，已关闭的 channel 会一直被选中、反复读到零值。
	for chA != nil || chB != nil {
		select {
		case v, ok := <-chA:
			if !ok {
				chA = nil // nil 之后这个 case 再也不会被选中
				continue
			}
			gotA = append(gotA, v)
		case v, ok := <-chB:
			if !ok {
				chB = nil
				continue
			}
			gotB = append(gotB, v)
		}
	}
	fmt.Println(gotA, gotB)
	// Output: [1] [2]
}

// 38 WaitGroup 等待所有 goroutine
func Example_waitGroup() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0

	for i := 1; i <= 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			total++
			mu.Unlock()
		}()
	}
	wg.Wait()
	fmt.Println(total)
	// Output: 100
}

// ---------- 38 context ----------

// 39 context 取消
func Example_contextCancel() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()

	cancel()
	<-done
	fmt.Println(ctx.Err())
	// Output: context canceled
}

// 39 context 超时
func Example_contextTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	<-ctx.Done()
	fmt.Println(errors.Is(ctx.Err(), context.DeadlineExceeded))
	// Output: true
}

// ---------- 39 同步原语与内存模型 ----------

// 40 sync.Once 只执行一次
func Example_syncOnce() {
	var once sync.Once
	n := 0

	for i := 0; i < 3; i++ {
		once.Do(func() { n++ })
	}
	fmt.Println(n)
	// Output: 1
}

// 40 sync.OnceValue 把结果也缓存下来
func Example_syncOnceValue() {
	loads := 0
	load := sync.OnceValue(func() string {
		loads++
		return "config"
	})

	fmt.Println(load(), load(), loads)
	// Output: config config 1
}

// 40 atomic 计数器
func Example_atomicCounter() {
	var count atomic.Int64
	count.Add(1)
	count.Add(2)

	fmt.Println(count.Load())
	fmt.Println(count.CompareAndSwap(3, 10), count.Load())
	// Output:
	// 3
	// true 10
}

// 40 atomic.Bool 与 atomic.Pointer[T]
func Example_atomicTypedValues() {
	var ready atomic.Bool
	fmt.Println(ready.Load())
	ready.Store(true)
	fmt.Println(ready.Swap(false), ready.Load())

	var cfg atomic.Pointer[user]
	cfg.Store(&user{Name: "Tom"})
	fmt.Println(cfg.Load().Name)
	// Output:
	// false
	// true false
	// Tom
}

// 40 原子操作之间的 happens-before：不用锁也能安全传递值
func Example_atomicHappensBefore() {
	var data atomic.Pointer[string]
	var done atomic.Bool

	// 先发布数据，再发布“已就绪”标志——顺序不能反
	go func() {
		s := "ready"
		data.Store(&s)
		done.Store(true)
	}()

	// 自旋等 done 变 true：原子读到的 true 与上面的原子写建立 happens-before，
	// 所以 *data.Load() 一定读得到 data.Store(&s) 写入的值，不需要额外的锁。
	// 若先 store done，自旋可能提前结束，data 还是 nil，解引用就 panic。
	for !done.Load() {
	}
	fmt.Println(*data.Load())
	// Output: ready
}
