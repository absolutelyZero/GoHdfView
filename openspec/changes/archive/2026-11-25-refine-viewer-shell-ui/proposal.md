# Proposal

## Why

Windows 下系统标题栏跟随浅色模式，在深色应用顶部留下一条白条；左侧对象树此前为框架默认样式，行高、缩进与图标都和 `ui/ui.pen` 的 A 方案有明显偏差。需要把这两处界面行为补齐并记录为正式规格，避免后续改动回退。

## What Changes

- Windows 上隐藏系统标题栏，由应用顶栏充当标题栏：保留系统窗口按钮（深色配色），为按钮预留空间，空白处可拖动窗口、双击最大化。
- 左侧对象树按设计稿对齐：26px 行高、每层 15px 缩进、箭头/图标/名称间距 6px、右侧留白 10px。
- 节点图标改为设计稿的线条图标：组为 folder，二维数据集为 image，其余数据集为 table-2，命名类型为 type，链接为 link。
- 筛选框支持 `Ctrl+K`（macOS `⌘K`）聚焦，并在框内显示快捷键提示。
- 侧栏标题栏改为图标按钮（全部展开、全部折叠、更多菜单），与设计稿一致。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `viewer-shell`: 新增 Windows 自定义标题栏与窗口按钮避让的行为要求。
- `object-tree`: 新增筛选快捷键要求，修改节点类型图标规则，并新增树行排版要求（行高、缩进、间距）。

## Impact

- 代码：`main.go`（窗口选项、顶栏与侧栏标题栏）、`internal/view/tree.go`、`internal/view/style.go`、`internal/hdf5/model.go` 与 `document.go`（缓存数据集秩）。
- 设计：`ui/ui.pen` A 方案（仅作对照，本次不修改设计稿）。
- 不改变 HDF5 读取接口与现有视图切换逻辑。
