# Tasks

## 1. 项目脚手架与工具链

- [x] 1.1 用 `go tool mygo init -template native` 生成原生 UI 骨架（`main.go`、`mygo.json`、`resources/icon.png`、`main_test.go`），把模块名改为 `gohdfview`；验证：`go build ./...` 通过，`go tool mygo build` 产出 `build/windows-amd64/GoHdfView.exe` 且双击可显示空窗口。
- [x] 1.2 在 `go.mod` 中固定 `go 1.27.3` 与 `toolchain go1.27.3`，加入 `github.com/egoist/mygo v0.10.0` 与 `github.com/scigolib/hdf5 v0.15.0`；验证：`go mod tidy` 无变更，`go list -m all` 显示两个依赖的固定版本，`CGO_ENABLED=0 go build ./...` 成功。
- [x] 1.3 在 `README.md` 记录构建前提（Go 1.27+ 或 `GOTOOLCHAIN=auto`、`GOMODCACHE` 可写目录、`CGO_ENABLED=0`）与 `go tool mygo dev` / `go tool mygo build` 的用法；验证：按 README 命令从干净 shell 复现构建成功。
- [x] 1.4 加入 `.gitignore`（`build/`、`.probe/`）并初始化 git 仓库，做首次提交；验证：`git status` 干净，`git ls-files` 不含 `build/` 与 `.probe/`。

## 2. 只读文件访问层（`internal/hdf5`）

- [x] 2.1 定义值类型对象模型：`NodeID`（对象地址 + 类型标记）、`NodeKind`（组/数据集/命名类型/软链接/外部链接）、`Node`、`DatasetInfo`（秩、维度、最大维度、元素数、`DatatypeInfo`、`LayoutInfo`、过滤器列表）、`AttributeInfo`；验证：`go vet ./internal/hdf5` 通过，包内为每个类型写 doc 注释。
- [x] 2.2 实现 `Open(path string) (*Document, error)` 与 `(*Document).Close()`，内部调用 `hdf5.Open` 并把库错误经 `errors.Is` 归入 `ErrNotHDF5` / `ErrCorrupt` / `ErrUnsupported` / `ErrTooLarge` 四类哨兵；验证：单元测试用「非 HDF5 字节流」「截断的 HDF5 文件」「不存在的路径」三个夹具断言分类正确，且 `Close` 可重复调用不 panic。
- [x] 2.3 实现 `(*Document).Root() *Node` 与 `(*Node).Children() ([]*Node, error)`，把 `*Group`/`*Dataset` 转成本层 `Node`，软链接在加载阶段已被库跳过则显式标注为不可展开；验证：对测试文件断言根组子节点数量与名称，且软链接不出现在 `Children()` 中（或在结果中标记为链接类型）。
- [x] 2.4 实现 `(*Node).Info() (*DatasetInfo, error)`，把 `Shape`、`MaxShape`、`Datatype`、`Layout` 的结果转成本层结构，`MaxShape` 中 `hdf5.Unlimited` 转为 `nil` 表示无限；验证：对 1D/2D/标量三个数据集断言秩、维度、元素数与无限维度表示。
- [x] 2.5 实现 `(*Node).Attributes() ([]AttributeInfo, error)`，把 `*core.Attribute` 的 datatype 与值转成可展示的类型名与字符串/数值表示，覆盖字符串、数值标量、数值数组三种形态；验证：对带 `units`（字符串）与 `scale_factor`（数值）属性的测试文件断言名称、类型与值。
- [x] 2.6 实现 `(*Document).ReadSlice(node *Node, start, count []uint64) (Slice, error)`，返回 `Slice`（含 `Values []float64`、`Shape []uint64`、`Truncated bool`），越界返回 `ErrOutOfBounds`，超出 `SetReadLimit` 的请求返回 `ErrTooLarge`；验证：单元测试断言正常切片的值与顺序、越界请求被拒绝、超过上限的请求被拒绝。
- [x] 2.7 实现 `(*Document).ReadStrings`、`(*Document).ReadCompound` 与枚举成员映射的封装，把 `ReadStrings`/`ReadCompound` 的结果转成本层可展示结构；验证：对字符串、复合、枚举三个测试数据集断言取值与成员名。
- [x] 2.8 编写测试夹具生成脚本 `internal/hdf5/testdata/gen.go`（`//go:build ignore`），用库的写入 API 一次性生成 `.h5` 夹具（含 deflate+shuffle 压缩数据集、分块数据集、软链接、属性、字符串、复合、枚举、标量、3D 数组），脚本产物提交到 `testdata/`；验证：脚本运行一次后 `testdata/` 出现全部夹具文件，且 `h5dump` 或库自身能重新打开它们。
- [x] 2.9 加入只读约束的自动化断言：测试扫描 `internal/`、`main.go`，断言源码中不出现 `CreateForWrite`、`OpenForWrite`、`WriteDataset`、`WriteAttribute` 等写入标识符；验证：故意加入一处 `hdf5.CreateForWrite` 调用时该测试失败，移除后通过。

## 3. 对象树与选中状态

- [x] 3.1 在 `internal/view/tree.go` 实现 `Tree(c *ui.Context, app *App)`，用 `ui.Outline[NodeID]` 展示根节点与按需加载的子节点，节点标签附类型图标与数据集维度概要（如 `[100×50]`）；验证：`ui.NewTester` 断言打开夹具后可见根节点名与一个数据集的 `[100×50]` 标签。
- [x] 3.2 在 `App` 中实现展开状态（`ui.OutlineState[NodeID]` 的 `Open`）与选中状态（`Selected *NodeID`），并让 `OutlineState.Generation` 在打开/关闭文件时自增；验证：测试展开一个组、选中同层另一个数据集后，断言该组仍处于展开状态。
- [x] 3.3 实现「全部展开」「全部折叠」操作，遍历当前已加载节点写入 `Open` 集合并自增 `Generation`；验证：测试调用两个操作后断言可见行数与根节点仍可见。
- [x] 3.4 在 `internal/view/breadcrumb.go` 实现当前对象完整路径的分段展示，点击某段时展开并选中对应祖先；验证：测试选中 `/results/run-3/temperature` 后断言界面包含三段文本，点击 `/results` 后断言选中项变为该组。
- [x] 3.5 实现键盘上下方向键在同层可见节点间移动选中项；验证：测试按 `Key` 模拟方向键后断言选中项与右侧视图同步变化。
- [x] 3.6 为含一万个成员的组加入性能守卫测试：构造一个 10000 成员的假 `Node` 树，断言 `Tree` 构建一次只访问可见行（用计数回调断言 `Children` 调用次数为常数级）；验证：`go test -run TestTreeLargeGroup` 通过且不超时。

## 4. 元数据与属性视图

- [x] 4.1 在 `internal/view/metadata.go` 实现数据集元数据区块：秩、维度、最大维度、元素总数；验证：测试选中 2D 数据集后断言界面出现 `[100, 50]` 与 `5000`。
- [x] 4.2 实现 datatype 区块：类别名、字节大小、字节序、符号性、定长/变长、填充方式，复合类型逐成员列出名称/偏移/类型；验证：测试分别选中 int32 数据集与复合类型数据集，断言对应文本。
- [x] 4.3 实现布局与过滤器区块：布局类别、分块尺寸、按顺序的过滤器列表（`shuffle`、`deflate (level 6)`），未知 ID 显示为 `filter(N)` 并标注未知；验证：测试选中压缩数据集断言过滤器顺序，构造未知 ID 的假 `LayoutInfo` 断言「未知过滤器」文本。
- [x] 4.4 在 `internal/view/attributes.go` 实现属性列表：名称、类型、值，数组值超长截断并提示可查看完整值，空属性显示「无属性」；验证：测试断言两个属性的名称与值、空属性对象的「无属性」文本。
- [x] 4.5 为组对象实现元数据视图（对象地址、成员数、属性），并为「选中组时数据标签不可用」提供状态字段；验证：测试选中组后断言数据标签处于不可用状态且元数据视图显示成员数。

## 5. 数据表格视图

- [ ] 5.1 在 `internal/view/table.go` 实现表格视图骨架：`ui.Table` 的 `n` 取当前页行数（默认 100 行 × 50 列），列头为列号、行首为行号，单元格从已加载的 `Slice` 取值；验证：测试选中 2D 夹具后断言首行文本包含列号与首个数值。
- [ ] 5.2 实现页状态与翻页：`App.TablePage`、页数由数据集维度与页尺寸计算，翻页触发一次 `ReadSlice`；验证：测试在 1000 行数据集上翻到第二页，断言请求的 `start`/`count` 正确且界面显示第 100 行起的数值。
- [ ] 5.3 实现秩适配：1D 单列、2D 行列矩阵、3D 及以上提供维度选择器（把除尾部两维外的维度固定到某下标）；验证：测试分别在 1D 与 3D 夹具上断言布局与维度选择器的可选值。
- [ ] 5.4 实现数值格式化：浮点按有效位显示、`NaN`/`+Inf`/`-Inf`、整数十进制，且对 8 字节整数超过 2^53 时追加 `≈` 标记；验证：表驱动单元测试覆盖 float32、float64、int32、int64 大值、uint64 大值、NaN，断言输出字符串。
- [ ] 5.5 实现字符串、复合、枚举三类数据的表格呈现：字符串按行截断、复合每成员一列、枚举显示成员名；验证：测试分别断言三种夹具的列头与单元格文本。
- [ ] 5.6 实现读取失败标注：当某次 `ReadSlice` 返回 `ErrUnsupported` 或 `ErrCorrupt` 时，受影响单元格显示不可用标记，视图其余部分正常，并提供查看错误详情的入口；验证：用假 `Document` 注入错误，断言表格仍渲染且出现不可用标记与详情按钮。
- [ ] 5.7 实现工作 goroutine 读取与过期结果丢弃：UI 线程发请求、后台执行、`win.Update` 写回，`Generation` 计数器丢弃过期结果；验证：测试注入一个延迟读取，切换选中对象后断言旧结果不覆盖新视图。

## 6. 图像视图

- [ ] 6.1 在 `internal/view/image.go` 实现二维数值到 `image.Gray` 的映射，值域默认取当前切片最小/最大，经 `ui.NewBitmap` 交给 `ui.Image`；验证：测试断言 2D 夹具切换图像视图后出现位图元素，且灰度极值与数值极值对应。
- [ ] 6.2 实现值域覆盖控件（最小/最大值输入）与色标绘制，值域变更后重新映射；验证：测试修改最小值后断言位图重建且色标文本更新。
- [ ] 6.3 为非数值数据集（字符串/复合）与秩不为 2 的数据集禁用图像视图并给出说明文本；验证：测试选中字符串数据集后断言图像视图不可用且出现说明。
- [ ] 6.4 为大尺寸 2D 数据集实现降采样读取（用 `ReadHyperslab` 的 stride 按目标像素数抽取），保证图像视图不整份载入；验证：在 10000×10000 假数据集上断言读取元素数不超过目标像素数的常数倍。

## 7. 桌面外壳集成

- [ ] 7.1 在 `main.go` 装配主窗口：`ui.Split` 左右分栏、右侧 `ui.Tabs` 三视图、底部状态栏（文件名、当前路径、元素总数）；验证：`ui.NewTester` 断言启动后出现空状态提示文本，打开夹具后状态栏含文件名。
- [ ] 7.2 用 `mygo.Menu` 装配「文件/视图/帮助」菜单与 `Accelerator`（Ctrl+O、Ctrl+W、Ctrl+Shift+E），菜单项调用与视图相同的操作函数；验证：`go tool mygo build` 后手动点击三个菜单项各一次，确认行为与快捷键一致。
- [ ] 7.3 用 `mygo.Dialog.Open` 实现文件选择对话框（过滤器 `.h5`、`.hdf5`、`.he5`、`.mat`）；验证：手动打开对话框确认过滤器生效，选中夹具后文件被加载。
- [ ] 7.4 实现拖放打开与命令行参数打开（`os.Args[1]`）；验证：手动把 `.h5` 拖入窗口确认加载；以 `GoHdfView.exe testdata\compressed.h5` 启动确认直接打开。
- [ ] 7.5 实现非模态错误提示：打开失败时保留当前文档，读取失败时保留视图并标注；验证：先打开有效夹具，再打开一个非 HDF5 文件，断言原文档仍可浏览且错误提示可关闭。
- [ ] 7.6 实现关闭文件与退出时释放句柄；验证：打开文件后执行「关闭文件」再重新打开同名文件成功，退出应用后文件未被占用（可被重命名）。

## 8. 集成验收

- [ ] 8.1 编写 `integration_test.go`：用 `ui.NewTester` 走完整路径——打开夹具 → 展开树 → 选中压缩数据集 → 断言元数据显示过滤器 → 切到数据视图断言首屏数值 → 切到属性视图断言属性；验证：`go test ./...` 全绿。
- [ ] 8.2 在真实夹具集（含 C 库、h5py、MATLAB v7.3 各写一份）上手工走查五个能力的每个 scenario，记录结果到 `docs/verification.md`；验证：文档中每个 scenario 都有「通过/受库限制」的结论，受限制项与 design.md 的 Risks 对应。
- [ ] 8.3 运行 `go tool mygo vet` 与 `go vet ./...`，修复所有 build 值误存与静态检查问题；验证：两条命令均无输出。
- [ ] 8.4 构建发布产物并验证为纯 Go：`CGO_ENABLED=0 go tool mygo build` 后检查 `build/windows-amd64/GoHdfView.exe`，用 `dumpbin /dependents`（或 `go version -m`）确认未链接 HDF5 DLL 与 WebView2；验证：依赖列表中只有系统 DLL。
- [ ] 8.5 在 README 补充功能说明、已知限制（SZIP/N-bit/Scale-offset 不支持、8 字节整数精度标注、硬链接环文件不支持、单文档）与截图；验证：README 的限制清单与 design.md 的 Risks 一一对应。

## Workflow follow-up

- 在评审要求满足后归档该变更（`openspec archive add-hdf5-viewer`）。
- 归档后校验 `openspec/specs/` 下生成的五个主 spec 内容完整、Purpose 非占位符。
