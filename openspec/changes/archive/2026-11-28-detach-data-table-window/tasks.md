# Tasks

## 1. 独立窗口状态模型

- [x] 1.1 在 main.go 定义 `tableWindow{win *mygo.Window; state *view.App}` 与 `app.windows []*tableWindow`；新增 `openTableInWindow()` 入口函数骨架；验证：编译通过。
- [x] 1.2 实现窗口工厂注入点（默认 `mygo.NewWindow`，测试可注入假工厂），使窗口创建/关闭可测；验证：单元测试用假工厂断言创建次数与标题。

## 2. 打开与内容固定

- [x] 2.1 实现 `openTableInWindow()`：新建 `view.App`，`SetDocument` 复用当前文件句柄、`SelectPath` 定位到当前选中数据集，注入 SetAsync/SetInvalidate（重绘指向自己的窗口并做指针身份比较）/SetCSVExporter，构造 `ui.View` 显示 `view.DataTable` 的窗口；验证：假工厂断言新窗口标题含数据集名与文件名。
- [x] 2.2 验证内容固定：窗口打开后主窗口切换选中节点，独立窗口仍显示原数据集；验证：Tester/测试断言独立窗口 App 的 Current 不随主窗口变化。

## 3. 入口与生命周期

- [x] 3.1 在数据视图工具栏增加「独立窗口」按钮，「视图」菜单与系统菜单增加「在独立窗口打开数据表」项，共用 `openTableInWindow()`，仅数值/可显示数据集可用；验证：Tester 测试按钮出现且点击触发一次打开。
- [x] 3.2 实现独立窗口关闭：窗口关闭事件/按钮从 `windows` 移除并 Close 窗口，不关闭共享文档句柄；主窗口 OnClose 遍历关闭所有独立窗口；验证：测试断言关闭独立窗口后集合减少、主窗口与文档不受影响，退出时全部清理。

## 4. 回归与验证

- [x] 4.1 运行 `go test ./...` 全量回归，确认既有打开/分页/定位/读取缓存/冻结行号列/CSV 导出与多文件统一树行为未受影响；验证：全部测试通过。

## 5. 样式一致性

- [x] 5.1 修正独立窗口 content 闭包：绘制前调用 `c.SetTheme(view.Theme())`，并以 `Background(view.ColBg)` 的 `ui.Column` 包裹 `view.DataTable`；验证：Tester 或截图对比断言独立窗口使用深色主题背景，视觉风格与主窗口一致。
