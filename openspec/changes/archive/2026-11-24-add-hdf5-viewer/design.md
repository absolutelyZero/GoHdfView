# Design

## Context

`GoHdfView` 从空目录起步：没有既有代码、没有既有 spec、没有 CI 配置。本设计要为 proposal.md 列出的五个能力选定实现路径，因此需要先确定两个外部约束，它们决定了后面所有选择。

**UI 框架：MyGo v0.10.0。** 它是纯 Go 的原生 UI 框架，`CGO_ENABLED=0`，不依赖 WebView2、Node 或 Bun。视图是一个 `func(c *ui.Context)` 函数，框架按帧调用它重建界面；控件是 build-pass 的临时值，只有 `ListState`、`OutlineState` 这类控件状态和 `ui.Handle` 可以跨帧存活。这一模型决定了状态必须集中在应用层的一个结构体里，视图只做投影。

**HDF5 解析：scigolib/hdf5 v0.15.0。** 纯 Go，支持 superblock v0/2/3 与 HDF5 2.0 格式，读路径完整覆盖组、数据集、属性、hyperslab 切片与过滤器解码。它同时提供写入 API，本变更在代码组织上把写入能力排除在外（见 Decisions）。

已知的库能力边界（决定了设计中的降级路径）：

- `Dataset.ReadSlice` / `ReadHyperslab` 的返回值**始终是 `[]float64`**：库内部把原始字节经 `core.ConvertToFloat64` 统一转换（`dataset_read_hyperslab.go:432` 与 chunked 路径同样如此）。因此 int64/uint64 超过 2^53 时会损失精度，`Dataset.Read()` 也一样。
- 支持的过滤器只有 deflate(GZIP)、shuffle、fletcher32、LZF。SZIP(4)、N-bit(5)、Scale-offset(6) 与第三方过滤器会返回包裹 `ErrNotSupported` 的错误。
- 硬链接成环的文件在 `hdf5.Open` 阶段被拒绝；无 SWMR、无并行 I/O；一个 `*File` 被多个 goroutine 使用需要调用方自行同步。
- 软链接在组加载时被跳过，不在 `Children()` 中出现。

工具链约束：MyGo v0.10.0 的 `go.mod` 要求 Go 1.27.3，因此构建机需要 Go 1.27+ 或允许 `GOTOOLCHAIN=auto` 自动下载工具链；本机 `E:\gocode` 的模块缓存不可写，验证阶段须把 `GOMODCACHE` 指向工作区内目录。

## Goals / Non-Goals

**Goals:**

- 把「文件访问 → 对象模型 → 视图」切成三层，使 HDF5 细节不泄漏到 UI 层，UI 层不感知库的具体 API。
- 让所有视图成为可无窗口测试的纯函数，用 `ui.NewTester` 断言界面文本，无需 GPU 或真实文件对话框。
- 大文件下的内存占用与数据集总大小解耦：表格只读可见切片，图像视图只读降采样后所需的行。
- 在库能力边界处提供确定的降级行为，而不是让错误冒泡成空白界面。

**Non-Goals:**

- 不设计写入路径、缓存淘汰策略或并发读调度；单文档、单线程读已满足只读浏览。
- 不为 HDF4、SZIP、N-bit、Scale-offset 设计替代解码器；这些只做「如实报告不支持」。
- 不做插件化视图扩展点；三个视图在编译期固定。
- 不做 macOS/Linux 的适配验证；MyGo 的菜单在非 macOS 平台行为一致，但不在本次验收范围。

## Decisions

### 决策 1：分层为 `internal/hdf5`、`internal/view`、`main.go` 三层

`internal/hdf5` 定义应用自己的对象模型（`Node`、`DatasetInfo`、`AttributeInfo`），把 `scigolib/hdf5` 的类型转换成本层的值类型后再交给上层；`internal/view` 只依赖这个模型与 MyGo；`main.go` 负责窗口、菜单、对话框与状态装配。

**理由**：`scigolib/hdf5` 的 `Object` 接口只有 `Name()`，`*Group` 与 `*Dataset` 的具体能力差异很大，直接在视图中做类型断言会让 UI 层与库耦合。包一层值模型后，视图测试可以构造假数据，不必准备真实 HDF5 文件。

**备选**：直接在视图中持有 `*hdf5.File` 并断言。被否，因为视图测试需要真实文件，且库升级会波及全部 UI 代码。

### 决策 2：对象树用 `ui.Outline` + `Generation`，而不是递归构建

树用 `ui.Outline[NodeID]`，`roots` 返回根节点，`children` 按需返回某个组的直接成员。`OutlineState.Generation` 在文件打开/关闭时自增，使框架复用行而不必每帧重走整棵树。

**理由**：`OutlineState` 的注释明确说明 `Generation` 非零时只在 `Open`、`roots`、`Generation` 变化时重走 open 项，这正是「展开一万个成员的组仍要响应」所需的语义；`Open` 是 `Selection[NodeID]`，天然保存展开状态。

**备选**：用 `ui.Tree` + 递归 `TreeItem`。被否，因为 `TreeItem` 的 `children` 是 `func()`，每帧都会构建全部子节点，深层或宽层文件会退化。

节点身份用 `NodeID`（对象地址 `uint64` + 类型标记），而不是路径字符串：同一对象可能被多个链接指向，地址是稳定的，路径不是。

### 决策 3：数据表格采用「窗口化切片」，行数按可见区推算

表格的 `n` 参数传当前页的行数（默认 100 行 × 50 列，可由用户调整），单元格回调只访问已读入内存的切片；滚动到页边界时触发一次 `ReadSlice` 读取下一页。表格不绑定数据集的行数，数据集维度只用于计算页数与越界检查。

**理由**：`ui.Table` 的 `cell func(row, col int)` 会在布局时对可见行调用，但它没有「可见行范围」回调，无法把虚拟化交给框架。把「页」作为表格的 `n`，让框架的滚动发生在页内，翻页由应用控制，既保证内存上界，也避免依赖未公开的可见区信息。

**备选 A**：整份 `Read()` 后表格绑定全量。被否，十亿元素的数据集会直接触发读取上限或耗尽内存。

**备选 B**：用 `ReadHyperslab` 的 stride 做降采样预览。保留为图像视图的手段（见决策 5），但不用于表格——表格必须显示真实值。

### 决策 4：切片读取串行化在应用层，用「读请求 + 结果」两段式

UI 线程发起读取请求，工作 goroutine 执行 `ReadSlice`，结果通过 `win.Update(func(){...})` 写回应用状态。同一时刻只有一个读取在飞；新的选择会取消尚未开始的请求（用 generation 计数器比对，丢弃过期结果）。

**理由**：`scigolib/hdf5` 的一个 `*File` 并发使用需调用方同步，因此不能并发读；而 `win.Update` 是 MyGo 提供的、把状态变更搬回主线程的唯一正确方式（`Invalidate` 只重绘不改状态）。用 generation 比对而不是 channel 取消，是因为库的读取没有 context 参数，无法中途打断，只能丢弃结果。

**备选**：在主线程同步读取。被否，一个慢读取会冻结整个窗口，违反 `viewer-shell` 中「错误提示不阻塞浏览」与 `dataset-data-view` 中「可感知的延迟内显示」的要求。

### 决策 5：图像视图用 `ui.Bitmap` 承载，值域映射在应用层完成

二维数值数组经应用层映射为 `image.Gray` 或 `image.RGBA`（伪彩色），再经 `ui.NewBitmap(img)` 交给 `ui.Image`。值域默认取当前切片的最小/最大值，用户可覆盖；色标用一组 `ui.Text` + 背景色块绘制，不引入图表库。

**理由**：`ui.Bitmap` 是框架内的位图源，`Pixelated()` 保证放大时不被插值糊掉，适合数值场。伪彩色映射自己实现（20 行查表），避免为一个功能引入依赖。

**备选**：用 `ui.Chart` 的热力图。被否，`ui.Chart` 面向的是折线/柱状类图表，十万级像素不是它的设计目标。

### 决策 6：只读保证靠「依赖裁剪 + 代码审查」，而非运行时开关

`internal/hdf5` 包只暴露读取入口；`scigolib/hdf5` 的 `CreateForWrite`、`OpenForWrite` 及任何 `Write*` 方法不被调用，并通过一个包内测试（`go vet` + 源码扫描 `grep`）断言仓库中不出现这些标识符。文件以 `hdf5.Open` 打开，该函数内部用 `os.Open`（只读）语义读取。

**理由**：`hdf5.Open` 已经是只读路径，写入能力在库中是独立的函数集合，不共享入口。用源码级断言可以在 CI 中持续验证，而不必引入自定义的文件系统包装。

**备选**：用 `fs.FS` 包装成只读文件系统再交给库。被否，库接受的是路径而非 `fs.FS`，包装需要改库或复制其 `Open` 逻辑。

### 决策 7：错误用哨兵错误分流，视图展示分类后的消息

`internal/hdf5` 把库返回的错误经 `errors.Is` 归入四类：`ErrNotHDF5`、`ErrCorrupt`、`ErrUnsupported`、`ErrTooLarge`。UI 层按类别选择消息模板与是否保留当前文档。

**理由**：库已提供 `ErrCorrupt`、`ErrNotSupported`、`ErrLimit` 等哨兵，直接透传会让用户看到英文内部错误；而「整个文件打不开」与「某个数据集读不了」的处理策略不同（前者回到空状态，后者保留文档并在视图内标注），必须分类。

### 决策 8：菜单与快捷键用 `mygo.Menu` + `Accelerator`，不自行处理键盘

「文件/视图/帮助」三个菜单通过 `(*Window).SetMenu` 装配，`MenuItem.Accelerator` 声明 Ctrl+O、Ctrl+W、Ctrl+Shift+E。视图内不重复实现这些快捷键，避免与原生菜单冲突。

**理由**：`SetMenu` 在 Windows 上映射为原生菜单栏，`Accelerator` 由框架处理分发；自行在 `c.Key(...)` 层拦截会与原生菜单重复触发。

### 决策 9：精度损失显式标注，而不是假装精确

数据视图在格式化时，若元素类型是 8 字节整数且 `|v| > 2^53`，在单元格值后加 `≈` 标记，并在状态栏/提示中说明原因（库的读路径统一转换为 float64）。这一行为已写入 `dataset-data-view` 的 spec。

**理由**：库的 `ReadSlice` 只返回 `[]float64`，绕过它需要自行解析 datatype 与存储布局，属于重写库的一半。诚实地标注限制比静默显示错误值更符合科研工具的要求；后续若要精确显示，可在 `internal/hdf5` 中增加按 datatype 自行解码的路径，而不影响 UI。

## Risks / Trade-offs

- [库的 `ReadSlice` 只返回 `[]float64`，uint64/int64 大值丢精度] → 用 `≈` 标记并在元数据中标注；spec 已声明该限制。后续增强点在 `internal/hdf5` 内，UI 无需改动。
- [SZIP / N-bit / Scale-offset 数据集无法解码] → `internal/hdf5` 把这类错误映射为 `ErrUnsupported`，数据视图显示「该过滤器不受支持」，元数据仍完整可读；对象树不因此隐藏节点。
- [硬链接成环的文件可能无法打开或无限展开] → 实测（`h5py_cycle.h5`，`/a/back` 硬链接回 `/`）：解析库的 `visitedBTrees` 守卫让文件**正常打开**，回环成员被当作空组，树不会无限展开。`internal/hdf5` 仍保留 `object graph cycle` 文本到 `ErrUnsupported` 的映射，覆盖库在其它路径上拒绝成环文件的情况；两种表现都满足「不挂死、不显示错误数据」的要求。
- [`ui.Table` 没有可见行范围回调，窗口化只能按页做] → 页大小可配置；翻页延迟由「读取中」状态覆盖。若后续框架提供可见区回调，只需替换 `internal/view/table.go` 的页计算。
- [MyGo 要求 Go 1.27.3，构建机 Go 版本可能更旧] → `go.mod` 声明 `go 1.27.3` 与 `toolchain` 指令；README 记录 `GOTOOLCHAIN=auto` 的用法。本机 `E:\gocode` 不可写的问题用 `GOMODCACHE` 覆盖，不影响最终用户。
- [视图按帧重建，误把 build 值存进结构体会在运行时静默失效] → 所有跨帧状态集中在 `App` 结构体与 `ListState`/`OutlineState`；CI 中运行 `mygo vet`（该命令专门检查这类错误）。
- [读取在工作 goroutine 中执行，文件可能在读取中途被外部删除] → 读失败映射为 `ErrCorrupt` 类，视图标注该切片不可用，文档保持打开；不尝试自动重开文件。
- [单文档模型限制了对照浏览] → 明确接受：proposal 已把「同时只保留一个打开的文档」写入 spec，多标签页留待后续变更。

## Migration Plan

全新项目，无迁移。落地顺序：先建 `go.mod` 与三层骨架并用一个最小 `main.go` 打通窗口，再实现 `internal/hdf5` 的对象模型与只读打开，然后按「树 → 元数据 → 表格 → 图像」的顺序逐个接入视图，最后补菜单、拖放与命令行参数。回滚策略：本变更为纯新增，删除目录即可回到空仓库状态。

## Open Questions

- 表格的默认页尺寸（行 × 列）取多少最合适？取决于目标用户的数据形态，可在实现时用 100×50 起步，之后按实测调整，不影响 spec 与任务拆分。
- 伪彩色的默认色带选哪一种（viridis 风格还是灰度）？属视觉偏好，实现时先给灰度，色带作为可选项加入。

**已决定**：表格默认页取 100 行 × 50 列（`defaultPageRows`/`defaultPageCols`），实测在 100000×10000 的数据集上首屏只读一页、翻页无重复读取。伪彩色先只提供灰度（`grayscale`），值域可手动覆盖；彩色色带留作后续增强。
