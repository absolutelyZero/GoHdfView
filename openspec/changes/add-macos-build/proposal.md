# Proposal

## Why

当前项目只有 Windows amd64 的自动打包与发布流程（`.github/workflows/release.yml`），README 也明确写「只支持 Windows」。macOS 用户只能自行 clone 源码并用 Go 工具链手动构建，门槛高且容易因 Go 版本、tags、缓存目录等细节踩坑。需要让 CI 在打标签时同时产出 macOS 包并发布到 GitHub Release，使 macOS 用户能像 Windows 用户一样直接下载安装包。

## What Changes

- 在 GitHub Actions 中新增 macOS 构建任务：在 `macos-latest` runner 上使用与 Windows 一致的 Go 版本、`CGO_ENABLED=0` 与 `mygo_nowebview` tag 编译原生桌面应用。
- 同时产出两个架构的包：Intel（amd64）与 Apple Silicon（arm64）。
- 产物遵循 mygo 的 macOS 原生产物格式（详见 design.md），打包为可下载的归档文件并附带 SHA-256 校验文件。
- 标签触发（`v*`）时把 macOS 归档与校验文件发布到同一个 GitHub Release，与 Windows 产物并列；手动触发（`workflow_dispatch`）时上传为本次运行的 Artifact，便于打标签前试跑。
- 更新 `README.md` 的「构建前提」与平台说明：将支持平台从「仅 Windows」扩展为「Windows + macOS」，并补充 macOS 本地构建/自检命令。
- **非目标**：不在本变更中新增 Linux 包、代码签名/公证（notarization）或 Homebrew 分发；这些可后续单独提出。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

无。本变更只调整构建与发布流水线及文档，不改变应用的任何用户可见运行时行为；spec 描述的是系统行为而非构建 tooling，因此不需要新增或修改 capability spec（已在 `.openspec.yaml` 中设置 `skip_specs: true`）。打包产物契约在 `design.md` 中记录。

## Impact

- CI：`openspec/changes/add-macos-build` 实施后修改 `.github/workflows/release.yml`（新增 `macos-amd64` / `macos-arm64` 或等价 job，调整 Release 附件列表）。
- 文档：`README.md`（平台矩阵、本地构建与自检说明）。
- 依赖：继续复用现有 `go.mod` 固定的 Go 版本、`github.com/egoist/mygo v0.10.0` 工具链与 `-tags=mygo_nowebview`，不引入新的运行时依赖。
- 产物命名：新增 `GoHdfView-<version>-macos-amd64.zip` / `GoHdfView-<version>-macos-arm64.zip` 及对应 `.sha256`（若 mygo 原生产物为 `.app` 目录，按 design.md 的决定封装为 zip）。
