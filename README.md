# GoHdfView

用 Go 与 [MyGo](https://mygo.run) 复刻的 HDFView 只读阅读器：打开 HDF5 文件，浏览对象树、查看元数据与属性、以表格或图像方式查看数据集。

- **只读**：本项目不提供任何写入、编辑或另存为能力，也不会修改被打开的文件。写入 API 仅出现在测试夹具生成脚本 `internal/hdf5/testdata/gen.go`（`//go:build ignore`，不参与常规构建），并由 `internal/hdf5/readonly_test.go` 扫描源码来保证这一点。
- **纯 Go 实现**：不依赖 HDF5 C 库与 WebView2；支持 Windows 与 macOS（amd64 / arm64），只支持 HDF5（不支持 HDF4）。

## 与 HDFView 的差异

GoHdfView 侧重「快速打开并精确浏览大数据集」，与 HDFView 相比主要有两点体验差异：

| 关注点 | HDFView | GoHdfView |
| --- | --- | --- |
| 打开数据集 | 通常需要载入整个 dataset，大文件打开或跳转时容易卡顿 | 按页窗口化读取（默认 100 行 × 50 列），分页打开与翻页都不会引起卡顿，已读页还有缓存 |
| 定位数据 | 主要依靠滚动查找目标单元格 | 直接输入坐标快速定位，坐标长度等于数据集秩，超大秩用逗号分隔输入 |

此外，对象树采用惰性加载并支持方向键导航，上万成员的分组也不会卡住界面。

## 功能

| 区域 | 能力 |
| --- | --- |
| 对象树 | 按层次展示组、数据集与命名类型；数据集名后附维度概要（如 `[100×50]`）；惰性加载子节点，展开/折叠、全部展开（`Ctrl+Shift+E`）、全部折叠、方向键导航；上万成员的分组不会卡住界面。 |
| 元数据 | 数据空间（秩、维度、最大值、元素总数）、数据类型（类别、元素大小、字节序、符号性、字符串长度与填充、复合成员、枚举成员表）、存储布局（紧凑/连续/分块、分块尺寸、按应用顺序列出的过滤器及参数）。 |
| 属性 | 列出对象的全部属性及其类型与值；只读展示，双击不会进入编辑。 |
| 数据视图 | 表格按页窗口化读取（默认 100 行 × 50 列）；1D 单列、2D 矩阵、3D 及以上固定前导维；行号列号从 0 开始；底部左侧为行方向（上下）分页，右侧为列方向（左右）分页；中部可按坐标快速定位（坐标长度等于数据集秩，超大秩用逗号分隔输入）；浮点非有限值显示 `NaN`/`+Inf`/`-Inf`；枚举显示成员名；复合数据集每个成员一列；变长字符串可点击查看被截断的全文。 |
| 图像视图 | 二维数值数据集渲染为灰度图，带值域输入与色标；大数组按步长降采样（`ReadStrided`），读取量只与像素数有关。非数值或非二维数据集会说明不可用原因。 |
| 外壳 | 左树 / 右工作区 / 底部状态栏；菜单与快捷键（`Ctrl+O` 打开、`Ctrl+W` 关闭、`Ctrl+1/2/3` 切视图）；系统文件对话框、拖放打开、命令行参数打开；错误提示非模态且可关闭。 |

手工走查记录（夹具清单、逐 scenario 结论、受库限制项）见 [`docs/verification.md`](docs/verification.md)。

## 构建前提

| 要求 | 说明 |
| --- | --- |
| Go 1.27.3 | `go.mod` 固定 `go 1.27.3` 与 `toolchain go1.27.3`。本机装的是更旧的 Go 时，`GOTOOLCHAIN=auto`（默认值）会让 Go 自动下载并切换到 1.27.3。 |
| `CGO_ENABLED=0` | 纯 Go 构建，不依赖 HDF5 C 库。 |
| 可写的 `GOMODCACHE` / `GOPATH` / `GOCACHE` / `TMP` | 默认缓存与临时目录不可写时（常见于受限的 Windows 账户或沙箱），需要指向可写目录。 |

支持 **Windows amd64** 与 **macOS（amd64 / arm64）** 三个平台产物，并支持 **HDF5**（不支持 HDF4）。macOS 产物由 GitHub Actions 在打标签时自动构建并发布，也可按下方命令本地构建。

## 从干净 shell 构建

下面是一份可直接复制的完整流程。第 1 步只在默认缓存目录不可写时才需要，路径换成任意可写目录即可。

```bash
# 1) 可选：把 Go 的缓存与临时目录指到可写位置
#    Windows 上必须用绝对路径（D:\... 或 C:\...）。
#    MSYS/Git Bash 里的 /d/... 形式 Go 不接受，会报 "GOMODCACHE entry is relative"。
export GOMODCACHE='D:\path\to\repo\.probe\mod'
export GOPATH='D:\path\to\repo\.probe\gopath'
export GOCACHE='D:\path\to\repo\.probe\gocache'
export TMP='D:\path\to\repo\.probe\tmp'
export TEMP="$TMP"

# 2) 拉取依赖
go mod download

# 3) 编译检查与测试
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...

# 4) 打包原生桌面应用
#    mygo_nowebview：本项目只用原生 UI，不需要 WebView2 运行时；
#    加上它后产物小约 0.6 MB，二进制里也不再含 WebView2 的加载代码与字符串。
GOFLAGS=-tags=mygo_nowebview CGO_ENABLED=0 go tool mygo build -skip-installer
```

产物在 `build/windows-amd64/GoHdfView.exe`，可直接双击运行，或从命令行传入文件路径：

```bash
./build/windows-amd64/GoHdfView.exe path\to\file.h5
```

`-skip-installer` 用来跳过 Windows 安装包的生成；不需要安装包时加上它，可以避免下载 NSIS 到 `%LOCALAPPDATA%\mygo`。

### 在 macOS 上构建

macOS 无需 cgo 或额外工具链，可直接交叉编译两个架构（在 macOS 或任意能运行 Go 的机器上均可）：

```bash
# Intel（amd64）
GOFLAGS=-tags=mygo_nowebview CGO_ENABLED=0 go tool mygo build -skip-dmg -platform darwin/amd64
# Apple Silicon（arm64）
GOFLAGS=-tags=mygo_nowebview CGO_ENABLED=0 go tool mygo build -skip-dmg -platform darwin/arm64
```

产物分别在 `build/darwin-amd64/GoHdfView.app` 与 `build/darwin-arm64/GoHdfView.app`。`-skip-dmg` 跳过磁盘镜像生成；需要 dmg 时去掉该参数。可用 `open build/darwin-arm64/GoHdfView.app` 启动。

发布包由 CI 用 `ditto -c -k --sequesterRsrc --keepParent` 压成 zip，并附 `.sha256` 校验文件。本地校验：

```bash
shasum -a 256 -c GoHdfView-<version>-macos-arm64.zip.sha256
```

### 产物依赖自检


```bash
go version -m build/windows-amd64/GoHdfView.exe
```

应看到 `CGO_ENABLED=0`、`-tags=mygo_nowebview,mygo_nodownload,mygo_noinspector`，依赖只有 `purego`、`mygo`、`go-text/typesetting`、`scigolib/hdf5` 与 `golang.org/x/{image,net,text}`——没有 HDF5 C 库，也没有 WebView2；PE 导入表只有 `kernel32.dll`。macOS 产物同样不含 HDF5 C 库与 WebView2，可用 `otool -L build/darwin-arm64/GoHdfView.app/Contents/MacOS/GoHdfView` 检查动态库依赖。验证细节见 [`docs/verification.md`](docs/verification.md)。

## 开发

```bash
go tool mygo dev
```

`mygo dev` 以开发模式启动窗口，改动 Go 源码后自动重新编译并重启。这是 MyGo 原生模板，没有前端构建步骤，不需要 Bun 或 Node。

### 环境变量速查

| 变量 | 用途 |
| --- | --- |
| `GOMODCACHE` | 模块缓存。不可写时 `go build` 报 `Access is denied`。 |
| `GOPATH` | 工作区缓存，`GOMODCACHE` 未单独设置时也由它派生。 |
| `GOCACHE` | 构建缓存。不可写时同样报 `Access is denied`。 |
| `TMP` / `TEMP` | 编译期临时目录。不可写时报 `creating work dir: mkdir C:\Users\...\Temp\go-build...: Access is denied`。 |
| `GOTOOLCHAIN` | 保持默认 `auto`，让 Go 按 `go.mod` 自动取用 1.27.3 工具链。 |

若报错信息里出现 `C:\Users\<你>\AppData\Local\go-build\...: Access is denied`，就是 `GOCACHE` 不可写；出现 `creating work dir: mkdir ...\Temp\go-build...: Access is denied`，就是 `TMP` 不可写。两者按上面的方式覆盖即可。所有路径都要写成 Windows 绝对路径，`/d/...` 这种 MSYS 写法会被 Go 判为相对路径。

## 目录结构

```
main.go                     应用入口：窗口、菜单、命令行参数
internal/hdf5/              只读 HDF5 访问层（解析库的唯一使用者）
internal/hdf5/testdata/     测试夹具与生成脚本（gen.go、gen_h5py.py）
internal/view/              MyGo 界面：对象树、元数据、表格、图像
docs/                       验证记录（verification.md）
resources/icon.png          应用图标
openspec/                   规格与变更记录（OpenSpec）
```

## 测试与验证

```bash
CGO_ENABLED=0 go test ./...   # 覆盖访问层、各视图与集成路径
go vet ./...
go tool mygo vet              # 检查把 build 值存进结构体这类 MyGo 专属错误
```

夹具由 `internal/hdf5/testdata/gen.go`（scigolib 写接口）与 `gen_h5py.py`（h5py）生成，重新生成后需重跑测试。手工走查的夹具清单与逐条结论在 [`docs/verification.md`](docs/verification.md)。

## 已知限制

下表与 [`design.md`](openspec/changes/archive/2026-11-24-add-hdf5-viewer/design.md) 的 Risks / Trade-offs 一一对应，编号一致。

| # | 限制 | 表现与处理 |
| --- | --- | --- |
| 1 | 8 字节整数精度 | 解析库的切片读取统一返回 `[]float64`，超过 2^53 的 `int64`/`uint64` 无法精确表示。这类值显示时追加 ` ≈` 标记，元数据里同时给出原始数据类型；无符号 64 位整数不会显示成负数。 |
| 2 | 过滤器支持范围 | 只支持 deflate、shuffle、fletcher32、lzf；SZIP、N-bit、Scale-offset 与第三方过滤器无法解码，数据视图会显示「该过滤器不受支持，无法解码」，对象树与元数据仍完整可读。 |
| 3 | 硬链接成环的文件 | 实测可正常打开，回环处的成员被当作空组，不会无限展开；若解析库在其它路径上拒绝成环文件，则归类为「特性不受支持」而非「文件损坏」。 |
| 4 | 表格按页读取 | 框架的表格控件没有可见行范围回调，只能按页窗口化（默认 100 行 × 50 列）。翻页会重新读取该页，已读页有缓存；单元格不会被重复读取。 |
| 5 | Go 版本要求 | `go.mod` 声明 `go 1.27.3` 与 `toolchain go1.27.3`；构建机 Go 更旧时由 `GOTOOLCHAIN=auto` 自动取用。 |
| 6 | 视图按帧重建 | 跨帧状态必须放在 `App` 与 `ListState`/`OutlineState` 里，误把 build 值存进结构体会在运行时静默失效；`go tool mygo vet` 专门检查这类错误，已纳入验收。 |
| 7 | 读取期间文件被外部删除或截断 | 读取失败归类为「文件已损坏」，视图标注该区域不可用，文档保持打开，不尝试自动重开。 |
| 8 | 单文档模型 | 同时只保留一个打开的文档；多标签对照浏览留待后续变更。 |

其它已知边界：

- 只读，且只支持 HDF5；打开 HDF4 文件会明确提示「仅支持 HDF5」。
- 不支持 SWMR 与并行 I/O。
- 软链接与外部链接成员不显示：解析库在加载阶段跳过非硬链接，也未暴露链接种类；应用侧已备好链接节点的字形与 `NodeKind`，等库补齐后即可展示。
- Windows amd64 与 macOS（amd64 / arm64）已纳入自动构建与发布；Linux 未做验证。macOS 产物未签名、未经公证。

## 许可

本项目以 [MIT 许可证](LICENSE) 发布，完整条款见仓库根目录的 `LICENSE` 文件。

第三方依赖（`purego`、`mygo`、`go-text/typesetting`、`scigolib/hdf5`、`golang.org/x/{image,net,text}`）各自遵循其原始许可证；测试夹具的生成与验证依赖 HDF5 与 h5py，仅用于开发，不随产物分发。
