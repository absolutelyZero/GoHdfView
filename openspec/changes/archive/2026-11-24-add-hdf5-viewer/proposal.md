# Proposal

## Why

HDFView 是 HDF 生态里事实上的图形浏览器，但它由 Java 写成并依赖 JNI 绑定的原生 HDF 库：在 Windows 上要装 JRE、要匹配位数的 DLL，启动慢、包体大，而且把编辑能力一并带进来，对「只想看一眼数据」的场景是负担。用 MyGo（纯 Go、无 cgo 的原生 UI 框架）复刻一个**只读** HDF5 阅读器，可以得到一个单文件、免外部依赖、双击即开的查看工具；只读定位同时消除了误写科研数据的风险。

## What Changes

- 新增 Windows 桌面应用 `GoHdfView`，界面用 MyGo 的原生 UI（`ui` 包）从 Go 直接构建，不使用 WebView2、不引入 Java/JNI、保持 `CGO_ENABLED=0`。
- 打开并浏览 HDF5 文件（`.h5` / `.hdf5` / `.he5` / MATLAB v7.3 `.mat` 等）：以只读模式打开，文件句柄生命周期由应用管理。
- 左侧对象树展示文件内的组、数据集、命名类型与硬/软/外部链接；右侧工作区按选中对象切换「数据」「元数据」「属性」三个视图。
- 数据集数据视图：表格视图按行列分页，只按需读取当前可视区域的 hyperslab，支持大数组而不必整份载入；图像视图把二维（及可降维的三维）数值数组渲染为灰度或伪彩色位图。
- 元数据视图：dataspace 维度与最大值、datatype 类别/大小/字节序/符号性、存储布局（compact / contiguous / chunked）与分块尺寸、过滤器管线（deflate、shuffle、fletcher32、lzf 等）、对象地址。
- 属性视图：列出组与数据集上的属性名称、类型与值，只读展示。
- 明确的只读约束：不提供任何写回 HDF5 文件的代码路径；不引入 `hdf5` 库的写入 API（`CreateForWrite`、`OpenForWrite`、`Write*`），从依赖层面就杜绝写入。
- **明确不做**：HDF4 文件、SWMR、并行 I/O、数据/属性编辑、另存为、把数据导出为新 HDF5 文件、macOS 与 Linux 的交付验证。

## Capabilities

### New Capabilities

- `hdf5-file-access`: 打开、校验与关闭 HDF5 文件，只读保证，以及针对损坏文件、不支持特性（HDF4、SZIP/N-bit/Scale-offset 过滤器、硬链接环）的诊断与降级呈现。
- `object-tree`: 文件内对象层次的浏览——组、数据集、命名类型、链接的树形展示、展开折叠、选中与「定位到当前对象」，以及每层的对象计数。
- `dataset-metadata`: 组与数据集的元数据呈现——dataspace、datatype、存储布局、过滤器管线、属性清单与属性值。
- `dataset-data-view`: 数据集数值的呈现——表格视图的分页与按需切片读取、数值格式化、字符串与复合类型渲染，以及二维/三维数值数组的图像视图。
- `viewer-shell`: 桌面外壳——窗口与菜单、文件打开对话框与拖放、左右分栏与状态栏、视图切换、键盘快捷键与错误提示。

### Modified Capabilities

无。本仓库此前没有任何 spec，五个能力均为新增。

## Impact

- **新增代码**：`main.go`（应用入口与窗口装配）、`internal/hdf5`（只读文件访问与对象模型封装）、`internal/view`（树、元数据、表格、图像四个视图的 Go UI 构建函数），以及 `ui` 视图的无窗口测试。
- **依赖**：`github.com/egoist/mygo`（要求 Go 1.27.3 工具链，`CGO_ENABLED=0`）、`github.com/scigolib/hdf5` v0.15.0（纯 Go 只读/可写 HDF5 解析，本变更只用到读路径）。新增依赖均由 `go.mod` 固定版本。
- **构建**：`go tool mygo build` 产出 `build/windows-amd64/GoHdfView.exe`；不引入 Bun 与前端工具链（原生 UI 模板不含前端）。
- **数据安全**：应用对用户文件只做 `os.Open` 等价访问，不产生任何写入，不修改文件时间戳之外的元数据。
- **可测性**：所有视图为 `func(c *ui.Context)` 形式的纯函数，可用 `ui.NewTester` 在无窗口环境下断言，无需 GPU。
