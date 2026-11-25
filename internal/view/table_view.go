package view

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// 表格的视觉尺寸取自设计稿（A · 仪器）。
const (
	tableHeadH   = 32
	tableRowH    = 28
	tableGutterW = 46
	tableColW    = 111
	tablePadX    = 12
)

// gridCol 描述自绘表格的一列。
type gridCol struct {
	title  string
	width  float32
	align  ui.Align
	gutter bool
}

func (g gridCol) headColor() ui.Color {
	if g.gutter {
		return ColGutter
	}
	return ColMute
}

// dataGrid 画设计稿样式的表格：32px 表头、28px 行高、1px 行分隔线，
// 列宽固定、超出部分横向滚动。rows 是本页行数，cell 负责填充单元格内容。
func dataGrid(c *ui.Context, cols []gridCol, rows int, cell func(row, col int)) ui.Element {
	scroll := ui.ScrollBoth(c).Grow(1)
	scroll.Children(func() {
		head := ui.Row(c).Height(tableHeadH).Shrink(0).AlignItems(ui.Stretch).
			Background(ColHeadBg).Sticky().Top(0)
		head.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLine) })
		head.Children(func() {
			for _, col := range cols {
				h := ui.Row(c).Width(col.width).Shrink(0).Padding(0, tablePadX).AlignItems(ui.Center)
				if col.align == ui.End {
					h.Justify(ui.End)
				}
				h.Children(func() {
					ui.Text(c, col.title).Font(FontMono).FontSize(10.5).
						TextColor(col.headColor()).SingleLine()
				})
			}
		})
		for i := 0; i < rows; i++ {
			i := i
			row := ui.Row(c).Height(tableRowH).Shrink(0).AlignItems(ui.Stretch)
			row.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColRowLine) })
			row.Children(func() {
				for j := range cols {
					j := j
					box := ui.Row(c).Width(cols[j].width).Shrink(0).
						Padding(0, tablePadX).AlignItems(ui.Center)
					if cols[j].align == ui.End {
						box.Justify(ui.End)
					}
					box.Children(func() { cell(i, j) })
				}
			})
		}
	})
	return scroll
}

// DataTable 构建数据视图的表格（任务 5.1–5.6）。
func DataTable(c *ui.Context, app *App) {
	l := app.Layout()
	if !l.OK {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, l.Reason).FontSize(12).TextColor(ColMute)
		})
		return
	}
	if l.Scalar {
		scalarValue(c, app, l)
		return
	}

	app.ensureTable(l)
	ui.Column(c).Fill().Children(func() {
		tableToolbar(c, app, l)
		if app.tableErr != nil {
			readFailure(c, app, app.tableErr)
		}
		tableBody(c, app, l)
		pager(c, app, l)
		if app.locateErr != "" {
			locateFailure(c, app)
		}
	})
}

// scalarValue 显示秩为 0 的单个值。
func scalarValue(c *ui.Context, app *App, l TableLayout) {
	app.ensureTable(l)
	ui.Column(c).Fill().Padding(24).Gap(8).Children(func() {
		ui.Text(c, "标量值").FontSize(11).TextColor(ColMute).SingleLine()
		switch d := app.tableData.(type) {
		case *hdf5.Slice:
			if len(d.Values) > 0 {
				ui.Text(c, formatNumber(app.datatype(), d.Values[0])).
					Font(FontMono).FontSize(22).TextColor(ColText)
				return
			}
			ui.Text(c, "（无值）").FontSize(12).TextColor(ColMute)
		default:
			if app.tableErr != nil {
				readFailure(c, app, app.tableErr)
				return
			}
			ui.Text(c, "（无值）").FontSize(12).TextColor(ColMute)
		}
	})
}

// datatype 返回当前数据集的元素类型；读取失败时返回零值。
func (a *App) datatype() hdf5.DatatypeInfo {
	if a.curInfo != nil {
		return a.curInfo.Datatype
	}
	if a.Current == nil {
		return hdf5.DatatypeInfo{}
	}
	info, err := a.info(a.Current)
	if err != nil {
		return hdf5.DatatypeInfo{}
	}
	return info.Datatype
}

// tableToolbar 显示维度选择器（秩 ≥ 3）。
func tableToolbar(c *ui.Context, app *App, l TableLayout) {
	if len(l.Leading) == 0 {
		return
	}
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 20).Gap(10)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
	bar.Children(func() {
		ui.Text(c, "固定前导维").FontSize(11).TextColor(ColMute).SingleLine()
		for i := range l.Leading {
			idx := i
			limit := int(l.Dims[idx])
			opts := dimOptions(limit)
			sel := strconv.FormatUint(app.Table.Fixed[idx], 10)
			ui.Select(c, &sel, opts).OnChange(func() {
				if v, err := strconv.ParseUint(sel, 10, 64); err == nil {
					app.SetFixedDim(idx, v)
				}
			})
		}
	})
}

// dimOptions 生成维度选择器的选项；过大的维度只列出前若干项。
func dimOptions(n int) []string {
	const cap = 500
	if n <= 0 {
		return []string{"0"}
	}
	if n > cap {
		n = cap
	}
	out := make([]string, n)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}

// tableBody 画表格本体或字符串/复合的替代呈现。
func tableBody(c *ui.Context, app *App, l TableLayout) {
	switch l.Kind {
	case readStrings:
		stringsTable(c, app, l)
		return
	case readCompound:
		compoundTable(c, app, l)
		return
	}

	rs, rc, cs, cc := app.pageWindow(l)
	if rc <= 0 || cc <= 0 {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "（空数据集）").FontSize(12).TextColor(ColMute)
		})
		return
	}

	// 注意判 nil：读取失败时 fakeReader/真实读取器返回的是
	// 类型化 nil 指针，装进 any 后不等于 nil，单看 ok 会拿到空指针。
	data, ok := app.tableData.(*hdf5.Slice)
	if !ok || data == nil {
		// 没有数据时：读取失败就画一张全部标为不可用的表，
		// 让用户看到受影响的区域范围；否则说明仍在读取。
		if app.tableErr != nil {
			failedTable(c, app, l, rs, rc, cs, cc)
			return
		}
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").FontSize(12).TextColor(ColMute)
		})
		return
	}

	// 列：首列是行号，其后是当前列页的列号。
	cols := make([]gridCol, 0, cc+1)
	cols = append(cols, gridCol{title: "#", width: tableGutterW, align: ui.End, gutter: true})
	for j := 0; j < cc; j++ {
		cols = append(cols, gridCol{title: strconv.Itoa(cs + j), width: tableColW, align: ui.End})
	}

	dt := app.datatype()
	dataGrid(c, cols, rc, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).Font(FontMono).FontSize(10.5).
				TextColor(ColFaint).SingleLine()
			return
		}
		idx := row*cc + (col - 1)
		if idx < 0 || idx >= len(data.Values) {
			ui.Text(c, "—").Font(FontMono).FontSize(11.5).TextColor(ColMute).SingleLine()
			return
		}
		ui.Text(c, formatNumber(dt, data.Values[idx])).Font(FontMono).
			FontSize(11.5).TextColor(ColCell).SingleLine()
	})
}

// failedTable 在读取失败时画出受影响区域的占位表（任务 5.6）。
//
// 只标注本页范围内的单元格，视图其它部分（树、元数据、翻页）照常可用。
func failedTable(c *ui.Context, app *App, l TableLayout, rs, rc, cs, cc int) {
	cols := make([]gridCol, 0, cc+1)
	cols = append(cols, gridCol{title: "#", width: tableGutterW, align: ui.End, gutter: true})
	for j := 0; j < cc; j++ {
		cols = append(cols, gridCol{title: strconv.Itoa(cs + j), width: tableColW, align: ui.End})
	}
	dataGrid(c, cols, rc, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).Font(FontMono).FontSize(10.5).
				TextColor(ColFaint).SingleLine()
			return
		}
		ui.Text(c, failureCell).Font(FontMono).FontSize(11.5).
			TextColor(ColMute).SingleLine()
	})
}

// stringsTable 以单列展示字符串数据集。
//
// 单元格里的长文本会被截断；点击单元格把完整值放到上方的详情区，
// 这是 spec「变长字符串数据集」要求的「可通过单元格详情查看完整值」。
func stringsTable(c *ui.Context, app *App, l TableLayout) {
	data, ok := app.tableData.(*hdf5.Strings)
	if !ok || data == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").FontSize(12).TextColor(ColMute)
		})
		return
	}
	if app.cellDetail != "" {
		cellDetailPanel(c, app)
	}
	rs := app.Table.RowPage * defaultPageRows
	n := min(defaultPageRows, len(data.Values)-rs)
	if n < 0 {
		n = 0
	}
	cols := []gridCol{
		{title: "#", width: tableGutterW, align: ui.End, gutter: true},
		{title: "值", width: 320, align: ui.Start},
	}
	dataGrid(c, cols, n, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).Font(FontMono).FontSize(10.5).
				TextColor(ColFaint).SingleLine()
			return
		}
		idx := rs + row
		if idx < 0 || idx >= len(data.Values) {
			ui.Text(c, "—").FontSize(11.5).TextColor(ColMute).SingleLine()
			return
		}
		full := data.Values[idx]
		ui.Text(c, truncateCell(full)).FontSize(11.5).TextColor(ColCell).SingleLine().
			OnClick(func() { app.cellDetail = full })
	})
}

// cellDetailPanel 显示被点开的单元格全文。
func cellDetailPanel(c *ui.Context, app *App) {
	box := ui.Column(c).Shrink(0).Padding(8, 20).Gap(4).Background(ColPanel2)
	box.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLine) })
	box.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "单元格全文").FontSize(11).TextColor(ColMute).SingleLine()
			ui.Textf(c, "%d 个字符", len([]rune(app.cellDetail))).
				FontSize(11).TextColor(ColFaint).SingleLine()
			ui.Spacer(c).Grow(1)
			GhostButton(c, "收起", func() { app.cellDetail = "" })
		})
		ui.Text(c, app.cellDetail).FontSize(11.5).TextColor(ColCell)
	})
}

// compoundTable 以每成员一列展示复合数据集。
func compoundTable(c *ui.Context, app *App, l TableLayout) {
	data, ok := app.tableData.(*hdf5.Compound)
	if !ok || data == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").FontSize(12).TextColor(ColMute)
		})
		return
	}
	cols := make([]gridCol, 0, len(data.Members)+1)
	cols = append(cols, gridCol{title: "#", width: tableGutterW, align: ui.End, gutter: true})
	for _, m := range data.Members {
		cols = append(cols, gridCol{title: m, width: 140, align: ui.Start})
	}
	rs := app.Table.RowPage * defaultPageRows
	n := min(defaultPageRows, len(data.Rows)-rs)
	if n < 0 {
		n = 0
	}
	dataGrid(c, cols, n, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).Font(FontMono).FontSize(10.5).
				TextColor(ColFaint).SingleLine()
			return
		}
		idx := rs + row
		if idx < 0 || idx >= len(data.Rows) {
			ui.Text(c, "—").FontSize(11.5).TextColor(ColMute).SingleLine()
			return
		}
		name := data.Members[col-1]
		ui.Text(c, formatAny(data.Rows[idx][name])).Font(FontMono).
			FontSize(11.5).TextColor(ColCell).SingleLine()
	})
}

// pager 显示翻页控件与快速定位。
//
// 行方向（上下）分页固定在底部左侧，列方向（左右）分页固定在底部右侧，
// 坐标定位位于两者之间，避免两个方向混在同一处。
func pager(c *ui.Context, app *App, l TableLayout) {
	rows := l.Rows
	if l.Kind != readNumeric {
		rows = pageRowTotal(app, l)
	}
	rs, rc, cs, cc := app.pageWindow(l)
	rowEnd := rs
	if rc > 0 {
		rowEnd = rs + rc - 1
	}
	colPaged := l.Rank >= 2 && l.Cols > defaultPageCols

	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 20).Gap(10)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
	bar.Children(func() {
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Textf(c, "行 %d–%d / %d", rs, rowEnd, rows).
				Font(FontMono).FontSize(11).TextColor(ColMute).SingleLine()
			pagerButton(c, "上一屏", app.Table.RowPage <= 0, func() {
				app.PrevPage(l)
				app.syncLocate(l)
			})
			pagerButton(c, "下一屏", rows <= 0 || (app.Table.RowPage+1)*defaultPageRows >= rows, func() {
				app.NextPage(l)
				app.syncLocate(l)
			})
		})
		ui.Spacer(c).Grow(1)
		locateBar(c, app, l)
		ui.Spacer(c).Grow(1)
		if colPaged {
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Textf(c, "列 %d–%d / %d", cs, cs+max(cc-1, 0), l.Cols).
					Font(FontMono).FontSize(11).TextColor(ColFaint).SingleLine()
				pagerButton(c, "左侧列", app.Table.ColPage <= 0, func() {
					app.PrevColPage(l)
					app.syncLocate(l)
				})
				pagerButton(c, "右侧列", (app.Table.ColPage+1)*defaultPageCols >= l.Cols, func() {
					app.NextColPage(l)
					app.syncLocate(l)
				})
			})
		}
	})
}

// pagerButton 是分页用的小按钮；到达边界时置灰且不响应点击。
func pagerButton(c *ui.Context, label string, disabled bool, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Padding(4, 9).Radius(5).Border(1, ColLine)
	b.Children(func() {
		col := ColDim
		if disabled {
			col = ColFaint
		}
		ui.Text(c, label).FontSize(11.5).TextColor(col).SingleLine()
	})
	if disabled {
		b.Disabled(true)
	} else if onClick != nil {
		b.OnClick(onClick)
	}
	return b
}

// locateBar 是快速定位控件：秩不超过 quickLocateMax 时逐维输入，
// 否则用一个逗号分隔的输入框，保证底栏宽度可控。
func locateBar(c *ui.Context, app *App, l TableLayout) {
	if !app.locateReady {
		app.fillLocate(l)
		app.locateReady = true
	}
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, IconLocate).Size(12, 12).TextColor(ColAccent)
		ui.Text(c, "定位").FontSize(11.5).TextColor(ColDim).SingleLine()
		if l.Rank > quickLocateMax {
			in := ui.TextInput(c, &app.locateText).Width(190).Font(FontMono).
				FontSize(11).Placeholder("D0, D1, …")
			in.Label("坐标输入")
			in.OnTextKey(func(k ui.TextKey) bool { return app.locateKey(l, -1, k) })
		} else {
			for i := 0; i < l.Rank; i++ {
				i := i
				ui.Text(c, axisName(i)).Font(FontMono).FontSize(10).
					TextColor(ColFaint).SingleLine()
				in := ui.TextInput(c, &app.locateBoxes[i]).Width(54).Font(FontMono).
					FontSize(11).Placeholder("下标")
				in.Label(axisName(i) + " 输入")
				in.OnTextKey(func(k ui.TextKey) bool { return app.locateKey(l, i, k) })
			}
		}
		b := ui.ButtonBase(c).Padding(4, 9).Radius(5).Border(1, ColAccent)
		b.Children(func() {
			ui.Text(c, "跳至").FontSize(11.5).TextColor(ColAccent).SingleLine()
		})
		b.OnClick(func() { app.submitLocateButton(l) })
	})
}

// locateKey 处理定位输入框里的按键：Enter 提交，Esc 清空本次输入。
func (a *App) locateKey(l TableLayout, i int, k ui.TextKey) bool {
	switch k.Key {
	case ui.KeyEnter:
		if k.Mods != 0 {
			return false
		}
		if i < 0 {
			a.submitLocateText(l, k.Text())
		} else {
			a.submitLocateBoxes(l, i, k.Text())
		}
		return true
	case ui.KeyEscape:
		if k.Mods != 0 {
			return false
		}
		a.clearLocate()
		k.SetText("")
		return true
	}
	return false
}

// submitLocateButton 处理「跳至」按钮：使用上一帧已写回的值。
func (a *App) submitLocateButton(l TableLayout) {
	if l.Rank > quickLocateMax {
		a.submitLocateText(l, a.locateText)
		return
	}
	a.submitLocateBoxes(l, -1, "")
}

// submitLocateBoxes 解析逐维输入并定位；focused 是被编辑的输入框，
// 它的最新文本由 k.Text() 提供，其余维度取已写回的值。
func (a *App) submitLocateBoxes(l TableLayout, focused int, text string) {
	coords := make([]uint64, l.Rank)
	for i := 0; i < l.Rank && i < quickLocateMax; i++ {
		s := a.locateBoxes[i]
		if i == focused {
			s = text
		}
		v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if err != nil {
			a.locateErr = axisName(i) + " 必须是整数下标"
			return
		}
		coords[i] = v
	}
	a.applyLocate(l, coords)
}

// submitLocateText 解析逗号分隔的坐标文本并定位。
func (a *App) submitLocateText(l TableLayout, text string) {
	coords, err := ParseCoords(text, l.Rank)
	if err != nil {
		a.locateErr = err.Error()
		return
	}
	a.applyLocate(l, coords)
}

// applyLocate 执行坐标定位，失败时保留当前页并记录提示。
func (a *App) applyLocate(l TableLayout, coords []uint64) {
	if err := a.Locate(l, coords); err != nil {
		a.locateErr = err.Error()
		return
	}
	a.locateErr = ""
	a.fillLocate(l)
}

// syncLocate 在翻页后把输入框更新到当前页起点。
func (a *App) syncLocate(l TableLayout) {
	a.locateErr = ""
	a.fillLocate(l)
}

// fillLocate 用当前页的坐标填充输入框。
func (a *App) fillLocate(l TableLayout) {
	rs, _, cs, _ := a.pageWindow(l)
	lead := len(l.Leading)
	coords := make([]uint64, l.Rank)
	for i := 0; i < lead && i < l.Rank; i++ {
		coords[i] = a.Table.Fixed[i]
	}
	switch {
	case l.Rank >= 2:
		coords[l.Rank-2] = uint64(rs)
		coords[l.Rank-1] = uint64(cs)
	case l.Rank == 1:
		coords[0] = uint64(rs)
	}
	if l.Rank <= quickLocateMax {
		for i, v := range coords {
			a.locateBoxes[i] = strconv.FormatUint(v, 10)
		}
		for i := l.Rank; i < quickLocateMax; i++ {
			a.locateBoxes[i] = ""
		}
		return
	}
	parts := make([]string, len(coords))
	for i, v := range coords {
		parts[i] = strconv.FormatUint(v, 10)
	}
	a.locateText = strings.Join(parts, ", ")
}

// clearLocate 放弃本次输入（Esc）。
func (a *App) clearLocate() {
	a.locateBoxes = [quickLocateMax]string{}
	a.locateText = ""
	a.locateErr = ""
}

// locateFailure 在底栏下方显示定位失败的提示，不阻塞其它操作。
func locateFailure(c *ui.Context, app *App) {
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(4, 20).Gap(6).Children(func() {
		ui.Icon(c, IconInfo).Size(11, 11).TextColor(ColAccent)
		ui.Text(c, "定位失败："+app.locateErr).FontSize(11).
			TextColor(ColAccent).SingleLine()
	})
}

// pageRowTotal 返回字符串/复合数据集的总行数。
func pageRowTotal(app *App, l TableLayout) int {
	switch d := app.tableData.(type) {
	case *hdf5.Strings:
		return len(d.Values)
	case *hdf5.Compound:
		return len(d.Rows)
	}
	return l.Rows
}

// readFailure 显示读取失败的提示与原因（任务 5.6）。
//
// 提示是非模态的：表格其余部分照常渲染，用户可以继续浏览其它对象。
// 「详情」按钮展开完整错误文本，默认只显示一行摘要。
func readFailure(c *ui.Context, app *App, err error) {
	// 「不支持」「超上限」与「读取失败」对用户是不同的结论：前者是
	// 这个文件用了本阅读器不解码的过滤器，重试也没用；超上限需要
	// 换一种读法；只有后者可能是局部损坏。
	head := "该区域读取失败，单元格显示为不可用："
	switch {
	case errors.Is(err, hdf5.ErrUnsupported):
		head = "该过滤器不受支持，无法解码："
	case errors.Is(err, hdf5.ErrTooLarge):
		head = "该区域一次读取的数据量超过上限，请改用分页或图像视图："
	}
	box := ui.Column(c).Shrink(0).Padding(8, 20).Gap(4).Background(ColWarnBg)
	box.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLine) })
	box.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "⚠").FontSize(12).TextColor(ColAccent).SingleLine()
			ui.Text(c, head).FontSize(11.5).TextColor(ColDim).SingleLine()
			ui.Spacer(c).Grow(1)
			label := "详情"
			if app.errDetail {
				label = "收起"
			}
			GhostButton(c, label, func() { app.errDetail = !app.errDetail })
		})
		if app.errDetail {
			ui.Text(c, err.Error()).FontSize(11.5).TextColor(ColMute)
		}
	})
}

// failureCell 是读取失败时单元格显示的占位文本。
const failureCell = "不可用"

// tableFailure 报告当前页是否因读取失败而没有数据。
func tableFailure(app *App) error {
	if app.tableData == nil && app.tableErr != nil {
		return app.tableErr
	}
	return nil
}

// truncateCell 截断过长的单元格文本。
func truncateCell(s string) string {
	const limit = 200
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}

// formatAny 把复合成员的任意值格式化为文本。
func formatAny(v any) string {
	switch x := v.(type) {
	case nil:
		return "—"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case string:
		return truncateCell(x)
	default:
		return "—"
	}
}

// approxThreshold 是 2^53：超过它的整数无法在 float64 中精确表示。
const approxThreshold = 1 << 53

// formatNumber 按元素类型格式化一个数值（任务 5.4）。
//
// 解析库把一切数值统一转成 float64，因此 8 字节整数超过 2^53 时
// 已经损失精度；这里不假装精确，而是在值后加 ≈ 标记（design.md 决策 9）。
// 无符号 64 位整数按无符号解释显示，不出现负数。
func formatNumber(dt hdf5.DatatypeInfo, v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	}

	// 枚举：显示成员名。
	if dt.IsEnum() && len(dt.EnumMembers) > 0 {
		if name, ok := enumName(dt, v); ok {
			return name
		}
	}

	if dt.IsInteger() {
		s := strconv.FormatFloat(v, 'f', 0, 64)
		if math.Abs(v) > approxThreshold {
			return s + " ≈"
		}
		return s
	}

	bits := 64
	if dt.Size == 4 {
		bits = 32
	}
	return strconv.FormatFloat(v, 'g', -1, bits)
}

// enumName 在枚举成员表里查找数值对应的名称。
func enumName(dt hdf5.DatatypeInfo, v float64) (string, bool) {
	for _, m := range dt.EnumMembers {
		if float64(m.Value) == v {
			return m.Name, true
		}
	}
	return "", false
}

// tooLargeString 说明字符串/复合数据集为何不能整份展示。
//
// 解析库只提供整份读取，没有按行切片，所以这类数据集在超过上限时
// 只能拒绝并告诉用户下一步能做什么（spec 的「超大切片请求被拒绝」）。
func tooLargeString(total int) error {
	return fmt.Errorf("%w：该数据集有 %d 个元素，超过字符串视图上限 %d；"+
		"请改用元数据视图查看结构", hdf5.ErrTooLarge, total, stringElemLimit)
}
