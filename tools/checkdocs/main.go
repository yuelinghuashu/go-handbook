// checkdocs 校验 docs/ 下学习文档的四类问题：
//
//  1. 断链：正文里引用的 `NN-xxx.md`（含指向它的 Markdown 链接目标）是否真实存在
//  2. 锚点：正文里写的““xxx”一节”是否能对应到一个真实标题
//  3. 版本标注：篇首的 `> Go 版本：>= X.Y.Z` 是否每篇都有、彼此一致，且与 go.mod 同版本
//  4. 示例输出：把 Go 代码块包成程序跑起来，逐条比对打印语句上标出的期望输出
//
// 用法：
//
//	go run . -dir ../../docs
//	go run . -dir ../../docs -examples=false   # 只做 1、2、3 三项（很快）
//	go run . -dir ../../docs -all              # 诊断用：连没有期望输出的块也编译一遍
//	go run . -dir ../../docs -j 4              # 限定并行度（默认按 CPU 数）
//	go run . -dir ../../docs -cache=false      # 关掉块级缓存，全部重跑
//	go run . -cache-clear                      # 清空块级缓存后退出
//
// 退出码非 0 表示发现问题，可以直接接进 CI。
//
// 第 4 项是最慢的一项：每个块都要起一个 go 进程去编译并运行（400 个块 ≈ 40 秒）。
// 所以它做两件事——并行跑（-j，默认 CPU 数），以及把"跑成功"的结果按内容哈希缓存起来
// （放在用户缓存目录，不进仓库）：文档改一段，重跑就只跑改动到的那几个块。
//
// 关于 -all：默认只编译"有可识别期望输出"的块，因此一个**没有 print 行**的坏块
// （未定义变量、类型写错）会安安静静地待在文档里，CI 一直绿着——22 篇一度有 16 个
// 这样的块。加 -all 会把每个块都试编译一次、逐块打 OK/FAIL，用来做人工体检。
// 它不接 CI：全仓有一大批块本来就"依赖前文定义"，那是文档允许的片段写法。
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	flag.Parse()

	// -cache-clear 只干清缓存这一件事，清完就退出（给 make clean-cache 用）。
	if *cacheClear {
		path := cacheFilePath()
		if path == "" {
			fmt.Fprintln(os.Stderr, "拿不到用户缓存目录，没有可清的块级缓存")
			os.Exit(2)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "清空块级缓存失败:", err)
			os.Exit(2)
		}
		fmt.Println("已清空块级结果缓存：", path)
		return
	}

	docs, err := loadDocs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取文档失败:", err)
		os.Exit(2)
	}
	// 篇首声明的 Go 版本既是"逐篇一致"的检查对象，也是校验示例时临时模块
	// go.mod 的 go 指令来源——原先那里写死过一份 `go 1.27.1`，会和文档各说各话。
	// 这里只声明变量，按编号 3 在下面统一检查，保证输出顺序是 1→4。
	var version string

	failed := 0
	failed += checkLinks(docs, *dir, *verbose)
	failed += checkAnchors(docs, *verbose)
	failed += checkVersion(docs, *dir, &version)
	if *withExamples {
		failed += checkExamples(docs, version, *verbose)
	} else {
		fmt.Println("[4/4] 示例输出检查：已跳过（-examples=false）")
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("发现 %d 处问题\n", failed)
		os.Exit(1)
	}
	fmt.Println("全部检查通过 ✓")
}

// 命令行开关。放包级是为了让 checkExamples 也能读到 -all/-unbuilt，
// 解析仍然只在 main 里做一次（flag.Parse）。
var (
	dir          = flag.String("dir", "docs", "文档目录")
	withExamples = flag.Bool("examples", true, "是否运行示例输出检查（较慢）")
	unbuiltAll   = flag.Bool("unbuilt", false, "列出全部“有期望输出却没跑起来”的块（默认只列前 20 条）")
	allBlocks    = flag.Bool("all", false, "连没有期望输出的块也编译运行一遍（诊断用，见 checkExamples）")
	verbose      = flag.Bool("v", false, "打印每个通过的检查项")
	// 下面三个只管第 4 项的速度：缓存命中时连 go 进程都不用起。
	jobs       = flag.Int("j", 0, "并行跑示例的进程数，0 表示按 CPU 数")
	useCache   = flag.Bool("cache", true, "使用块级结果缓存（放用户缓存目录；改一段文档只重跑那一段）")
	cacheClear = flag.Bool("cache-clear", false, "清空块级结果缓存后退出")
)

// ---------- 文档模型 ----------

type doc struct {
	name   string
	lines  []string
	blocks []block
}

type block struct {
	start int // 1-based 行号
	code  string
}

var fenceRe = regexp.MustCompile("^```(\\w*)")

func loadDocs(dir string) ([]doc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var docs []doc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(raw), "\n")
		d := doc{name: e.Name(), lines: lines}
		for i := 0; i < len(lines); i++ {
			m := fenceRe.FindStringSubmatch(lines[i])
			if m == nil {
				continue
			}
			lang := m[1]
			start := i + 1
			var body []string
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
				body = append(body, lines[j])
				j++
			}
			if lang == "go" {
				d.blocks = append(d.blocks, block{start: start + 1, code: strings.Join(body, "\n")})
			}
			i = j
		}
		docs = append(docs, d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].name < docs[j].name })
	return docs, nil
}

func (d doc) text() string { return strings.Join(d.lines, "\n") }

func (d doc) headings() map[string]bool {
	h := map[string]bool{}
	for _, l := range d.lines {
		if strings.HasPrefix(l, "#") {
			h[strings.TrimSpace(strings.TrimLeft(l, "#"))] = true
		}
	}
	return h
}

// ---------- 1. 断链 ----------

var linkRe = regexp.MustCompile("`(\\d\\d-[^`]+\\.md)`")

// 跨篇引用现在有两种写法：反引号包住文件名，以及指向它的 Markdown 链接
// （例如“下一篇进入 [`01-变量与基础类型.md`](01-变量与基础类型.md)。”）。
// 两种都要查：只查反引号的话，链接目标写错就漏过去了。

// Markdown 链接的目标部分：](目标)，目标里不含空白和右括号
var mdTargetRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// “反引号文件名 + 紧随其后的链接目标”，用于比对文字与目标是否指向同一篇
var mdTextTargetRe = regexp.MustCompile("`(\\d\\d-[^`]+\\.md)`\\]\\(([^)\\s]+)\\)")

// linkTarget 把链接目标规整成可比较的文件路径：去掉 <> 包裹和 #锚点。
func linkTarget(raw string) string {
	t := strings.TrimSuffix(strings.TrimPrefix(raw, "<"), ">")
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t = t[:i]
	}
	return t
}

// localRefs 返回一行里引用的本地 Markdown 文件，按出现顺序排列，同一文件名只留一次。
//
// 只认 .md，并且跳过带 scheme 的外链。这一点是必要的：文档里 `errors.AsType[T](...)`
// 这类泛型调用长得也像 ](...)，但它们不以 .md 结尾，不会被误判成链接。
func localRefs(line string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		if t == "" || !strings.HasSuffix(t, ".md") || strings.Contains(t, "://") || seen[t] {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
		add(m[1])
	}
	for _, m := range mdTargetRe.FindAllStringSubmatch(line, -1) {
		add(linkTarget(m[1]))
	}
	return out
}

// mismatchedLinkText 找出“反引号里写的文件名”与“链接目标”不是同一篇的配对。
//
// 两边各自的存在性都会被 localRefs 查到，但如果写错的那一篇**恰好也存在**，
// 只查存在性发现不了，所以这里再比对一次文字与目标。
func mismatchedLinkText(line string) (text, target string, bad bool) {
	for _, m := range mdTextTargetRe.FindAllStringSubmatch(line, -1) {
		t := linkTarget(m[2])
		if t == "" || strings.Contains(t, "://") || t == m[1] {
			continue
		}
		return m[1], t, true
	}
	return "", "", false
}

func checkLinks(docs []doc, dir string, verbose bool) int {
	fmt.Println("[1/4] 断链检查")
	exist := map[string]bool{}
	for _, d := range docs {
		exist[d.name] = true
	}
	// 链接目标可能带相对路径（例如末篇的 ../README.md），这类只能落到磁盘上确认
	onDisk := map[string]bool{}
	exists := func(name string) bool {
		if exist[name] {
			return true
		}
		v, ok := onDisk[name]
		if !ok {
			_, err := os.Stat(filepath.Join(dir, name))
			v = err == nil
			onDisk[name] = v
		}
		return v
	}
	bad := 0
	for _, d := range docs {
		for i, l := range d.lines {
			for _, ref := range localRefs(l) {
				if !exists(ref) {
					fmt.Printf("  %s:%d 引用了不存在的文件 %s\n", d.name, i+1, ref)
					bad++
				}
			}
			if text, target, ok := mismatchedLinkText(l); ok {
				fmt.Printf("  %s:%d 链接文字与目标不一致：%s → %s\n", d.name, i+1, text, target)
				bad++
			}
		}
	}
	if bad == 0 {
		fmt.Println("  没有断链 ✓")
	}
	return bad
}

// ---------- 2. 锚点 ----------

var anchorRe = regexp.MustCompile("“([^”]+)”[一两]?节")

func checkAnchors(docs []doc, verbose bool) int {
	fmt.Println("[2/4] 锚点检查")
	all := map[string]bool{}
	for _, d := range docs {
		for h := range d.headings() {
			all[h] = true
		}
	}
	bad, checked := 0, 0
	for _, d := range docs {
		for i, l := range d.lines {
			for _, m := range anchorRe.FindAllStringSubmatch(l, -1) {
				for _, name := range strings.Split(m[1], "、") {
					name = strings.TrimSpace(name)
					if name == "" {
						continue
					}
					checked++
					if !all[name] {
						fmt.Printf("  %s:%d 锚点“%s”没有对应标题\n", d.name, i+1, name)
						bad++
					}
				}
			}
		}
	}
	if bad == 0 {
		fmt.Printf("  %d 个锚点全部命中 ✓\n", checked)
	}
	return bad
}

// ---------- 4. 示例输出：期望值提取与程序包装 ----------

// 打印语句所在行的注释，用 —— 或 ，等分隔出"期望输出"部分
var printLineRe = regexp.MustCompile(`fmt\.Print[fmln]*\(`)
var expectRe = regexp.MustCompile(`//\s*(.+)$`)

// 只把"看起来像 Go 打印结果"的注释当作断言，避免把说明性文字误判成输出
var valueShapeRe = regexp.MustCompile(`^[\[\{"'\-]|^-?\d|^(true|false|nil|map\[|<\w+>)`)

var qualifierRe = regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\.`)

// 说明性注释里常见的开头，出现这些一律不当断言
var prosePrefix = []string{"形如", "例如", "注意", "这里", "说明", "示意", "每次", "取决于"}

var imports = map[string]string{
	"fmt": "fmt", "strings": "strings", "slices": "slices", "maps": "maps", "cmp": "cmp",
	"sort": "sort", "errors": "errors", "os": "os", "time": "time", "math": "math",
	"strconv": "strconv", "unicode": "unicode", "utf8": "unicode/utf8", "regexp": "regexp",
	"flag": "flag", "json": "encoding/json", "jsonv2": "encoding/json/v2",
	"jsontext": "encoding/json/jsontext", "base64": "encoding/base64", "hex": "encoding/hex",
	"binary": "encoding/binary", "csv": "encoding/csv", "xml": "encoding/xml", "gob": "encoding/gob",
	"crc32": "hash/crc32", "gzip": "compress/gzip", "zip": "archive/zip", "tar": "archive/tar",
	"io": "io", "bufio": "bufio", "bytes": "bytes", "filepath": "path/filepath", "path": "path",
	// io/fs 的包名就是 fs（GOROOT/src/io/fs/fs.go 的 package 子句），所以 `fs.XXX`
	// 必须映射到 "io/fs"。这里一度把它删掉过，理由是"io/fs 下并没有一个叫 fs 的包"——
	// 理由本身是错的，副作用是 22 篇所有 `fs.XXX` 的块连编译都进不去、永远查不了。
	"fs":   "io/fs",
	"exec": "os/exec", "signal": "os/signal", "runtime": "runtime", "debug": "runtime/debug",
	"pprof": "runtime/pprof", "reflect": "reflect", "unsafe": "unsafe", "atomic": "sync/atomic",
	"sync": "sync", "context": "context", "iter": "iter", "uuid": "uuid", "slog": "log/slog",
	"log": "log", "big": "math/big", "bits": "math/bits", "cmplx": "math/cmplx", "rand": "math/rand/v2",
	"template": "text/template", "heap": "container/heap", "sha256": "crypto/sha256",
	"sha3": "crypto/sha3", "mldsa": "crypto/mldsa", "http": "net/http", "url": "net/url",
	"tls": "crypto/tls", "ecdh": "crypto/ecdh", "mlkem": "crypto/mlkem", "hpke": "crypto/hpke",
	"netip": "net/netip", "unique": "unique", "weak": "weak", "buildinfo": "debug/buildinfo",
	// testing/fstest 提供 fstest.MapFS：文档里用内存文件系统当例子素材时用得到它，
	// 与上面的 io/fs 是两回事（一个包名是 fstest，一个是 fs），两条都要留着。
	"fstest": "testing/fstest",
	// 19 篇的片段里会出现 *testing.T（t.Context/t.Deadline 之类的方法调用），
	// 34 篇用 structs.HostLayout 标布局；不加这两条，包装出来的程序会缺 import、
	// 被静默跳过（明明是真错误却看不出来）。
	"testing": "testing", "structs": "structs",
}

// aliasImports 记下"必须带别名才能用"的包路径：包的**包名**与文档里用的标识符不一致时，
// 按路径直接 import 会绑错名字。文档里用 `jsonv2` 区分 v1/v2，而 encoding/json/v2 的
// 包名就是 json，不加别名会与 encoding/json 撞名。
var aliasImports = map[string]string{
	"jsonv2": "encoding/json/v2",
}

var declRe = regexp.MustCompile(`^(func|type|const|var)\b`)

// declLines 返回从第 i 行开始的声明占多少行。
//
// 只有"以 { 或 ( 结尾"的声明（函数体、struct/interface、分组的 const/var）才需要按括号配对
// 往下找；其余（`type Counter int`、`var c struct{ X, Y int } = b`、单行写法的函数）就占一行。
// 早先按"括号是否配平 + 行末是不是 }"判断，会把 `var c struct{...} = b` 这种自带花括号的
// 单行声明一路吃到下一行，提到包级后编译报错。
func declLines(lines []string, i int) int {
	l := strings.TrimSpace(lines[i])
	if !strings.HasSuffix(l, "{") && !strings.HasSuffix(l, "(") {
		return 1
	}
	depth := 0
	for j := i; j < len(lines); j++ {
		depth += strings.Count(lines[j], "{") + strings.Count(lines[j], "(") -
			strings.Count(lines[j], "}") - strings.Count(lines[j], ")")
		if depth <= 0 {
			return j - i + 1
		}
	}
	return len(lines) - i
}

// splitDecls 把片段拆成"要提到包级的声明"和"留在函数体里的语句"，两者都保持原顺序。
//
// 函数、类型、常量必须提上去（Go 不允许在函数里声明具名函数）。变量默认留在原处：
// 提到包级会让引用局部变量的写法（`var c struct{ X, Y int } = b`）编译失败，
// 只有被提上去的函数真正引用到的变量才跟着上去。
func splitDecls(code string) (decls, body []string) {
	type item struct {
		lines  []string
		isVar  bool
		isDecl bool
	}
	lines := strings.Split(code, "\n")
	var items []item
	for i := 0; i < len(lines); {
		if !declRe.MatchString(lines[i]) {
			items = append(items, item{lines: []string{lines[i]}})
			i++
			continue
		}
		n := declLines(lines, i)
		chunk := lines[i : i+n]
		trimmed := strings.TrimSpace(strings.Join(chunk, "\n"))
		isVar := strings.HasPrefix(trimmed, "var ")
		items = append(items, item{lines: chunk, isVar: isVar, isDecl: true})
		i += n
	}

	// 先看非 var 的声明里提到了哪些名字，用来判断哪些 var 必须一起提升。
	var hoistedText strings.Builder
	for _, it := range items {
		if it.isDecl && !it.isVar {
			hoistedText.WriteString(strings.Join(it.lines, "\n"))
			hoistedText.WriteString("\n")
		}
	}
	hoistVar := map[int]bool{}
	for idx, it := range items {
		if !it.isVar {
			continue
		}
		name := declaredName(strings.Join(it.lines, "\n"))
		if name == "" {
			continue
		}
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(hoistedText.String()) {
			hoistVar[idx] = true
		}
	}

	for idx, it := range items {
		switch {
		case it.isDecl && (!it.isVar || hoistVar[idx]):
			decls = append(decls, strings.Join(it.lines, "\n"))
		default:
			body = append(body, it.lines...)
		}
	}
	return decls, body
}

var varNameRe = regexp.MustCompile(`^var\s+(\w+)`)

func declaredName(decl string) string {
	// `var (a, b int)` 这类分组取不到名字，返回空串表示"不提上去"
	m := varNameRe.FindStringSubmatch(strings.TrimSpace(decl))
	if m == nil {
		return ""
	}
	return m[1]
}

// buildPrograms 把文档片段包成可运行程序，按尝试顺序返回若干候选；做不到时返回 false。
//
// 候选不只一个是有原因的：文档里大量出现 `if err != nil { return err }` 这种片段，
// 它们是"写在返回 error 的函数里"的（01 篇开头就把这条约定写明了），直接塞进 main
// 会因为 `too many return values` 编译失败、被静默跳过。main 包装失败时再试
// `func run() error`，让这类片段也能被真的跑一遍。
func buildPrograms(code string) ([]string, bool) {
	if regexp.MustCompile(`(?m)^package\s`).MatchString(code) {
		return nil, false
	}
	decls, body := splitDecls(code)

	var imp strings.Builder
	var paths []string
	seen := map[string]bool{}
	for _, m := range qualifierRe.FindAllStringSubmatch(stripComments(code), -1) {
		p, ok := imports[m[1]]
		if !ok || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&imp, "\t%s\n", importSpec(p))
	}
	if imp.Len() == 0 {
		imp.WriteString("\t_ \"unsafe\"\n")
	}

	var bodyIndented strings.Builder
	for _, l := range body {
		if strings.TrimSpace(l) != "" {
			bodyIndented.WriteString("\t" + l + "\n")
		}
	}

	head := "package main\n\nimport (\n" + imp.String() + ")\n\n" +
		strings.Join(decls, "\n\n") + "\n\n"
	body = nil
	src := []string{
		head + "func main() {\n" + bodyIndented.String() + "}\n",
		head + "func main() {\n\t_ = run()\n}\n\nfunc run() error {\n" +
			bodyIndented.String() + "\treturn nil\n}\n",
		head + "func main() {\n\t_, _ = run()\n}\n\nfunc run() (any, any) {\n" +
			bodyIndented.String() + "\treturn nil, nil\n}\n",
	}
	return src, true
}

// importSpec 返回 import 行里的内容：包的包名与文档里的写法不一致时必须带别名
// （encoding/json/v2 的包名是 json，不加别名会和 encoding/json 撞名）。
func importSpec(path string) string {
	for alias, p := range aliasImports {
		if p == path {
			return alias + " " + strconv.Quote(path)
		}
	}
	return strconv.Quote(path)
}

// stripComments 去掉行注释，只用于"猜这段代码需要哪些 import"。
//
// 注释里经常提到带包名的标识符（例如解释文字里的 `time.Duration`、`sort.Slice`），
// 如果把它们也算作引用，就会给块引入一个用不到的 import，让本来能跑的块编译失败。
// 用引号状态判断 `//` 是否落在字符串字面量里，避免把 `"http://x"` 从中间截断。
func stripComments(code string) string {
	var out strings.Builder
	for _, line := range strings.Split(code, "\n") {
		var quote byte
		escaped := false
		cut := -1
		for i := 0; i < len(line); i++ {
			c := line[i]
			if quote != 0 {
				switch {
				case escaped:
					escaped = false
				case c == '\\' && quote != '`':
					escaped = true
				case c == quote:
					quote = 0
				}
				continue
			}
			switch {
			// 字符字面量也要认：`case '/':` 或 `[]byte{'/'}` 里的斜杠不是注释开头。
			// 少了这一条，`strings.Split(s, "//")` 这行之后的 `//` 会被当注释截断，
			// 括号配对随之失衡、猜 import 也漏掉真正用到的包。
			case c == '"' || c == '`' || c == '\'':
				quote = c
			case c == '/' && i+1 < len(line) && line[i+1] == '/':
				cut = i
			}
			if cut >= 0 {
				break
			}
		}
		if cut >= 0 {
			line = line[:cut]
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

// expectations 提取"打印行上标出的期望输出"
func expectations(code string) []string {
	var out []string
	for _, line := range strings.Split(code, "\n") {
		if !printLineRe.MatchString(line) {
			continue
		}
		m := expectRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		c := strings.TrimSpace(m[1])
		// 去掉破折号、冒号后的解释、括号补充
		for _, sep := range []string{"——", "–", "，", "；", "：", "（", "(", "。"} {
			if i := strings.Index(c, sep); i >= 0 {
				c = strings.TrimSpace(c[:i])
			}
		}
		if c == "" || strings.Contains(c, "…") || strings.Contains(c, "...") {
			continue
		}
		skip := false
		for _, p := range prosePrefix {
			if strings.HasPrefix(c, p) {
				skip = true
				break
			}
		}
		if skip || !valueShapeRe.MatchString(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// ---------- 3. 篇首的 Go 版本 ----------

// 每篇篇首形如 `> Go 版本：>= 1.27.1`（后面的 `> 相关标准库：…` 是可选行）。
var docVersionRe = regexp.MustCompile(`^>\s*Go 版本：>= (\d+\.\d+(?:\.\d+)?)\s*$`)

// moduleGoVersion 从 go.mod 里取 go 指令声明的版本；文件不存在时返回空串。
//
// 从文档目录往上找：checkdocs 通常以 `-dir docs` 从仓库根运行，但也能被指向别的
// 目录（例如 tools/checkdocs 下的 `go run . -dir ../../docs`），所以不能假设固定深度。
func moduleGoVersion(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		raw, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for _, l := range strings.Split(string(raw), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "go "); ok {
					return strings.TrimSpace(v)
				}
			}
			return ""
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// sameRelease 判断两个版本号是不是同一个 Go 版本：`1.27` 与 `1.27.1` 算同一个，
// 这样 go.mod 写 patch 版、文档写 `>= 1.27` 也不会互相打架。
func sameRelease(a, b string) bool {
	minor := func(v string) string {
		parts := strings.Split(v, ".")
		if len(parts) < 2 {
			return v
		}
		return parts[0] + "." + parts[1]
	}
	return a == b || minor(a) == minor(b)
}

// checkVersion 校验"版本号只写在一个地方"这件事：
//
//  1. 每篇文档都必须有篇首版本标注（少了就没人知道这篇按哪个版本实测过）
//  2. 所有篇的版本必须一致（升级时漏改某篇，这里会当场指出来，不必靠人肉比对）
//  3. 与仓库 go.mod 的 go 指令必须是同一个 Go 版本（只有找得到 go.mod 时才查）
//
// 找到的版本写回 out，供示例检查生成临时模块的 go.mod 使用：这样"文档说按哪个
// 版本实测"与"校验时实际用哪个版本编译"是同一份来源，不会再各改一半。
func checkVersion(docs []doc, dir string, out *string) int {
	fmt.Println("[3/4] 版本标注检查")
	canonical, firstDoc := "", ""
	bad := 0
	for _, d := range docs {
		v := ""
		for _, l := range d.lines {
			if m := docVersionRe.FindStringSubmatch(l); m != nil {
				v = m[1]
				break
			}
		}
		if v == "" {
			fmt.Printf("  %s 缺少篇首版本标注（形如 `> Go 版本：>= 1.27.1`）\n", d.name)
			bad++
			continue
		}
		if canonical == "" {
			canonical, firstDoc = v, d.name
			continue
		}
		if !sameRelease(v, canonical) {
			fmt.Printf("  %s 写的是 %s，与 %s 的 %s 不一致\n", d.name, v, firstDoc, canonical)
			bad++
		}
	}
	if canonical == "" {
		fmt.Println("  没有任何一篇声明了 Go 版本")
		return bad + 1
	}
	*out = canonical
	if mod := moduleGoVersion(dir); mod != "" && !sameRelease(canonical, mod) {
		fmt.Printf("  文档声明 %s，但 go.mod 的 go 指令是 %s\n", canonical, mod)
		bad++
	}
	if bad == 0 {
		fmt.Printf("  %d 篇全部标注 %s ✓\n", len(docs), canonical)
	}
	return bad
}

// ---------- 4. 示例输出：并行运行、块级缓存与比对 ----------

// exTask 是一个待跑的代码块。先把文档里所有块收集成任务、并行执行，最后按文档顺序统计
// 与打印——并行不能把 CI 日志里的问题顺序打乱。
type exTask struct {
	doc  string
	line int
	code string
	want []string
}

// exResult 是一个块的执行结果，也是缓存里存的东西。
//
//   - Ran：块独立编译并运行成功，Got 是它的输出；
//   - PackageDecl：块里带 package 声明，本来就无法包装（不是错误，-all 里也不报 FAIL）；
//   - ErrMsg：没能独立跑起来时的第一条诊断，进"依赖前文片段"清单；
//   - TimedOut：超时。它不进缓存——超时可能是机器负载导致的偶发，存下来会一直错下去。
type exResult struct {
	Ran         bool   `json:"ran"`
	PackageDecl bool   `json:"pkgDecl,omitempty"`
	Got         string `json:"got,omitempty"`
	ErrMsg      string `json:"errMsg,omitempty"`

	TimedOut bool `json:"-"`
}

// cacheSchema 参与缓存键：checkdocs 的包装方式（buildPrograms/splitDecls）或比对规则改了、
// 老结论可能不再成立时，把它 +1，旧缓存自动失效。
const cacheSchema = "v1"

// blockKey 是块级缓存的键。一段代码的结果只取决于它自己、Go 版本和平台：每个块都是在同一个
// 临时模块里独立编译的，不读同篇的其它块（依赖前文定义的片段，结果是"编译失败"，同样稳定）。
// 平台进键是因为输出里难免有系统相关的文案。
func blockKey(goVersion, code string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{
		cacheSchema, runtime.Version(), runtime.GOOS, runtime.GOARCH, goVersion, code,
	}, "\x00")))
	return hex.EncodeToString(h[:16])
}

// cacheFilePath 把块级缓存放用户缓存目录，不放仓库里：它只是本地的加速状态，不该出现在
// git status 里，也不该被"跑得快一点"之外的任何东西依赖。拿不到目录时返回空串（=不用缓存）。
func cacheFilePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "go-handbook", "checkdocs-cache.json")
}

// loadCache 读块级缓存。文件不存在、或读不出来（例如格式变了）都当成空缓存：缓存只影响
// 速度、不影响结论，任何异常都不该让检查失败。
func loadCache(path string) map[string]exResult {
	m := map[string]exResult{}
	if path == "" || !*useCache {
		return m
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		if *verbose {
			fmt.Printf("  块级缓存无法解析，本次当作空缓存：%v\n", err)
		}
		return map[string]exResult{}
	}
	return m
}

// saveCache 写块级缓存。map 里已经带着旧内容（loadCache 的返回值被原地追加），所以
// `-dir` 只指一部分文档、或 -all 与默认模式交替跑，都不会把别的块的结果丢掉。
func saveCache(path string, m map[string]exResult) {
	if path == "" || !*useCache || len(m) == 0 {
		return
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		if *verbose {
			fmt.Printf("  建缓存目录失败，本次不写缓存：%v\n", err)
		}
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil && *verbose {
		fmt.Printf("  写块级缓存失败（不影响结论）：%v\n", err)
	}
}

// parseJobs 把 -j 换算成实际的并行度：默认按 CPU 数，至少 1。
func parseJobs(n int) int {
	if n > 0 {
		return n
	}
	if n := runtime.GOMAXPROCS(0); n > 0 {
		return n
	}
	return 1
}

// runBlocks 并行跑所有任务，返回与 tasks 等长的结果（顺序即文档顺序）和缓存命中数。
//
// 每个块用一个独立的源文件名：早先所有块共用临时目录里的 m.go，那样没法并行——两个
// worker 会互相覆盖源码，甚至跑到对方写的程序。
func runBlocks(tmp string, tasks []exTask, goVersion string, cache map[string]exResult, workers int) ([]exResult, int) {
	results := make([]exResult, len(tasks))
	var todo []int
	hits := 0
	for i, t := range tasks {
		if r, ok := cache[blockKey(goVersion, t.code)]; ok {
			results[i] = r
			hits++
			continue
		}
		todo = append(todo, i)
	}
	if workers > len(todo) {
		workers = len(todo)
	}
	if workers < 1 {
		workers = 1
	}
	idx := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				results[i] = runBlock(tmp, i, tasks[i].code)
			}
		}()
	}
	for _, i := range todo {
		idx <- i
	}
	close(idx)
	wg.Wait()
	return results, hits
}

// runBlock 跑一个块：包装成程序（见 buildPrograms），编译并执行，时间上限 30 秒。
func runBlock(tmp string, i int, code string) exResult {
	candidates, ok := buildPrograms(code)
	if !ok {
		return exResult{PackageDecl: true, ErrMsg: "块里有 package 声明，整块程序不参与包装"}
	}
	name := fmt.Sprintf("b%04d.go", i)
	file := filepath.Join(tmp, name)

	var out []byte
	var lastErr error
	timedOut := false
	for _, src := range candidates {
		if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
			lastErr = err
			continue
		}
		// 加超时保护：块里可能有不会终止的循环（例如 ticker + for），没有超时会让整个
		// 检查永久挂住。WaitDelay 用于在 go run 被取消后关掉它的管道——否则 go run
		// 启动的那个二进制还在写 stdout，Output() 会一直等管道关闭。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "go", "run", name)
		cmd.Dir = tmp
		cmd.WaitDelay = 5 * time.Second
		var err error
		out, err = cmd.Output()
		timedOut = ctx.Err() == context.DeadlineExceeded
		cancel()
		lastErr = err
		if err == nil {
			break
		}
		if timedOut {
			break // 超时换包装也没用，别把 30 秒乘上候选数
		}
	}
	switch {
	case lastErr == nil:
		return exResult{Ran: true, Got: string(out)}
	case timedOut:
		return exResult{TimedOut: true}
	default:
		return exResult{ErrMsg: firstBuildError(lastErr)}
	}
}

func checkExamples(docs []doc, goVersion string, verbose bool) int {
	fmt.Println("[4/4] 示例输出检查")
	// 先收集任务。默认只跑"有期望输出"的块：没有断言的块是片段，跑不跑都不影响比对。
	// 加 -all 会把每个块都试编译一次、逐块打 OK/FAIL，用来做人工体检（见包注释）。
	var tasks []exTask
	for _, d := range docs {
		for _, b := range d.blocks {
			want := expectations(b.code)
			if len(want) == 0 && !*allBlocks {
				continue
			}
			tasks = append(tasks, exTask{doc: d.name, line: b.start, code: b.code, want: want})
		}
	}

	tmp, err := os.MkdirTemp("", "checkdocs-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "  创建临时目录失败:", err)
		return 1
	}
	defer os.RemoveAll(tmp)
	// 临时模块的 go 指令用文档声明的版本：这样"文档说按哪个版本实测"和"校验时实际
	// 用哪个版本编译"是同一份来源，升级 Go 时不会再出现两处各改一半的情况。
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"),
		[]byte("module checkdocs\n\ngo "+goVersion+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "  写 go.mod 失败:", err)
		return 1
	}

	cachePath := cacheFilePath()
	cache := loadCache(cachePath)
	workers := parseJobs(*jobs)
	results, hits := runBlocks(tmp, tasks, goVersion, cache, workers)

	ran, skipped, bad := 0, 0, 0
	var unbuilt []string
	// 统计与打印统一按文档顺序走一遍：并行执行，输出顺序和串行时完全一样。
	for i, t := range tasks {
		r := results[i]
		switch {
		case r.TimedOut:
			skipped++
			fmt.Printf("  %s:%d 运行超过 30s，已跳过（块里可能有不会终止的循环）\n",
				t.doc, t.line)
		case !r.Ran:
			skipped++ // 片段依赖前文上下文、无法独立运行
			unbuilt = append(unbuilt, fmt.Sprintf("%s:%d %s", t.doc, t.line, r.ErrMsg))
			if *allBlocks && !r.PackageDecl {
				fmt.Printf("  FAIL %s:%d %s\n", t.doc, t.line, r.ErrMsg)
			}
		default:
			ran++
			if *allBlocks {
				fmt.Printf("  OK   %s:%d（断言 %d 条）\n", t.doc, t.line, len(t.want))
			}
			for _, w := range t.want {
				if outputMatches(w, r.Got) {
					continue
				}
				fmt.Printf("  %s:%d 期望输出 %q 未出现\n      实际输出: %q\n",
					t.doc, t.line, w, strings.TrimSpace(r.Got))
				bad++
			}
		}
		// 只缓存"跑成功"的块：编译失败可能是环境偶发（模块解析、并发压力），存下来会
		// 一直错下去；而失败本来很快，重跑也不贵。
		if r.Ran {
			cache[blockKey(goVersion, t.code)] = exResult{Ran: true, Got: r.Got}
		}
	}
	saveCache(cachePath, cache)

	fmt.Printf("  实际运行 %d 个可运行块，跳过 %d 个依赖上下文的块\n", ran, skipped)
	if verbose {
		fmt.Printf("  并行 %d 路，块级缓存命中 %d/%d\n", workers, hits, len(tasks))
	}
	if len(unbuilt) > 0 {
		// 只列前若干条：这些块"写了期望输出却没跑起来"，可能是依赖前文的片段，
		// 也可能是真错误（写错类型、块之间复制粘贴漏了定义）。列出来才能人工分辨。
		// 想知道全部条目就加 -unbuilt：这份清单就是"哪一段依赖前文定义"的权威列表。
		if *allBlocks {
			fmt.Printf("  其中 %d 个块没能独立编译运行（-all 模式：含没有断言的片段，按需核对）：\n", len(unbuilt))
		} else {
			fmt.Printf("  其中 %d 个块有期望输出但没能运行（依赖前文片段或真错误，按需核对）：\n", len(unbuilt))
		}
		maxList := 20
		if *unbuiltAll {
			maxList = len(unbuilt)
		}
		for i, msg := range unbuilt {
			if i == maxList {
				fmt.Printf("    ……另有 %d 处（加 -unbuilt 看完整列表）\n", len(unbuilt)-maxList)
				break
			}
			fmt.Printf("    %s\n", msg)
		}
	}
	if bad == 0 {
		fmt.Println("  所有可比对的期望输出都吻合 ✓")
	}
	return bad
}

// firstBuildError 从 `go run` 的报错里挑一行最有用的：跳过 "# ..." 这类包名行，
// 取第一条带位置的诊断。
func firstBuildError(err error) string {
	var ee *exec.ExitError
	if !errors.As(err, &ee) || len(ee.Stderr) == 0 {
		return err.Error()
	}
	for _, l := range strings.Split(string(ee.Stderr), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		// 去掉 `./m.go:` 这样的文件名前缀，留下 `14:7: undefined: m`
		if i := strings.Index(l, ".go:"); i >= 0 {
			l = l[i+len(".go:"):]
		}
		return l
	}
	return err.Error()
}

var wsRe = regexp.MustCompile(`\s+`)

// outputMatches 判断期望输出是否与真实输出相符。
//
// 先做整体比对（忽略空白差异），失败后回退到"逐 token 比对"：文档里常用
// `// 0 1 2 3 4` 或 `// "go" / "is" / "fast"` 这种把多行输出压成一行、
// 或用斜杠分隔的写法，逐 token 判断能支持这种约定，同时仍能发现
// 某个值根本不在输出里（真正的错误）。
func outputMatches(want, got string) bool {
	nw := wsRe.ReplaceAllString(strings.TrimSpace(want), " ")
	ng := wsRe.ReplaceAllString(strings.TrimSpace(got), " ")
	if strings.Contains(ng, nw) {
		return true
	}
	return allTokensPresent(nw, ng)
}

func allTokensPresent(want, got string) bool {
	tokens := strings.FieldsFunc(want, func(r rune) bool {
		return r == ' ' || r == '/' || r == '、' || r == '\t'
	})
	checked := 0
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		// 只有"看起来是值"的 token 才参与判断，纯标点忽略
		if t == "" || !valueShapeRe.MatchString(t) {
			continue
		}
		checked++
		if !strings.Contains(got, t) {
			return false
		}
	}
	// 一个可判断的 token 都没有时，不要武断判错
	return checked > 0
}
