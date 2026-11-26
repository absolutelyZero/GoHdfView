# Tasks

## 1. macOS 构建任务接入 CI

- [x] 1.1 在 `.github/workflows/release.yml` 中新增 `macos-amd64` 与 `macos-arm64` 两个 job（`runs-on: macos-latest`），复用 checkouts、`go-version-file: go.mod` 的 setup-go、`go vet ./...`、`go tool mygo vet`、`CGO_ENABLED=0 go test ./...` 检查；验证：在 Actions 页手动触发 workflow，两个 macOS job 的检查步骤均通过。
- [x] 1.2 为两个 macOS job 添加构建步骤，使用 `CGO_ENABLED=0` 与 `GOFLAGS=-tags=mygo_nowebview`，通过明确的 `-platform` 参数分别构建 darwin/amd64 与 darwin/arm64，产物落到独立目录；验证：手动触发后从 CI 日志确认 `go tool mygo build` 成功且产物目录存在期望文件（`mygo` 的实际参数以 `go tool mygo build -h` 与首次试跑日志为准，记录在本任务备注中）。
- [x] 1.3 归档产物：按决策 3 用 `ditto`（若为 `.app` 目录）或等价压缩方式生成 `GoHdfView-<version>-macos-amd64.zip` 与 `GoHdfView-<version>-macos-arm64.zip`，并各自生成 `.sha256`；版本在标签触发时取 `${GITHUB_REF_NAME#v}`，手动触发时取 `dev` 或短 SHA；验证：手动触发后 Artifacts 中出现两个 zip 与两个 sha256，解压后应用结构与 `mygo` 输出一致，`shasum -a 256 -c` 校验通过。

## 2. 发布与触发规则

- [x] 2.1 扩展标签触发（`github.ref_type == 'tag'`）的 `softprops/action-gh-release` 步骤，把 macOS 的 zip 与 sha256 加入 `files`，与 Windows 产物并列；验证：用临时测试标签（如 `v0.0.0-test-mac`）推送后，Draft/Release 中出现四个平台的归档与校验文件，发布说明包含 macOS 未签名提示。
- [x] 2.2 扩展手动触发（`github.ref_type != 'tag'`）的 `actions/upload-artifact`，上传 macOS zip 与 sha256；验证：手动触发的运行页 Artifacts 含 `GoHdfView-macos-amd64` 与 `GoHdfView-macos-arm64` 两项且可下载。
- [x] 2.3 在发布说明 body 中补充 macOS 支持说明与未签名打开方式（右键打开 / 系统设置允许）；验证：测试标签的 Release 正文包含该说明。

## 3. 文档更新

- [x] 3.1 更新 `README.md`「构建前提」与平台说明：把「只支持 Windows」改为「Windows amd64 与 macOS（amd64 / arm64）」，补充 macOS 本地构建命令（含 `CGO_ENABLED=0`、`-tags=mygo_nowebview`、`-skip-installer` 与 `-platform darwin/amd64` / `darwin/arm64`）；验证：文档中命令与 CI 实际使用的参数一致，无残留「仅 Windows」表述。
- [x] 3.2 在 `README.md` 补充 macOS 产物依赖自检说明（若适用）：产物不含 HDF5 C 库与 WebView2、未签名的打开方式；验证：文档说明与 design.md 的风险条目一致。

## 4. 集成验证

- [ ] 4.1 手动触发 workflow 试跑完整流程（checks → 四平台构建 → 归档 → Artifacts），确认无失败步骤；验证：Actions 运行页所有 job 为绿色，Artifacts 可下载且校验文件正确。
- [ ] 4.2 推送测试标签验证 Release 附件齐全：包含 Windows zip/sha256 与 macOS amd64、arm64 的 zip/sha256 共八份文件；验证：Release 页面文件列表与命名符合 design.md 的产物契约。

## Workflow follow-up

- 评审通过后归档本变更。
