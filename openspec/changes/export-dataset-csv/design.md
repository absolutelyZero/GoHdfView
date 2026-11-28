# Design

## Context

数据表由 internal/view 绘制，按页读取（defaultPageRows=100、defaultPageCols=50）；数值统一以 float64 进入视图，显示文本由 formatNumber 生成。main.go 持有窗口与 mygo.Dialog，负责系统交互；view.App 的异步调度与重绘回调均由 main 注入（见 main.go 中 SetAsync/SetInvalidate）。导出必须遵守只读约束：只写用户指定的 CSV，不触碰 HDF5 源文件。动机与范围见 proposal.md，需求见 specs delta。

## Goals / Non-Goals

**Goals:**
- 数值表可导出当前页或整个数据集，CSV 结构清晰（行号列 + 列号表头）。
- 单元格文本与表格显示一致，复用现有格式化逻辑。
- 整个数据集导出采用分块读取与流式写入，内存占用与数据集规模无关。
- 保持 view 层与窗口/对话框层解耦，便于单元测试。

**Non-Goals:**
- 不导出字符串表与复合表。
- 不改变表格的分页状态、选择状态或读取缓存语义。
- 不引入第三方 CSV 库。

## Decisions

1. **职责分离：view 生成数据，main 负责落盘。** 在 view.App 注入导出回调（形如 `SetCSVExporter(func(filename string, write func(io.Writer) error))`）：main 用保存对话框取得目标路径后打开文件，把 io.Writer 交给 view 写入 CSV；view 通过 Reader 读数据、formatNumber 格式化。理由：view 不依赖 mygo 窗口层；测试可注入假回调断言导出入口与生成的 CSV 内容。备选方案（view 直接调用对话框并写文件）会让 view 依赖窗口 API 且难以测试，不采用。
2. **导出范围在导出时选择。** 数据表工具栏增加「导出 CSV」入口，用弹出菜单给出「当前页」与「整个数据集」两个选项。理由：两个范围的代价差异大（当前页即时、全部可能耗时），必须由用户明确选择；入口贴近数据表，比系统菜单更容易发现。备选方案（固定导出当前页）不满足用户要求的两个范围。
3. **分块读取而非翻页驱动。** 「整个数据集」按 defaultPageRows × defaultPageCols 的块调用 Reader.ReadSlice，start/count 的构造复用 requestFor 的规则（高维固定前导维），逐块格式化并写入 io.Writer，不修改 RowPage/ColPage/TableList 等 UI 状态。理由：翻页会触发异步读取与界面状态变化，导出应是只读副作用；分块读取保证内存有界。
4. **CSV 结构。** 使用标准库 encoding/csv：首行第一列为 `#`，其后为列号（与表格列头一致）；数据行第一列为行号，其后为 formatNumber 的结果（含大整数的 ≈ 精度提示，与表格一致）。理由：与表格显示一致便于核对；≈ 提示避免导出数据被误认为精确值。备选（导出原始 float64）会与表格显示不一致，不采用。
5. **失败与部分文件处理。** 导出在 goroutine 上执行（复用 main 已有的异步模式）；先写临时文件，全部成功后原子重命名为目标文件，任一读取或写入错误则删除临时文件并经 setError 显示非模态提示。理由：避免半成品 CSV 冒充完整导出；不阻塞界面。
6. **成功提示与快捷打开。** 导出成功后，main 记录保存路径为「最近导出」状态（含路径与完成时间），经 win.Update 切回主线程，在数据表附近渲染一条非模态提示条：显示路径，并带「打开文件」「打开文件夹」两个按钮，分别调用 mygo.Shell.OpenPath(path) 与 mygo.Shell.ShowItemInFolder(path)。理由：复用现有非模态提示模式，不打断浏览；系统 Shell API 跨平台一致。备选（弹模态对话框）会阻塞浏览，不采用。两个操作失败时均可经 setError 显示原因。

## Risks / Trade-offs

- [超大文件导出耗时很长] → 分块读取 + 流式写入 + 后台执行；界面保持响应，仅显示开始/完成提示。
- [CSV 含 ≈ 标记与枚举名，不是纯数值] → 与表格显示保持一致是明确需求；在导出入口的提示中说明文本来源。
- [导出期间数据集被切换] → 导出开始前快照当前节点与布局；中途变化不影响本次导出（导出的是发起时的数据集）。
- [MyGo 保存对话框 API 细节] → 实现时以 mygo v0.10.0 源码为准确认 SaveDialogOptions 字段与返回值（Dialog.Save），该项仅影响调用写法，不改变架构或需求。
- [Shell 打开操作在测试环境无窗口] → 测试中通过注入的 Shell 处理器或断言提示条状态验证，不实际调用系统 Shell。

## Migration Plan

无数据迁移：新增功能，不改变文件格式与配置项；回滚只需移除导出入口与相关代码。
