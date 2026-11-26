# Design

## Context

参见 proposal.md 的 Why。现状：`go.mod` 固定 `go 1.27.3`，依赖 `github.com/egoist/mygo v0.10.0`；README 给出纯 Go 构建命令 `GOFLAGS=-tags=mygo_nowebview CGO_ENABLED=0 go tool mygo build -skip-installer`，Windows 产物固定在 `build/windows-amd64/GoHdfView.exe`。现有 `.github/workflows/release.yml` 在 `v*` 标签推送或手动触发时执行 `go vet`、`go tool mygo vet`、`go test`（`CGO_ENABLED=0`），随后构建 Windows 包、生成 zip 与 `.sha256`，标签触发发布 Release、手动触发上传 Artifact。

约束：
- 纯 Go 构建，`CGO_ENABLED=0`，不依赖 HDF5 C 库或 WebView2。
- `mygo_nowebview` tag 必须保留，以维持与 Windows 产物一致的「原生 UI、无 WebView2」特性。
- CI 中 Go 版本须与 Windows job 同源（`go-version-file: go.mod`），避免版本漂移。
- 本机当前不可用 Go 工具链（`go` 不在 PATH，module cache 中未找到 mygo 源码），因此无法在此环境实测 `mygo` 的 macOS 原生输出细节；设计采用保守假设并在任务中要求实施阶段以实际 CI 输出为准验证。

## Goals / Non-Goals

**Goals:**

- 打 `v*` 标签时，macOS amd64 与 arm64 包自动构建并出现在与 Windows 相同的 Release 中，命名清晰、带校验文件。
- 手动触发 CI 时可试跑 macOS 构建并把产物挂到本次运行，不创建 Release。
- 构建命令、tags、校验步骤与 Windows 保持一致，便于维护。
- README 的平台矩阵与本地构建说明反映 macOS 支持。

**Non-Goals:**

- 不实现代码签名、公证（notarization）或 Apple Developer ID 分发；未签名产物的 Gatekeeper 提示在发布说明中交代。
- 不新增 Linux 包、Homebrew formula、pkg 安装包或跨架构 Universal 二进制。
- 不改变应用运行时行为、HDF5 读取接口、分页尺寸或任何 capability spec。
- 不在本变更中实际打标签发布版本。

## Decisions

### 决策 1：扩展现有 `release.yml`，不新建独立 workflow

在现有文件里新增两个并行 job（与 `windows-amd64` 平级）：`macos-amd64` 与 `macos-arm64`，均在 `runs-on: macos-latest`。两者复用相同的 checkout、setup-go、vet、test 步骤，仅在 build 的 `-platform` 参数与产物路径上不同。

理由：Windows 与 macOS 共享同一套检查与同一份发布说明；放在一个 workflow 里可保证标签触发时四份产物进入同一个 Release。替代方案是独立 workflow，但会导致两个 Release 事件竞争、发布说明重复且难以保持版本一致，未采用。

### 决策 2：两个目标架构分别构建，不产出 Universal 二进制

分别以 `-platform darwin/amd64` 与 `-platform darwin/arm64`（具体参数以 `mygo build` 实际支持为准）构建，产物路径分别落到 `build/darwin-amd64/` 与 `build/darwin-arm64/`。每个架构单独归档为 `GoHdfView-<version>-macos-<arch>.zip` 并生成 `.sha256`。

理由：分别构建与 Windows 的 `windows/amd64` 模式一致、CI 简单可靠；Universal 二进制需要额外的 `lipo` 合并步骤且在未实测 mygo 输出前风险更高。若后续需要 Universal，可作为独立变更追加。

### 决策 3：产物格式以 mygo 原生产出为准，统一封装为 zip

`mygo build` 在 macOS 上的原生输出可能是 `.app` 目录或可执行文件（实施时以实际 CI 日志为准）：
- 若产出 `.app` 目录：用 `ditto -c -k --sequesterRsrc --keepParent` 压缩为 zip（保留 macOS 资源分支与符号链接，优于 `zip -r`）。
- 若产出单个可执行文件：参照 Windows 的处理，直接压缩该文件为 zip。

无论哪种，最终上传的是 `.zip` 与 `.sha256`，与 Windows 归档格式保持一致。理由：zip 是跨平台可下载的通用归档，且保留 `.app` 的目录结构与元数据。替代方案是直接上传 `.app` 或 `.dmg`：前者在 GitHub 上以文件夹形式不便下载，后者需要额外工具与配置，均未在本次范围内。

### 决策 4：构建与校验命令参数化，避免四份重复脚本

把 Go 版本、tags、`CGO_ENABLED=0`、`go vet` / `mygo vet` / `go test` 的检查步骤保持与 Windows job 完全一致；构建步骤使用矩阵或明确的 platform 参数传递目标架构。归档步骤用 `VERSION="${GITHUB_REF_NAME#v}"` 统一版本号（手动触发时 `GITHUB_REF_NAME` 可能是分支名，版本取 `dev` 或 `GITHUB_SHA` 短哈希，避免产出误导性的版本号）。

理由：减少复制粘贴导致的参数漂移；版本处理兼容两种触发方式。

### 决策 5：跳过 capability spec，打包契约写入 design.md

本变更只改 CI 与文档，不改变应用的用户可见运行时行为。按照 spec 的定义（系统行为契约），不新增或修改 capability spec，改在 `.openspec.yaml` 设置 `skip_specs: true`，并把产物命名、格式、校验与触发规则作为打包契约记录在本设计文档和 tasks 的验收点中。

## Risks / Trade-offs

- [mygo v0.10.0 的 macOS 平台参数或输出路径与 Windows 不同、未在本机实测] → 在任务中要求实施阶段先在 CI 手动触发试跑，依据实际 `mygo build` 日志确认 `-platform` 参数与输出目录；必要时用 `go tool mygo build -h` 与 CI 产物自检。
- [未签名产物在 macOS 上被 Gatekeeper 拦截] → 在 Release 发布说明中明确「未签名，首次打开需右键→打开或在系统设置中允许」，不实现签名/公证。
- [两个 macOS job 与 Windows job 并行，Release 附件写入冲突] → `softprops/action-gh-release` 支持并行上传附件到同一 Release；若出现竞态，改为等待所有构建完成后用一个专门的发布 job 统一上传。
- [`macos-latest` runner 架构变化导致默认行为漂移] → 始终显式传 `-platform` 目标架构，不依赖 runner 默认架构。
- [手动触发时版本号取自分支名造成命名混乱] → 显式处理 `GITHUB_REF_NAME`，非标签时使用 `dev` 或短 SHA。
- [zip 丢失 `.app` 的符号链接或资源分支] → 使用 `ditto` 而非普通 `zip -r` 压缩 `.app`。

## Migration Plan

无数据或接口迁移。合并后即可生效：下一次 `v*` 标签推送会产出含 Windows 与 macOS 四份附件的 Release；回滚时从 `release.yml` 移除 macOS job 并恢复 README 的平台说明即可。

## Open Questions

- mygo v0.10.0 在 macOS 上的确切 `-platform` 参数字符串与默认输出目录：需在实施阶段通过 `go tool mygo build -h` 和一次手动触发 CI 确认；此问题只影响命令参数，不改变产物契约或任务拆分。
- 是否需要为 macOS 包生成 `.dmg`：当前决定只产出 zip；若用户后续需要 dmg，作为独立变更处理。
