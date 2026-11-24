# GoHdfView

用 Go 与 [MyGo](https://mygo.run) 复刻的 HDFView 只读阅读器：打开 HDF5 文件，浏览对象树、查看元数据与属性、以表格或图像方式查看数据集。

**只读**：本项目不提供任何写入、编辑或另存为能力，也不会修改被打开的文件。写入 API 仅出现在测试夹具生成脚本 `internal/hdf5/testdata/gen.go`（`//go:build ignore`，不参与常规构建），并由 `internal/hdf5/readonly_test.go` 扫描源码来保证这一点。

## 构建前提

| 要求 | 说明 |
| --- | --- |
| Go 1.27.3 | `go.mod` 固定 `go 1.27.3` 与 `toolchain go1.27.3`。本机装的是更旧的 Go 时，`GOTOOLCHAIN=auto`（默认值）会让 Go 自动下载并切换到 1.27.3。 |
| `CGO_ENABLED=0` | 纯 Go 构建，不依赖 HDF5 C 库。 |
| 可写的 `GOMODCACHE` / `GOPATH` / `GOCACHE` / `TMP` | 默认缓存与临时目录不可写时（常见于受限的 Windows 账户或沙箱），需要指向可写目录。 |

只支持 **Windows**（当前验证平台）与 **HDF5**（不支持 HDF4）。

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
go tool mygo build -skip-installer
```

产物在 `build/windows-amd64/GoHdfView.exe`，可直接双击运行，或从命令行传入文件路径：

```bash
./build/windows-amd64/GoHdfView.exe path\to\file.h5
```

`-skip-installer` 用来跳过 Windows 安装包的生成；不需要安装包时加上它，可以避免下载 NSIS 到 `%LOCALAPPDATA%\mygo`。

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
internal/view/              MyGo 界面：对象树、元数据、表格、图像
resources/icon.png          应用图标
openspec/                   规格与变更记录（OpenSpec）
```

## 已知限制

- 只读，且只支持 HDF5；打开 HDF4 文件会明确提示「仅支持 HDF5」。
- 依赖的纯 Go 解析库 `github.com/scigolib/hdf5` 尚不支持 SZIP、N-bit、Scale-offset 等第三方过滤器；使用这些过滤器的数据集会被标记为不受支持，而不是显示错误数据。
- 不支持 SWMR 与并行 I/O。
- 仅在 Windows 上验证；macOS 与 Linux 未做验证。

## 许可

见 `openspec/changes/add-hdf5-viewer/proposal.md` 的 Impact 章节。
