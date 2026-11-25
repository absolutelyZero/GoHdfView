# Tasks

## 1. Windows 自定义标题栏

- [x] 1.1 在 `main.go` 中为 Windows 设置 `TitleBarStyle: mygo.TitleBarHidden` 与 `TitleBarHeight: topBarH`，其他平台保持系统标题栏；验证：Windows 启动后顶部无浅色系统标题栏。
- [x] 1.2 顶栏读取 `c.TitleBar()`，左右内边距避开窗口按钮，并在有控件区域时调用 `DragWindow()`；验证：`TestTopBarAvoidsWindowButtons` 断言「打开」按钮不进入窗口按钮区域。
- [x] 1.3 调用 `SetWindowButtonsTheme(mygo.ThemeDark)` 让窗口按钮适配深色顶栏；验证：截图中最小化/最大化/关闭为浅色图标。
- [x] 1.4 拖动、双击顶栏与窗口按钮仍由系统处理；验证：实际窗口拖动与双击最大化可用。

## 2. 侧栏标题栏与筛选

- [x] 2.1 侧栏标题改为 11px/500 字重、1.1 字距，右侧改为 13px 图标按钮（全部展开、全部折叠）与更多菜单；验证：截图图标顺序与 `ui.pen` 一致，`integration_test.go` 仍可点击「全部展开」。
- [x] 2.2 筛选框改为 8px 间距、`$a-mute` 占位色、右侧显示 `Ctrl K`/`⌘K` 提示，并实现同名快捷键聚焦；验证：`TestTreeSearchShortcut` 通过。
- [x] 2.3 移除设计稿中没有的筛选框底部分隔线；验证：截图无额外横线。

## 3. 树行排版与图标

- [x] 3.1 树容器增加 6px 顶部、10px 底部留白，行高固定 26px；验证：`TestTreeRowHeight` 断言相邻行间隔 26px。
- [x] 3.2 缩进改为基础 10px、每层 15px，箭头/图标/名称之间各 6px，行右侧留 10px；验证：截图与设计稿行几何一致。
- [x] 3.3 节点图标改用 SVG：folder、image（秩为 2）、table-2、type、link；验证：matrix 显示图像图标，组显示文件夹图标。
- [x] 3.4 在 `hdf5.Node` 构造时缓存 `Rank` 与 `ShapeText`；验证：`go test ./internal/hdf5` 通过，树渲染不额外读文件。

## 4. 集成验证

- [x] 4.1 运行 `go build ./...` 与 `go test ./...` 全部通过。
- [x] 4.2 启动应用截图核对：顶部无白条、侧栏标题图标、筛选提示、26px 行高、缩进与图标均与设计稿一致。

## Workflow follow-up

- 归档本变更，并把两条 delta 同步进主规格 `viewer-shell` 与 `object-tree`。
