# Design

## Context

窗口由 `main.go` 的 `mygo.NewWindow` 创建，视图是原生 UI（`ui.View(a.view)`），顶栏 `topBar` 是 46px 的应用内控件。框架提供 `WindowOptions.TitleBarStyle = mygo.TitleBarHidden`、`ui.Context.TitleBar()` 与 `ui.Element.DragWindow()`，可在隐藏系统标题栏后由视图绘制标题栏。左侧树使用 `ui.Outline` 与自定义 `Prefix`，框架的 `ListState.RowHeight` 只对表格生效，树行高度由内容撑起。

## Goals / Non-Goals

**Goals:**

- Windows 下不再出现浅色系统标题栏，且窗口拖动、最大化、关闭等系统行为保持不变。
- 左侧树的行高、缩进、间距与图标与 `ui/ui.pen` A 方案一致。

**Non-Goals:**

- 不隐藏系统窗口按钮、不自绘最小化/最大化/关闭。
- 不修改 `ui/ui.pen` 设计稿。
- 不改变 HDF5 读取、视图切换与分页行为。

## Decisions

### 决策 1：Windows 隐藏标题栏，顶栏充当标题栏

仅在 `runtime.GOOS == "windows"` 时设置 `TitleBarStyle: mygo.TitleBarHidden` 与 `TitleBarHeight: 46`；顶栏按 `c.TitleBar().Right/Left` 增加左右内边距避开窗口按钮，并在 `tb.Height > 0` 时调用 `DragWindow()`。

替代方案是用 DWM 强制深色系统标题栏（`Theme.SetSource(ThemeDark)`）：改动更小，但标题栏仍占一行、与设计稿的顶栏结构重复，未采用。

### 决策 2：窗口按钮沿用系统绘制，按深色背景配色

调用 `SetWindowButtonsTheme(mygo.ThemeDark)`，让系统按钮的浅色图标适配深色顶栏，保留贴靠布局、系统菜单与无障碍行为。

### 决策 3：树行高由 26px 占位撑起

`ListState.RowHeight` 只作用于表格，树行原本只有约 16.6px。在 `Prefix` 里加入 `Height(26)` 的缩进占位，使每行固定 26px，同时不改动框架的列表实现。

### 决策 4：图标使用 SVG，数据集秩在节点构造时缓存

`ui.Outline` 只构建可见行，逐行使用 `ui.Icon` 的成本可控。图标按节点类型选择：folder、image（秩为 2）、table-2、type、link。为避免每帧调用 `Node.Info()` 读取描述符，节点构造时把 `Rank` 与 `ShapeText` 一起缓存。

### 决策 5：筛选框快捷键通过窗口级 Shortcut 聚焦

`c.Shortcut(Ctrl/Cmd, K)` 命中时调用输入框的 `Focus()`；提示文案按平台显示 `Ctrl K` 或 `⌘K`。

## Risks / Trade-offs

- [隐藏标题栏后窗口按钮盖住顶栏操作] → 顶栏右内边距取 `c.TitleBar().Right + 16`，并用测试断言「打开」按钮不进入按钮区域。
- [非 Windows 平台布局回退] → `c.TitleBar()` 在普通窗口返回零，内边距自然回到 16px，不需要平台分支布局。
- [二维非数值数据集显示图像图标] → 图标仅作类型提示，点击后数据视图仍按实际类型显示不可用原因。
- [高 DPI 下 26px 行高的视觉密度] → 使用 DIP 常量，随系统缩放等比放大，与设计稿比例一致。

## Migration Plan

无数据迁移。回滚时移除 `TitleBarStyle`/`TitleBarHeight` 设置与 `DragWindow` 调用，并恢复树行 Prefix 的缩进占位即可。
