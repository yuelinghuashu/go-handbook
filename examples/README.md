# examples

这个目录存放 `docs/` 学习文档里**应当能跑出所写结果**的核心示例。

每个示例是一个 `Example` 函数，末尾用 `// Output:` 注释写出期望输出，`go test` 会真的执行它并逐字比对——**文档和代码在这里不会各说各话**。

```go
func Example_mapCommaOk() {
	m := map[string]int{"a": 1}

	v, ok := m["missing"]
	fmt.Println(v, ok)
	// Output: 0 false
}
```

## 运行

在**仓库根目录**执行（`examples/` 与 `tools/checkdocs/` 同属一个 Go 模块）：

```bash
go test ./examples/...                      # 全部校验
go test -v ./examples/...                   # 看每个示例的名字
go test -run Example_mapCommaOk -v ./examples/...   # 只跑一个

go test ./...                               # 也可以：整个仓库的测试
```

## 约定

- **函数名统一用 `Example_xxx` 形式**（下划线后缀）。这样不要求包里真的存在同名标识符，也不会被 `go vet` 的示例命名检查挑刺。
- **`// Output:` 必须是函数体里最后一个注释块**。如果它前面还有别的注释行，`go test` 不会识别成输出断言，示例会被编译但**静默地不执行**——这是很容易踩的坑。
- **输出顺序不确定时用 `// Unordered output:`**。
- 每个示例只演示一个知识点，名字里写清是哪个（`Example_slicesDeleteZeroesTail`）。
- 示例要**确定性**：随机来源用固定种子（`rand.New(rand.NewPCG(1, 2))`），不要依赖当前时间、map 遍历顺序、goroutine 调度顺序。
- 需要演示 panic 时用 `defer recover()` 把它变成可断言的输出，不要让示例真的崩掉。

## 与 `docs/` 的关系

示例是文档的**可执行版本**，不是文档的替代品：

- 文档负责讲清"为什么"和"什么时候用"，示例负责证明"输出确实是这样"。
- 改动文档里的示例时，**先改这里让它通过，再把代码抄回文档**，顺序反了就容易写出对不上的输出。
- `tools/checkdocs`（仓库根的 `./tools/checkdocs`）会直接在 `docs/` 的代码块上做同样的比对，覆盖那些还没搬过来的示例。

## 覆盖范围

每个示例上方都有一行 `// NN ...` 标注它属于哪一篇，所以"某一篇配套了哪些示例"可以直接查。**读完某一篇想动手跑一遍**，按篇号找函数名即可：

```bash
grep -n "^// 07 " examples/*.go                      # 07 篇的示例
go test -run 'Example_(appendSelf|appendGrowth)' -v ./examples/...
```

| 篇 | 示例函数 |
| --- | --- |
| 01 | `variablesZeroValues`、`integerLiterals`、`untypedFloatConstantsAreExact`、`iotaReusesExpression`、`runeLiterals`、`stringLiteralKinds`、`intToString`、`floatFormatting`、`hexVerbOnString` |
| 02 | `integerDivision`、`integerOverflow`、`bitwiseOperators`、`minMaxBuiltins` |
| 05 | `errorsIsThroughWrapping`、`errorsJoinAndUnwrap` |
| 06 | `arrayValueSemantics`、`sliceToArray` |
| 07 | `nilSliceAppend`、`appendGrowth`、`appendSelf`、`appendOverlapSharedArray`、`copyOverlap`、`deleteElementAppend`、`fullSliceExpressionCapsCapacity`、`nilVsEmptySlice`、`clearSlice` |
| 08 | `nilMapReadOnly`、`mapCommaOk`、`mapDeleteAndClear`、`mapPrintIsSorted` |
| 09 | `structCopy`、`structComparable`、`anonymousStructSameType`、`embeddedPromotedFields`、`emptyStructSize` |
| 10 | `nilPointerDeref`、`newAllocatesZero`、`sizeofValueNotReferent`、`pointerMutation`、`pointerToStruct`、`arrayPointerParam`、`mapElementNotAddressable`、`addressability`、`modifyMapValueThroughPointer`、`newExpr`、`pointerModifiesCaller`、`pointerToSliceHeader`、`pointerComparison`、`loopVarAddress` |
| 11 | `valueReceiverCopies`、`methodValueAndExpression`、`methodSet` |
| 12 | `interfaceAndTypeAssertion`、`typeSwitch`、`nilInterfaceTrap`、`interfaceComparisonPanics`、`compileTimeInterfaceAssertion` |
| 13 | `genericsTypeInference`、`genericsUnionConstraint`、`genericsCmpOrdered`、`genericsTilde`、`genericsType`、`genericsUntypedConstantKind`、`genericsVsInterface`、`stdlibGenericBeforeCustom` |
| 14 | `iteratorCustom`、`iteratorStopsEarly`、`iteratorPull`、`iteratorCollect`、`iteratorChunk`、`iteratorReusable` |
| 15 | `slicesFindAndCompare`、`slicesDeleteZeroesTail`、`slicesInsertInPlace`、`slicesCompact`、`slicesCloneKeepsNil`、`slicesSortAndSearch`、`slicesCompareFuncOverflow`、`slicesMinFuncPanicsWhenEmpty` |
| 16 | `mapsCloneAndCopy`、`mapsEqualAndDeleteFunc`、`mapsKeysIsIterator` |
| 17 | `cmpCompareLessOr`、`cmpNaNOrdering` |
| 20 | `stringIsBytes`、`stringRunes`、`stringsCut`、`stringsBuilder`、`stringsNewReplacer`、`stringsChainedReplaceAll`、`stringsSplitSeqSingleUse` |
| 24 | `timeDuration`、`timeLayoutIsReferenceTime`、`timeDoNotUseEqualOperator`、`timeNormalization`、`timeParseDuration` |
| 25 | `randReproducibleSource`、`randGenericN`、`randShuffleDeterministic`、`mathRounding` |
| 26 | `jsonMarshal`、`jsonOmitzeroVsOmitempty`、`jsonNilVsEmptySlice`、`jsonHTMLEscaping`、`jsonUnmarshal`、`jsonDurationAsNanos` |
| 29 | `regexpASCIIvsUnicode`、`regexpReplaceLiteral`、`regexpQuoteMeta` |
| 30 | `serveMuxEnhancedRouting`、`netipAddr` |
| 32 | `tarDirectoryTypeflag` |
| 35 | `structPadding` |
| 36 | `heapPriorityQueue` |
| 38 | `channelUnbuffered`、`channelCloseAndRange`、`channelClosedReads`、`selectDefault`、`selectNilChannelDisablesCase`、`waitGroup` |
| 39 | `contextCancel`、`contextTimeout` |
| 40 | `syncOnce`、`syncOnceValue`、`atomicCounter`、`atomicTypedValues`、`atomicHappensBefore` |
| 41 | `TestSynctestTimeout`（`concurrency_test.go`，是 `Test` 而不是 `Example`——它需要 `*testing.T`，`tools/checkdocs` 没法把代码块包成 `main()` 跑） |

按文件看：`lang_basics_test.go` 覆盖基础与容器，`lang_types_test.go` 覆盖类型系统，`stdlib_test.go` 覆盖标准库与并发，`concurrency_test.go` 只有上面那一个 `synctest` 用例。

下列 12 篇暂时没有配套示例，它们的示例直接写在文档里、由 `tools/checkdocs` 校验：`00`、`03`、`04`、`18`、`21`、`22`、`23`、`27`、`28`、`31`、`33`、`34`。
