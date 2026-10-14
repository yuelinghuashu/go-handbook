package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 篇首版本标注的两种排版都要认：它带着 Markdown 硬换行的两个尾随空格，
// 正则必须容忍；`> 相关标准库：…` 那行不能被误当成版本。
func TestDocVersionRe(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"> Go 版本：>= 1.27.1  ", "1.27.1"},
		{"> Go 版本：>= 1.27.1", "1.27.1"},
		{"> Go 版本：>= 1.28", "1.28"},
		{"> 相关标准库：net、net/http", ""},
		{"Go 版本：>= 1.27.1", ""},
		{"> Go 版本：1.27.1", ""},
	}
	for _, tt := range tests {
		m := docVersionRe.FindStringSubmatch(tt.in)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != tt.want {
			t.Errorf("docVersionRe(%q) = %q，期望 %q", tt.in, got, tt.want)
		}
	}
}

// go.mod 写 patch 版（1.27.1）、文档写 `>= 1.27` 时不该互相打架：同一个 Go 版本即可。
// 但 1.27 与 1.28 必须算不一致，否则升级时漏改一半也检查不出来。
func TestSameRelease(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.27.1", "1.27.1", true},
		{"1.27", "1.27.1", true},
		{"1.27.1", "1.27", true},
		{"1.27", "1.28", false},
		{"1.27.1", "1.28.0", false},
		{"1.27", "1.270", false},
	}
	for _, tt := range tests {
		if got := sameRelease(tt.a, tt.b); got != tt.want {
			t.Errorf("sameRelease(%q, %q) = %v，期望 %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// moduleGoVersion 要能从文档目录往上找到仓库根的 go.mod——checkdocs 既会以
// `-dir docs` 从根运行，也会以 `-dir ../../docs` 从工具目录运行，不能假设固定深度。
func TestModuleGoVersionWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module x\n\ngo 1.30.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "docs", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := moduleGoVersion(nested); got != "1.30.2" {
		t.Errorf("moduleGoVersion(%q) = %q，期望 1.30.2", nested, got)
	}
	// 找不到 go.mod 时返回空串，让 checkVersion 跳过这一项而不是误报
	if got := moduleGoVersion(t.TempDir()); got != "" {
		t.Errorf("没有 go.mod 时 = %q，期望空串", got)
	}
}

// 带包名的写法不能算作引用，否则会给块加上用不到的 import、让它编译失败被静默跳过。
func TestStripComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // 期望的"去注释后"结果（用 strings.Contains 断言判据）
	}{
		{"行尾注释", `fmt.Println(cmp.Compare(1, 2)) // 约定同 time.Duration`, `fmt.Println(cmp.Compare(1, 2))`},
		{"整行注释", "// Type：完整类型，如 time.Duration\nfmt.Println(v.Type())", "fmt.Println(v.Type())"},
		{"字符串里的双斜杠不截断", `p := "http://example.com"` + "\n" + `u, _ := url.Parse(p)`, `url.Parse(p)`},
		{"原始字符串里的双斜杠不截断", "re := `^a//b$`\nregexp.MustCompile(re)", "regexp.MustCompile(re)"},
		{"注释在字符串之后", `fmt.Println("a//b") // 说明 sort.Slice`, `fmt.Println("a//b")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripComments(tt.in)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("stripComments(%q) = %q，期望包含 %q", tt.in, got, tt.want)
			}
			for _, banned := range []string{"// Type", "// 说明", "// 约定"} {
				if strings.Contains(got, banned) {
					t.Fatalf("stripComments(%q) = %q，注释 %q 没有被去掉", tt.in, got, banned)
				}
			}
		})
	}
}

// 注释里提到的包名不应该让 buildPrograms 引入用不到的 import。
func TestBuildProgramIgnoresCommentsForImports(t *testing.T) {
	code := "// ValueOf：把任意值装进 reflect.Value（例如 time.Duration 这种带包名的类型）\n" +
		"v := reflect.ValueOf(42)\n" +
		"fmt.Println(v.Type()) // int"
	srcs, ok := buildPrograms(code)
	if !ok {
		t.Fatal("buildPrograms 返回 false")
	}
	src := srcs[0]
	if strings.Contains(src, `"time"`) {
		t.Fatalf("注释里的 time.Duration 被当成了引用：\n%s", src)
	}
	if !strings.Contains(src, `"reflect"`) {
		t.Fatalf("真正的 reflect.ValueOf 没有被识别：\n%s", src)
	}
}

// 变量默认留在函数体里：`var c struct{...} = b` 引用的是局部变量 b，提到包级会编译失败。
// 只有被提上去的函数真正引用到的包级变量才跟着提升。
func TestBuildProgramsKeepsLocalVarsInBody(t *testing.T) {
	code := "a := 1\n" +
		"b := 2\n" +
		"\n" +
		"fmt.Println(a == b) // false\n" +
		"\n" +
		"var c struct{ X, Y int } = struct{ X, Y int }{1, 2}\n" +
		"fmt.Println(c) // {1 2}"
	srcs, ok := buildPrograms(code)
	if !ok {
		t.Fatal("buildPrograms 返回 false")
	}
	main := srcs[0]
	head, body, found := strings.Cut(main, "func main() {")
	if !found {
		t.Fatalf("没找到 func main()：\n%s", main)
	}
	if strings.Contains(head, "var c struct{") {
		t.Fatalf("局部 var 被提到了包级：\n%s", main)
	}
	if !strings.Contains(body, "var c struct{ X, Y int } = struct{ X, Y int }{1, 2}") {
		t.Fatalf("局部 var 没有留在函数体里：\n%s", main)
	}
	// 顺序也要保持：var c 必须在用到它的 fmt.Println(c) 之前
	if strings.Index(body, "var c struct{") > strings.Index(body, "fmt.Println(c)") {
		t.Fatalf("函数体里的语句顺序被打乱：\n%s", body)
	}
}

// 包级函数引用到的 var 必须跟着提到包级，否则函数体里看不到它。
func TestBuildProgramsHoistsReferencedVar(t *testing.T) {
	code := "var limit = 3\n" +
		"\n" +
		"func under(n int) bool { return n < limit }\n" +
		"\n" +
		"fmt.Println(under(2)) // true"
	srcs, ok := buildPrograms(code)
	if !ok {
		t.Fatal("buildPrograms 返回 false")
	}
	if !strings.Contains(srcs[0], "var limit = 3\n\nfunc under") {
		t.Fatalf("被函数引用的包级 var 没有提升：\n%s", srcs[0])
	}
}

// 片段里出现 `return err`（文档明说这类片段写在"返回 error 的函数"里）时，
// 除了 main 包装，还要给出 `func run() error` 的候选。
func TestBuildProgramsOffersRunWrapper(t *testing.T) {
	code := "f, err := os.Open(\"x\") // 假设这段写在返回 error 的函数里\n" +
		"if err != nil {\n" +
		"    return err\n" +
		"}\n" +
		"\n" +
		"_ = f"
	srcs, ok := buildPrograms(code)
	if !ok {
		t.Fatal("buildPrograms 返回 false")
	}
	if len(srcs) < 2 {
		t.Fatalf("只给出了 %d 个候选包装，期望至少 2 个", len(srcs))
	}
	if !strings.Contains(srcs[1], "func run() error {") {
		t.Fatalf("第二个候选不是 run() error 包装：\n%s", srcs[1])
	}
}

// io/fs 的包名就是 fs（GOROOT/src/io/fs/fs.go 的 package 子句），文档里的 `fs.XXX`
// 必须映射到 "io/fs"。这条映射一度被当成"不存在的包"删掉，副作用是 22 篇所有用 fs 的块
// 连编译都进不去、永远查不了，这里钉住它；同时确认 fstest 仍然映射到 testing/fstest
// （两者包名不同，不能互相替代）。
func TestBuildProgramsMapsIoFS(t *testing.T) {
	code := "fsys := fstest.MapFS{}\n" +
		"data, err := fs.ReadFile(fsys, \"a.txt\")\n" +
		"if err != nil {\n" +
		"    return err\n" +
		"}\n" +
		"\n" +
		"fmt.Println(len(data) == 0) // true"
	srcs, ok := buildPrograms(code)
	if !ok {
		t.Fatal("buildPrograms 返回 false")
	}
	if !strings.Contains(srcs[0], `"io/fs"`) {
		t.Fatalf("fs.XXX 没有映射到 io/fs：\n%s", srcs[0])
	}
	if !strings.Contains(srcs[0], `"testing/fstest"`) {
		t.Fatalf("fstest.XXX 没有映射到 testing/fstest：\n%s", srcs[0])
	}
}

// encoding/json/v2 的包名是 json，文档用 jsonv2 区分 v1/v2，检查工具必须带别名引入。
func TestImportSpecAlias(t *testing.T) {
	if got := importSpec("encoding/json/v2"); got != `jsonv2 "encoding/json/v2"` {
		t.Fatalf("importSpec(encoding/json/v2) = %s", got)
	}
	if got := importSpec("fmt"); got != `"fmt"` {
		t.Fatalf("importSpec(fmt) = %s", got)
	}
}

// 期望输出后面的解释用全角冒号接上时，断言只取冒号前的部分。
func TestExpectationsStripsFullWidthColon(t *testing.T) {
	tests := []struct{ in, want string }{
		{"fmt.Println(info.Mode().IsRegular()) // true：普通文件", "true"},
		{"fmt.Printf(\"%04o\\n\", m) // 0644：只要权限位", "0644"},
		{"fmt.Println(x) // 顺序不固定：map 的遍历顺序是随机的", ""},
	}
	for _, tt := range tests {
		got := expectations(tt.in)
		if tt.want == "" {
			if len(got) != 0 {
				t.Errorf("expectations(%q) = %v，期望不产生断言", tt.in, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != tt.want {
			t.Errorf("expectations(%q) = %v，期望 [%s]", tt.in, got, tt.want)
		}
	}
}

// 链接目标里的 #锚点、<> 包裹都不该影响“这个文件存不存在”的判断。
func TestLinkTarget(t *testing.T) {
	tests := []struct{ in, want string }{
		{"01-变量与基础类型.md", "01-变量与基础类型.md"},
		{"01-变量与基础类型.md#变量声明", "01-变量与基础类型.md"},
		{"<01-变量与基础类型.md>", "01-变量与基础类型.md"},
		{"../README.md#快速开始", "../README.md"},
	}
	for _, tt := range tests {
		if got := linkTarget(tt.in); got != tt.want {
			t.Errorf("linkTarget(%q) = %q，期望 %q", tt.in, got, tt.want)
		}
	}
}

// localRefs 决定“一行里引用了哪些本地文档”。两种写法都要收到，同时不能被
// errors.AsType[T](...) 这类泛型调用误伤——它不是链接，但字面上一样是 ](...)。
func TestLocalRefs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"反引号引用", "控制流的语法见 `03-控制流.md`。", []string{"03-控制流.md"}},
		{"Markdown 链接", "下一篇进入 [`02-表达式与运算符.md`](02-表达式与运算符.md)。",
			[]string{"02-表达式与运算符.md"}},
		{"链接带锚点", "见 [切片](07-切片.md#clear)。", []string{"07-切片.md"}},
		{"链接文字与目标相同时只报一次", "[`02-表达式与运算符.md`](02-表达式与运算符.md)",
			[]string{"02-表达式与运算符.md"}},
		{"泛型调用不是链接", `errors.AsType[*os.PathError](fmt.Errorf("x: %w", os.ErrNotExist))`, nil},
		{"外链不当作本地文件", "见 [手册](https://go.dev/ref/spec.md)。", nil},
		{"相对路径也算引用", "回到目录：[README.md](../README.md)。", []string{"../README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := localRefs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("localRefs(%q) = %v，期望 %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("localRefs(%q) = %v，期望 %v", tt.in, got, tt.want)
				}
			}
		})
	}
}

// 链接文字与目标写成两篇都存在的文件时，存在性检查发现不了，必须单独比对。
func TestMismatchedLinkText(t *testing.T) {
	bad := "[`01-变量与基础类型.md`](02-表达式与运算符.md)"
	text, target, ok := mismatchedLinkText(bad)
	if !ok || text != "01-变量与基础类型.md" || target != "02-表达式与运算符.md" {
		t.Fatalf("mismatchedLinkText(%q) = %q, %q, %v，期望命中", bad, text, target, ok)
	}
	for _, line := range []string{
		"[`01-变量与基础类型.md`](01-变量与基础类型.md)",
		"下一篇进入 `01-变量与基础类型.md`。",
		"见 [切片](07-切片.md#clear)。",
	} {
		if _, _, ok := mismatchedLinkText(line); ok {
			t.Fatalf("mismatchedLinkText(%q) 误报", line)
		}
	}
}

// 块级缓存的键决定"改一段文档要不要重跑"：同一段代码必须稳定命中，代码或文档声明的
// Go 版本变了必须换键——否则改完期望输出还会拿到旧结论，缓存就从加速变成假绿。
func TestBlockKeyStableAndContentSensitive(t *testing.T) {
	code := "_, err := os.ReadFile(\"\")\n\nfmt.Println(errors.Is(err, os.ErrNotExist)) // true\n"
	key := blockKey("1.27.1", code)
	if key != blockKey("1.27.1", code) {
		t.Fatal("同一段代码两次算出的键不同，缓存永远命中不了")
	}
	for _, other := range []string{
		blockKey("1.27.1", code+"\n"), // 期望输出的注释变了也算代码变了
		blockKey("1.26.0", code),      // 文档声明的 Go 版本参与临时模块的 go.mod
	} {
		if key == other {
			t.Fatalf("键 %q 撞车：内容或 Go 版本不同，键必须不同", key)
		}
	}
}

// 缓存只是本地状态：文件不存在、内容是垃圾、或整个路径为空（拿不到用户缓存目录）时，
// 都必须退化成"空缓存"，让检查照常跑，而不是报错或崩掉。
func TestCacheRoundTripAndBadInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "cache.json")
	if got := loadCache(path); len(got) != 0 {
		t.Fatalf("文件不存在时应得到空缓存，实际 %v", got)
	}
	if got := loadCache(""); len(got) != 0 {
		t.Fatalf("路径为空时应得到空缓存，实际 %v", got)
	}

	key := blockKey("1.27.1", "fmt.Println(1)")
	saveCache(path, map[string]exResult{key: {Ran: true, Got: "1\n"}})
	got := loadCache(path)
	if r, ok := got[key]; !ok || !r.Ran || r.Got != "1\n" {
		t.Fatalf("缓存往返后丢内容：%v", got)
	}

	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadCache(path); len(got) != 0 {
		t.Fatalf("缓存内容坏掉时应得到空缓存，实际 %v", got)
	}
}

// -j 只限定并行度，不能变成"0 个 worker"——那会让所有块都跑不了、还被当成依赖前文的片段。
func TestParseJobs(t *testing.T) {
	if got := parseJobs(1); got != 1 {
		t.Fatalf("parseJobs(1) = %d，期望 1", got)
	}
	if got := parseJobs(4); got != 4 {
		t.Fatalf("parseJobs(4) = %d，期望 4", got)
	}
	if got := parseJobs(0); got < 1 {
		t.Fatalf("parseJobs(0) = %d，期望按 CPU 数且至少为 1", got)
	}
}
