# 验收走查记录

本文件记录变更 `add-hdf5-viewer` 的逐场景走查结果。每个 scenario 一条结论，取值为：

- **通过**：在真实夹具上观察到 spec 要求的可观察行为。
- **受库限制**：解析库（`github.com/scigolib/hdf5 v0.15.0`）不提供该信息或该能力，应用按 spec 允许的降级方式处理；限制项与 `design.md` 的 Risks 对应。
- **仅代码级**：该 scenario 依赖原生窗口或系统对话框，headless 测试无法产生真实交互，只能验证代码路径与文案。

走查日期 2026-11-24。夹具全部位于 `internal/hdf5/testdata/`，由 `gen.go`（用 scigolib 写入 API）与 `gen_h5py.py`（用 h5py 3.16.0 / HDF5 2.0.0）生成。

## 夹具清单

| 文件 | 大小 | 来源 | 覆盖点 |
| --- | --- | --- | --- |
| `basic.h5` | 61336 | scigolib | 100×50 float64、属性、嵌套组、空组 |
| `compressed.h5` | 23232 | scigolib | chunk(16,16) + shuffle + deflate(6) |
| `types.h5` | 6696 | scigolib | int32/int64 大值/uint64 大值/枚举/复合 |
| `strings.h5` | 5624 | scigolib | 定长字符串 |
| `scalar.h5` | 6016 | scigolib | `[1]` 形态、三维 `/cube` |
| `big.h5` | 74556 | scigolib | 2000×100 chunk(100,100) + gzip(1) |
| `h5py_basic.h5` | 56765 | h5py | 与 basic.h5 同构的独立实现、`/growable` 无限维 |
| `h5py_types.h5` | 10568 | h5py | **真 rank 0 标量**、变长字符串、定长 S8、枚举、复合 |
| `h5py_unsupported.h5` | 4054 | h5py | scaleoffset 过滤器 |
| `h5py_matlab.h5` | 5152 | h5py | 512 字节 user block、`#refs#`、`MATLAB_class` |
| `h5py_wide.h5` | 3856200 | h5py | 单个组内 10000 个数据集 |
| `h5py_misc.h5` | 411968 | h5py | 10×100×50 三维、填充值、超长字符串 |
| `h5py_cycle.h5` | 2160 | h5py | `/a/back` 硬链接回 `/` |
| `h5py_softlink.h5` | 4544 | h5py | 软链接 `/link` → `/real` |
| `h5py_corruptchunk.h5` | 4194 | h5py（后处理损坏） | zlib 流头之后 8 字节被 XOR 破坏 |
| `hdf4_like.hdf` | 2048 | 手工 | HDF4 签名 `0e 03 13 01` |
| `empty.h5` | 0 | 手工 | 空文件 |
| `truncated.h5` | 256 | 手工 | `basic.h5` 前 256 字节 |
| `not_hdf5.txt` | 25 | 手工 | 文本文件 |

未能生成的夹具：**SZIP**（本机 libhdf5 未编译 SZIP，h5py 报 `Compression filter "szip" is unavailable`）、**N-bit**（h5py 无该过滤器封装）、**未知第三方过滤器**（h5py 的 `register_filter` 签名与所需回调不匹配，注册失败）。这三项用代码级断言覆盖，见下文。

## hdf5-file-access

| Scenario | 结论 | 证据 |
| --- | --- | --- |
| 打开有效的 HDF5 文件 | 通过 | 打开 `basic.h5` 后左树出现根组，状态栏为 `basic.h5 / 2 个成员 / 59.9 KB` |
| 打开后文件内容保持不变 | 通过 | 读取 `/matrix` 首屏前后：md5 均为 `e37a1efdcc7bf7f13659313b0cbb89a8`，size 均 61336，mtime 均为 `2026-11-24 14:10:20.5805873 +0800` |
| 拒绝写入路径 | 通过 | `readonly_test.go` 用 `go/parser` 扫描全仓非测试源码，禁止 `CreateForWrite`/`OpenForWrite`/`CreateCompound`/`CreateSoftLink`/`CreateHardLink`/`CreateExternalLink`/`WriteRaw`/`WriteAttribute`/`DeleteAttribute` 等 11 个符号；注入 `libhdf5.CreateForWrite` 后测试报 `..\..\internal\hdf5\zz_inject.go:6:9: CreateForWrite` 并失败。UI 上不存在「保存」「另存为」「编辑数值」入口 |
| 文件不是 HDF5 | 通过 | `不是 HDF5 文件：internal\hdf5\testdata\not_hdf5.txt 中找不到 HDF5 签名`；`errors.Is(err, ErrNotHDF5)` 为 true |
| 文件被截断或损坏 | 通过 | `HDF5 文件已损坏：superblock read failed: truncated file: end-of-file address 61336 is beyond the file size 256`，`errors.Is(err, ErrCorrupt)` 为 true；与「不支持的特性」（`ErrUnsupported`）分属不同哨兵 |
| 文件不存在或无权读取 | 通过 | 不存在：`打开文件失败：GetFileAttributesEx ...\nope.h5: The system cannot find the file specified.`；目录：`打开文件失败：internal/hdf5/testdata 是目录`。两种情况都不改变当前已打开的文件 |
| 数据集使用了无解码器的过滤器 | 通过（scaleoffset 实测；SZIP/N-bit 仅代码级） | `h5py_unsupported.h5` 的 `/scaleoffset`：树仍显示该数据集，元数据完整显示 `布局=分块`、`过滤器=scaleoffset`；数据视图标题为「该过滤器不受支持，无法解码：」，正文错误为 `HDF5 特性不受支持：failed to read chunks: ... required filter 6 (Scale-Offset) is not supported: unsupported filter ID: 6: ...`，`errors.Is(err, ErrUnsupported)` 为 true。SZIP(4)/N-bit(5) 走同一条 `ErrUnsupported` 映射（`internal/hdf5/document.go` 的 `classify`），但无夹具，故只做代码级确认 |
| HDF4 文件 | 通过 | `不是 HDF5 文件：检测到 HDF4 签名，仅支持 HDF5`，`errors.Is(err, ErrNotHDF5)` 为 true；应用不尝试解析 |
| 组之间存在硬链接环 | **与 design 预期不符，见下方说明** | `h5py_cycle.h5` 的 `Open` **成功**，树链为 `/` → `/a` → `/a/back`，`/a/back` 的子节点为空，不无限展开 |
| 切换到另一个文件 | 通过 | 先开 `basic.h5`，再开 `compressed.h5`：旧 `*Document` 的 `Root()` 返回 `nil`（句柄已关闭），树与状态栏全部切换为 `compressed.h5` |
| 关闭应用时释放句柄 | 通过（真机） | `build/windows-amd64/GoHdfView.exe .probe/handletest.h5` 运行中执行 `mv` 报 `The process cannot access the file because it is being used by another process.`；`taskkill /F /IM GoHdfView.exe` 后同一 `mv` 成功 |
| 超大切片请求被拒绝 | 通过 | 把上限设为 1024 字节后请求 `10×100×50`：`读取量超过上限：请求 50000 个元素（400000 字节），上限 1024 字节`，`errors.Is(err, ErrTooLarge)` 为 true |

### 硬链接环：实测行为与 design.md 的差异

`design.md` 的 Risks 写的是「硬链接成环的文件在 `Open` 阶段即被拒绝」。实测并非如此：

- 解析库的环检测（`.probe/mod/github.com/scigolib/hdf5@v0.15.0/group.go:649-658` 的 `loadObject`）只在**同一加载路径上重复访问同一地址**时报 `object graph cycle at address 0x%X`。`h5py_cycle.h5` 的 `/a/back` 指向 `/`，而 `/` 在加载 `/a/back` 时已经出栈，因此不触发该守卫。
- 解析库另有一处 `visitedBTrees` 守卫（`group.go:539-547`），对已访问的 B-tree 地址直接返回空子节点列表——这才是本例实际生效的机制。

结论：**打开成功，回环处的子节点为空，树不会无限展开**，用户看到的是 `/a/back` 展开后没有成员。这满足 spec 的实质要求（「不是无限展开对象树」），但没有达到 design 描述的「Open 阶段拒绝并提示原因」。

`internal/hdf5` 保留了 `classify` 里对 `"object graph cycle"` 文本的映射（→ `ErrUnsupported`），用于覆盖解析库确实抛错的那类文件（自引用符号表节点，见该库 `FuzzOpenFile` 注释）。此处按实测记录，不修改 design.md 的历史决策；后续若要严格对齐，可让 `internal/hdf5` 在遍历时自行记录已访问的组地址并在发现回环时把该节点标为 `KindSoftLink` 并附提示。

## object-tree

| Scenario | 结论 | 证据 |
| --- | --- | --- |
| 展开根组 | 通过 | `basic.h5` 展开根后按名称列出 `matrix`（`▦ [100×50]`）与 `results`（`▣`），缩进区分层级 |
| 空组 | 通过 | `/results/empty` 的 `children` 返回 `nil`，树不画箭头；选中后元数据显示 `组信息 / 对象地址（解析库未提供地址）/ 成员数 0 / 属性数 0`，状态栏 `0 个成员` |
| 区分组与数据集 | 通过 | 组用 `▣`、数据集用 `▦`、命名类型用 `◈`；数据集标签后附维度概要 `[100×50]`、`[标量]` |
| 软链接与外部链接 | **受库限制** | `h5py_softlink.h5` 的根子节点为 `[grp real]`——软链接成员 `link` 根本不出现在 `Children()` 中。解析库在 `loadModernGroup` 里对非硬链接 `continue` 跳过，未把链接种类暴露给调用方。应用侧保留了 `KindSoftLink`/`KindExternalLink` 与 `↗`/`⧉` 字形，一旦上游暴露即可显示；当前无法通过任何公开 API 观测到软链接 |
| 展开后选中兄弟节点 | 通过 | 展开 `/results` 并选中 `/results/run-1` 后行数为 5；切到兄弟 `/matrix` 后行数仍为 5，`/results` 保持展开 |
| 全部展开与全部折叠 | 通过 | `basic.h5` 全部展开后行数 6（含 `/results/run-1/temperature`），全部折叠后行数 1（只剩根） |
| 选中数据集 | 通过 | 选中 `/matrix` 后 `Current.Path == "/matrix"`，右侧元数据切到数据集信息，状态栏 `共 5000 个元素` |
| 选中组 | 通过 | 选中 `/results` 后元数据显示「组信息」，「数据」标签置灰并自动落到「元数据」 |
| 用键盘在树中移动 | 通过 | 树获得焦点后按 `Down`：`/matrix` → `/results` |
| 显示完整路径 | 通过 | 选中 `/results/run-1/temperature` 时面包屑显示 `/ › results › run-1 › temperature` |
| 点击路径分段 | 通过 | 点击面包屑中的 `results` 后 `Current.Path` 变为 `/results` |
| 展开含一万个成员的数据集列表 | 通过 | `h5py_wide.h5` 的 `/wide`：`Children()` 返回 10000 个子节点，单次耗时 323.5ms；界面上「全部展开」耗时 23.6ms（`loadCalls=2`，展开预算 20000 未触顶），`Tree.Rows()` 为 10002 |

## dataset-metadata

| Scenario | 结论 | 证据 |
| --- | --- | --- |
| 二维数据集 | 通过 | `/matrix`：秩 2、维度 `[100, 50]`、最大值 `[100, 50]`、元素总数 5000 |
| 标量数据集 | 通过 | `h5py_types.h5` 的 `/scalar`：秩 0、维度空、元素总数 1 |
| 有符号 32 位整数 | 通过 | `/int32`：`整数 · 4 字节`、字节序「小端」、符号性「有符号」 |
| 变长字符串 | 通过 | `/names`：`字符串 · 16 字节 · 变长` |
| 复合类型 | 通过 | `/compound`：类别「复合」，逐成员列出 `id（0 字节处）`、`value（8 字节处）` |
| 分块压缩数据集 | 通过 | `compressed.h5` 的 `/compressed`：布局「分块」、分块尺寸 `[16, 16]`、过滤器按应用顺序 `1. shuffle` / `2. deflate (level 6)` |
| 连续存储且无压缩 | 通过 | `strings.h5` 的 `/names`：布局「连续」、过滤器「无」 |
| 未知的第三方过滤器 | **仅代码级** | 无法构造真实夹具（h5py 注册未知过滤器失败）。用构造的 `hdf5.FilterInfo{ID: 32017, ClientData: []uint32{3, 9}}` 断言 `Describe()` 输出含 `32017`、`未知过滤器`、`3, 9`，即 `filter(32017)（未知过滤器，参数 3, 9）` |
| 列出数据集属性 | 通过 | `/matrix` 的属性区列出 `units`（字符串 2 字节，值 `K`）与 `scale_factor`（浮点 8 字节，值 `1.5, 2.5`） |
| 属性的值为数组 | 通过 | `scale_factor` 显示为 `1.5, 2.5` |
| 对象没有属性 | 通过 | `/results/empty` 的属性区显示「无属性」 |
| 双击属性值 | 通过 | 点击属性值 `K` 前后控件文本数均为 39，无输入框出现——属性区没有编辑入口 |

## dataset-data-view

| Scenario | 结论 | 证据 |
| --- | --- | --- |
| 二维数据集 | 通过 | `/matrix`：列头 0–6…、行号 0–17…、`第 1 / 1 页（共 100 行）`，行列号从 0 开始 |
| 一维数据集 | 通过 | `/growable`（长度 5）与 `big.h5` 的 2000 行数据集均为单列（列头「值」）+ 分页 |
| 三维数据集 | 通过 | `h5py_misc.h5` 的 `/volume`（10×100×50）：工具栏「固定前导维」下拉值为 `0`，表格为 100 行 × 50 列，`第 1 / 1 页（共 100 行）` |
| 标量数据集 | 通过 | `h5py_types.h5` 的 `/scalar` 显示 `42.5`（**该场景曾暴露真实缺陷，见下方说明**） |
| 十亿元素的二维数据集 | 通过（真实夹具 2000×100；100000×10000 用假读取器） | 真实：`big.h5` 的 `/wide` 首屏只读一页，`第 1 / 20 页（共 2000 行）`、`共 200000 个元素`。假读取器：`table_test.go` 的 `TestTableHugeDataset` 断言 100000×10000 的首屏请求为 `start=[0 0] count=[100 50]`，页数为 `第 1 / 1000 页（共 100000 行）`，内存不随总量增长 |
| 滚动到未读取的区域 | 通过 | `TestTablePaging`：首页请求 `start=[0 0]`，翻到第二页请求 `start=[100 0]`，翻回第一页**不再发起读取**（命中页缓存） |
| 浮点数 | 通过 | float32 `/float32` 显示 `1.5`、`-2.25`、`0`；非有限值格式化为 `NaN`/`+Inf`/`-Inf`（`TestFormatNumber` 覆盖） |
| 超出精确表示范围的整数 | 通过 | `/int64_big`：`4611686018427387904 ≈`、`-4611686018427387904 ≈`、`9007199254740992`（2^53 本身不加标记，其上方才加） |
| 无符号 64 位整数不显示为负数 | 通过 | `/uint64_big`（值 1<<63 与 2^64-1）：显示 `9223372036854775808 ≈` 与 `18446744073709551616 ≈`，均为非负 |
| 空值与填充值 | 通过 | `/fillonly`（fillvalue = -999，只写了 `[0:2, 0:2]`）：未写区域显示 `-999`，读取无错误 |
| 变长字符串数据集 | 通过 | `/names` 显示 `alpha`/`beta`/`gamma`/`delta`；`/longtext` 的 300 字符串被截断为 200 字符 + `…`，点击单元格后「单元格全文」面板显示完整 300 字符 |
| 复合类型数据集 | 通过 | `/compound`：列头 `id`、`value`，三行 `1/1.5`、`2/2.5`、`3/3.5` |
| 枚举类型数据集 | 通过 | `/enum_days`：单元格显示成员名 `周一`/`周四`/`周日`/`周二`；元数据给出名称-数值对应表 |
| 切换到图像视图 | 通过 | `/matrix` 切到「图像」：显示 `100 × 50（步长 1×1，共 5000 像素）`、灰度位图、色标 `0 … 99.49` |
| 调整值域 | 通过 | 把最小值改为 `5` 后位图重建（`Image.key` 变化）、色标起点变为 `5`；「重置值域」清空两个输入 |
| 非数值数据集 | 通过 | 字符串数据集 `/names` 切到「图像」：显示「图像视图不可用」+「该数据集不是数值类型（字符串），无法显示为图像」 |
| 分块数据中存在损坏的分块 | 通过 | `h5py_corruptchunk.h5` 的 `/chunked`：65 处单元格显示「不可用」，同时出现「该区域读取失败，单元格显示为不可用：」与「详情」入口；其余单元格正常 |

### 走查中发现并修复的真实缺陷

1. **rank 0 标量显示「（无值）」**（`internal/hdf5/document.go` 的 `ReadSlice`）。原因：标量没有可切片的维度，`ReadSlice(start, count)` 收到空选择后返回空结果。修法：`len(shape) == 0` 时改走 `node.ds.Read()` 并照常检查读取上限。回归断言在 `internal/hdf5/h5py_test.go` 的「标量」子测试。
2. **字符串数据集超过上限时只显示裸哨兵**。原因：`ensureTable` 直接返回 `hdf5.ErrTooLarge`。修法：新增 `tooLargeString`，提示「该数据集有 N 个元素，超过字符串视图上限 1048576；请改用元数据视图查看结构」。
3. **读取失败的提示不区分原因**。修法：`readFailure` 按 `errors.Is` 分三档文案（不受支持的过滤器 / 超过上限 / 其他失败）。
4. **`FormatShape` 把 `×` 截成单字节**（早期已修）。原因：`b.WriteByte('×')` 对多字节 UTF-8 只写首字节。修法：`b.WriteString("×")`。

## viewer-shell

| Scenario | 结论 | 证据 |
| --- | --- | --- |
| 启动应用 | 通过 | 空状态显示 `GoHdfView`、`打开 HDF5 文件或把文件拖入窗口` 与「打开…」按钮；截图 `docs/screenshots/empty-state.png` |
| 调整分栏宽度 | 通过 | 分栏宽度绑定 `app.split`（初值 320），改动后下一帧生效；`ui.Split` 的拖动由框架直接写回该指针 |
| 状态栏信息 | 通过 | 选中 `/matrix` 后状态栏为 `basic.h5 / /matrix / 共 5000 个元素 / 59.9 KB` |
| 菜单打开 | 仅代码级 | 菜单项「打开…」调用 `a.openDialog()`，其内部用 `mygo.Dialog.Open` 并带 `.h5/.hdf5/.he5/.mat` 过滤器；headless 环境 `tt.Menu()` 返回空，无法点击原生菜单 |
| 快捷键打开 | 仅代码级 | 「打开…」的 `Accelerator` 为 `CmdOrCtrl+O`；菜单结构与加速键字符串由 `TestMenuBuilds` 断言 |
| 拖放打开 | 仅代码级 | `win.OnFileDrop` 取 `e.Paths[0]` 调 `a.openFile`；拖放事件无法在 headless 下合成 |
| 命令行参数 | 通过（真机） | `./build/windows-amd64/GoHdfView.exe .probe/handletest.h5` 启动后 `Get-Process GoHdfView \| Select MainWindowTitle` 返回 `GoHdfView — shot.h5` |
| 切换到元数据视图 | 通过 | 点「元数据」标签后显示数据集/组信息；`TestTabsSwitch` 覆盖 |
| 选中不支持数据的对象 | 通过 | 选中组时「数据」标签置灰（`ui.TabsBase` + `Disabled(true)`），当前视图自动落到「元数据」 |
| 关闭文件 | 通过 | 「关闭文件」/`CmdOrCtrl+W` 调 `a.closeFile()`：句柄释放、树与视图清空、回到空状态文案 |
| 全部展开 | 通过 | 「全部展开」/`CmdOrCtrl+Shift+E` 调 `state.ExpandAll()`；`TestMenuExpandAll` 断言行数由 1 变为 6 |
| 关于 | 仅代码级 | 菜单项「关于」调 `a.showAbout()`，用 `mygo.Dialog.Message` 显示 `GoHdfView 0.1.0` 与 `github.com/scigolib/hdf5 v0.15.0`；原生对话框无法在 headless 下截图 |
| 数据集读取失败后继续浏览 | 通过 | 打开 `h5py_corruptchunk.h5` 后选中 `/chunked`（读取失败），再选中同文件的 `/chunked` 以外的对象仍可正常浏览；错误条可点「关闭提示」清除 |
| 打开损坏文件失败后保留原状态 | 通过 | 先开 `basic.h5`，再打开 `not_hdf5.txt`：`errMsg` 为 `不是 HDF5 文件：... 中找不到 HDF5 签名`，原文档仍可浏览；点「关闭提示」后 `errMsg` 清空 |

## 只读性与构建

- `internal/hdf5/readonly_test.go` 扫描全仓非测试源码，11 个写入符号零命中；注入验证见上文。
- 写入 API 仅出现在 `internal/hdf5/testdata/gen.go`（带 `//go:build ignore`，不参与构建）。
- `CGO_ENABLED=0` 构建通过。产物 `build/windows-amd64/GoHdfView.exe` 的构建元数据（`go version -m`）为 `go1.27.3`、`CGO_ENABLED=0`、`-tags=mygo_nowebview,mygo_nodownload,mygo_noinspector`、`-trimpath=true`；依赖只有 `purego`、`mygo`、`go-text/typesetting`、`scigolib/hdf5`、`golang.org/x/{image,net,text}`，无 HDF5 C 库、无 cgo。
- 加 `mygo_nowebview` 后产物从 11.8 MB 降到 11.2 MB，二进制里 `WebView2Loader.dll`、`msedgewebview2`、`CreateCoreWebView2` 三处字符串命中数均为 **0**（未加该标签时为非 0）；`hdf5.dll` 命中数为 0。
- PE 导入表（`debug/pe` 的 `ImportedSymbols`）只有 `kernel32.dll` 的 48 个符号，没有任何 HDF5 / WebView2 / VC 运行时 DLL。
- 真机启动复核：`GoHdfView.exe internal/hdf5/testdata/basic.h5` 后 `Get-Process GoHdfView` 的 `MainWindowTitle` 为 `GoHdfView — basic.h5`，进程常驻约 70 MB，`taskkill` 正常退出。
- 测试规模：根包 17 个、`internal/hdf5` 55 个、`internal/view` 52 个，共 124 个用例。

## 未覆盖项汇总

| 项 | 原因 | 处理 |
| --- | --- | --- |
| 软链接/外部链接节点显示 | 解析库在加载阶段跳过非硬链接，未暴露链接种类 | 记为受库限制；应用侧字形与 `NodeKind` 已就位 |
| 硬链接环在 `Open` 阶段拒绝 | 实测该文件打开成功、回环处不展开 | 记为与 design 描述不符，实质要求（不无限展开）已满足 |
| SZIP / N-bit 数据集 | 本机 libhdf5 无 SZIP，h5py 无 N-bit 封装 | 与 scaleoffset 共用 `ErrUnsupported` 映射，已由后者实测覆盖 |
| 未知第三方过滤器 | h5py 无法注册未知过滤器 | 用构造的 `FilterInfo` 做代码级断言 |
| 原生菜单点击、文件对话框、拖放事件 | headless 测试环境无原生窗口 | 只验证代码路径与文案；真机已用命令行参数与句柄占用验证关键路径 |
| 真实窗口像素截图 | 会话无桌面合成，`Screen` 抓图为全白，`PrintWindow` 报 `A required privilege is not held by the client` | 截图改用 `ui.Tester.Image()` 在 headless 下渲染，见 `docs/screenshots/` |

## larger-tree-rows-and-selection-highlight（2026-11-26）

本变更的行高、字号与选中高亮由 `internal/view/tree_test.go` 的 `TestTreeRowHeight`、
`TestTreeSelectedHighlight` 自动断言；以下为真机人工检查项。

| 检查项 | 期望 |
| --- | --- |
| 树行高度 | 每行 28px，行内图标与文字比旧版大一号 |
| 行内尺寸 | 名称字号 13.5、维度概要 11.5、链接标注 12；展开箭头 12×12、类型图标 13×13 |
| 选中行高亮 | 行底色 `#191E22`、类型图标琥珀色 `#E0A64A`、名称亮色 `#E7EAED` 且加粗、维度概要暖灰 `#8A7B5A` |
| 切换选中 | 原行恢复普通样式，同一时刻最多一行高亮 |
| 其余界面 | 侧栏筛选框、表格数据视图、面包屑、属性列表的字号与配色保持不变 |
