# Tasks

## 1. 样式常量与主题

- [x] 1.1 在 `internal/view/style.go` 将 `ColSel` 重命名为 `ColSelRow` 并把值改为 `#191E22`，保持赋给 `t.Accent`；补充新常量 `ColSelDim = ui.Hex("#8A7B5A")`（选中行维度概要色），同步更新 `t.Accent` 一行的注释。验证：`go build ./...` 通过，`grep -rn "ColSel\b" internal/view` 无残留旧引用。

## 2. 树行尺寸增大一号

- [x] 2.1 在 `internal/view/tree.go` 增大行内元素：行高占位 Box 26 → 28，展开箭头图标 11×11 → 12×12（含叶子节点 11×11 占位 Box），`kindIcon` 12×12 → 13×13；同步更新「设计稿的缩进」等注释中引用的行高数值。验证：`go build ./...` 通过。

- [x] 2.2 在 `internal/view/tree.go` 增大行文字：名称 FontSize 12.5 → 13.5，维度概要 10.5 → 11.5，链接标注 11 → 12。验证：`go build ./...` 通过。

- [x] 2.3 更新 `internal/view/tree_test.go` 的 `TestTreeRowHeight`：断言行高 28px（容差 ±1），同步更新断言消息。验证：`go test ./internal/view/ -run TestTreeRowHeight -count=1` 通过。

## 3. 选中行高亮

- [x] 3.1 在 `internal/view/tree.go` 按选中态取色：类型图标 `pick(on, ColAccent, ColMute)`；名称选中时加 `FontWeight(500)`；维度概要选中时取 `ColSelDim`，未选中保持 `ColMute`。验证：`go build ./...` 通过。

- [x] 3.2 在 `internal/view/tree_test.go` 新增选中行高亮测试：构造 fake 数据并选中一行，断言选中行的类型图标颜色为 `ColAccent`、底色生效（通过测试器对选中行元素取色/样式断言，参照现有测试的 `tt.Find` 用法），切换选中后原行恢复灰色。验证：`go test ./internal/view/ -run TestTree -count=1` 全部通过。

## 4. 整体验证

- [x] 4.1 运行全量测试与构建：`go test ./... -count=1`、`go vet ./...` 全部通过；如 `docs/verification.md` 维护界面验证清单，追加树行尺寸与选中高亮的人工检查项。验证：命令零失败，清单已更新。
