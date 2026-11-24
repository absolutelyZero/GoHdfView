package view

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// 分页尺寸的默认值（design.md 的 Open Questions：100 行 × 50 列）。
const (
	defaultPageRows = 100
	defaultPageCols = 50
	// stringElemLimit 是字符串/复合数据集允许整份读入的元素上限。
	// 解析库没有字符串的切片接口，超过此规模只提示而不读取。
	stringElemLimit = 1 << 20
)

// TableState 是表格视图的分页与维度选择状态。
type TableState struct {
	// RowPage 与 ColPage 是当前页在行、列方向上的页号，从 0 开始。
	RowPage int
	ColPage int
	// Fixed 保存秩大于 2 时被固定的前导维下标。
	Fixed []uint64
}

// tableKey 标识一次表格读取请求。
//
// 它是过期判定的依据：选中对象或翻页会改变 key，
// 迟到的结果因 key 不匹配而被丢弃（任务 5.7）。
type tableKey struct {
	node  hdf5.NodeID
	gen   uint64
	start []uint64
	count []uint64
	kind  readKind
}

// readKind 区分表格要读的数据形态。
type readKind uint8

const (
	readNumeric readKind = iota
	readStrings
	readCompound
)

// Async 是可注入的调度器：默认同步执行读取，
// main.go 换成工作 goroutine，避免大切片阻塞 UI 线程（design.md 决策 4）。
type Async func(work func())

// SetAsync 设置读取调度器；传 nil 恢复同步执行。
func (a *App) SetAsync(fn Async) { a.async = fn }

// SetInvalidate 设置「请重绘」的回调，异步结果写回后由它触发。
func (a *App) SetInvalidate(fn func()) { a.invalidate = fn }

// run 执行一次读取：有调度器时交给它，否则就地执行。
func (a *App) run(work func()) {
	if a.async != nil {
		a.async(work)
		return
	}
	work()
}

// resetTable 清空表格缓存；选中对象变化时调用。
func (a *App) resetTable() {
	a.Table = TableState{}
	a.TableList = ui.ListState{}
	a.tableKey = tableKey{}
	a.tableData = nil
	a.tableErr = nil
	a.inflight = nil
	a.gen++
}

// TableLayout 描述当前对象在表格里怎么摆。
type TableLayout struct {
	// Rows 与 Cols 是表格展示的整体行列数（已把高维折叠到固定下标）。
	Rows int
	Cols int
	// Rank 是数据集的秩。
	Rank int
	// Scalar 表示秩为 0：直接显示单值，不用表格。
	Scalar bool
	// Leading 是秩大于 2 时被固定的前导维。
	Leading []uint64
	// Kind 是要读取的数据形态。
	Kind readKind
	// Dims 是完整维度，用于计算切片起点。
	Dims []uint64
	// OK 为 false 时 Info 读取失败。
	OK bool
	// Reason 说明为什么不能显示数据。
	Reason string
}

// Layout 计算当前对象的表格布局。
func (a *App) Layout() TableLayout {
	n := a.Current
	if n == nil {
		return TableLayout{Reason: "请选择一个对象"}
	}
	if n.Kind != hdf5.KindDataset {
		return TableLayout{Reason: "请选择一个数据集"}
	}
	info, err := a.info(n)
	if err != nil {
		a.tableErr = err
		return TableLayout{Reason: "读取数据集描述失败：" + err.Error()}
	}
	a.curInfo = info

	out := TableLayout{Rank: info.Rank, Dims: info.Shape, OK: true}
	switch {
	case info.Datatype.IsString():
		out.Kind = readStrings
	case info.Datatype.IsCompound():
		out.Kind = readCompound
	default:
		out.Kind = readNumeric
	}

	switch info.Rank {
	case 0:
		out.Scalar = true
		out.Rows, out.Cols = 1, 1
	case 1:
		out.Rows, out.Cols = int(info.Shape[0]), 1
	case 2:
		out.Rows, out.Cols = int(info.Shape[0]), int(info.Shape[1])
	default:
		// 秩 ≥ 3：固定前导维，展示最后两维。
		lead := info.Shape[:info.Rank-2]
		out.Leading = make([]uint64, len(lead))
		a.ensureFixed(len(lead))
		for i := range lead {
			v := a.Table.Fixed[i]
			if v >= lead[i] {
				v = 0
			}
			out.Leading[i] = v
			a.Table.Fixed[i] = v
		}
		out.Rows = int(info.Shape[info.Rank-2])
		out.Cols = int(info.Shape[info.Rank-1])
	}

	// 维度可能大到 int 溢出；用 uint64 比较后再转换。
	if info.ElemCount == math.MaxUint64 && info.Overflow {
		return TableLayout{Rank: info.Rank, Dims: info.Shape, OK: true, Kind: out.Kind,
			Reason: "元素总数超出可表示范围，无法以表格展示"}
	}
	return out
}

// ensureFixed 保证 Fixed 长度足够。
func (a *App) ensureFixed(n int) {
	for len(a.Table.Fixed) < n {
		a.Table.Fixed = append(a.Table.Fixed, 0)
	}
	if len(a.Table.Fixed) > n {
		a.Table.Fixed = a.Table.Fixed[:n]
	}
}

// pageWindow 返回当前页的行、列范围与元素数。
func (a *App) pageWindow(l TableLayout) (rowStart, rowCount, colStart, colCount int) {
	rowStart = a.Table.RowPage * defaultPageRows
	colStart = a.Table.ColPage * defaultPageCols
	rowCount = min(defaultPageRows, l.Rows-rowStart)
	colCount = min(defaultPageCols, l.Cols-colStart)
	if rowCount < 0 {
		rowCount = 0
	}
	if colCount < 0 {
		colCount = 0
	}
	return
}

// PageCount 返回行方向的页数。
func PageCount(rows int) int {
	if rows <= 0 {
		return 1
	}
	return (rows + defaultPageRows - 1) / defaultPageRows
}

// ColPageCount 返回列方向的页数。
func ColPageCount(cols int) int {
	if cols <= 0 {
		return 1
	}
	return (cols + defaultPageCols - 1) / defaultPageCols
}

// requestFor 把当前页换算成读取请求。
func (a *App) requestFor(l TableLayout) tableKey {
	key := tableKey{node: a.Current.ID, gen: a.gen, kind: l.Kind}
	if l.Scalar {
		return key
	}
	rs, rc, cs, cc := a.pageWindow(l)
	if l.Kind != readNumeric {
		// 字符串与复合没有切片接口：整份读取，start/count 留空。
		return key
	}
	if l.Rank == 1 {
		key.start = []uint64{uint64(rs)}
		key.count = []uint64{uint64(rc)}
		return key
	}
	key.start = append(key.start, l.Leading...)
	key.count = make([]uint64, len(l.Leading))
	for i := range key.count {
		key.count[i] = 1
	}
	key.start = append(key.start, uint64(rs), uint64(cs))
	key.count = append(key.count, uint64(rc), uint64(cc))
	return key
}

// pageEntry 是一页已读数据。
type pageEntry struct {
	data any
	err  error
}

// pageCacheLimit 是页缓存的条目上限：保留最近读过的若干页，
// 使上下翻页与来回切换不再重复读取，又不会随数据集规模无限增长。
const pageCacheLimit = 8

// id 返回请求的文本指纹，用作页缓存的键。
//
// tableKey 含切片，不能直接作 map 键，因此拼成稳定字符串。
func (k tableKey) id() string {
	var b strings.Builder
	b.WriteString(k.node.Path)
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(k.node.Address, 10))
	b.WriteByte('|')
	// 不含 gen：gen 只用于丢弃过期结果，翻页本身不改变「这一页的内容」。
	b.WriteString(strconv.Itoa(int(k.kind)))
	for _, v := range k.start {
		b.WriteByte('|')
		b.WriteString(strconv.FormatUint(v, 10))
	}
	b.WriteByte('/')
	for _, v := range k.count {
		b.WriteByte('|')
		b.WriteString(strconv.FormatUint(v, 10))
	}
	return b.String()
}

// cacheGet 取一页已读结果。
func (a *App) cacheGet(id string) (pageEntry, bool) {
	e, ok := a.pageCache[id]
	return e, ok
}

// cachePut 存入一页结果，超出容量时淘汰最早的条目。
func (a *App) cachePut(id string, e pageEntry) {
	if _, exists := a.pageCache[id]; !exists {
		a.pageOrder = append(a.pageOrder, id)
	}
	a.pageCache[id] = e
	for len(a.pageOrder) > pageCacheLimit {
		old := a.pageOrder[0]
		a.pageOrder = a.pageOrder[1:]
		delete(a.pageCache, old)
	}
}

// sameKey 比较两个请求是否等价。
func sameKey(x, y tableKey) bool {
	if x.node != y.node || x.gen != y.gen || x.kind != y.kind {
		return false
	}
	if len(x.start) != len(y.start) || len(x.count) != len(y.count) {
		return false
	}
	for i := range x.start {
		if x.start[i] != y.start[i] || x.count[i] != y.count[i] {
			return false
		}
	}
	return true
}

// ensureTable 保证当前页的数据已就绪。
//
// 命中缓存时不读取；否则发起一次读取（可能异步）。
func (a *App) ensureTable(l TableLayout) {
	if !l.OK || a.Current == nil {
		return
	}
	rd := a.reader()
	if rd == nil {
		return
	}
	key := a.requestFor(l)
	if a.tableKey.node == key.node && a.tableKey.gen == key.gen &&
		sameKey(a.tableKey, key) && (a.tableData != nil || a.tableErr != nil) {
		return
	}
	// 该页之前读过：直接命中缓存，不再读文件。
	if e, ok := a.cacheGet(key.id()); ok {
		a.tableKey = key
		a.tableData = e.data
		a.tableErr = e.err
		return
	}
	a.tmu.Lock()
	busy := a.inflight != nil && sameKey(*a.inflight, key)
	if !busy {
		k := key
		a.inflight = &k
	}
	a.tmu.Unlock()
	if busy {
		return
	}
	k := key
	node := a.Current
	kind := l.Kind
	scalar := l.Scalar
	total := l.Rows * l.Cols
	a.run(func() {
		var (
			data any
			err  error
		)
		switch kind {
		case readStrings:
			if total > stringElemLimit {
				err = hdf5.ErrTooLarge
			} else {
				data, err = rd.ReadStrings(node)
			}
		case readCompound:
			if total > stringElemLimit {
				err = hdf5.ErrTooLarge
			} else {
				data, err = rd.ReadCompound(node)
			}
		default:
			if scalar {
				data, err = rd.ReadSlice(node, nil, nil)
			} else {
				data, err = rd.ReadSlice(node, k.start, k.count)
			}
		}
		a.finishTable(k, data, err)
	})
}

// finishTable 写回读取结果，并丢弃过期结果。
//
// 过期判定同时看请求代号与当前对象：切换对象后到达的旧结果
// 不会覆盖新视图（任务 5.7）。
func (a *App) finishTable(key tableKey, data any, err error) {
	if key.gen != a.gen || a.Current == nil || key.node != a.Current.ID {
		a.tmu.Lock()
		a.inflight = nil
		a.tmu.Unlock()
		return // 过期结果，丢弃
	}
	a.tmu.Lock()
	a.tableKey = key
	a.tableData = data
	a.tableErr = err
	a.inflight = nil
	a.cachePut(key.id(), pageEntry{data: data, err: err})
	a.tmu.Unlock()
	if a.invalidate != nil {
		a.invalidate()
	}
}

// Busy 报告是否有读取正在进行。
//
// 异步读取时 UI 线程与工作 goroutine 并发访问 inflight，
// 因此这里加锁；同步读取时它总是返回 false。
func (a *App) Busy() bool {
	a.tmu.Lock()
	defer a.tmu.Unlock()
	return a.inflight != nil
}

// NextPage 翻到下一页（行方向）。
func (a *App) NextPage(l TableLayout) {
	if (a.Table.RowPage+1)*defaultPageRows < l.Rows {
		a.Table.RowPage++
		a.gen++
	}
}

// PrevPage 翻到上一页（行方向）。
func (a *App) PrevPage(l TableLayout) {
	if a.Table.RowPage > 0 {
		a.Table.RowPage--
		a.gen++
	}
}

// NextColPage 翻到右侧下一列页。
func (a *App) NextColPage(l TableLayout) {
	if (a.Table.ColPage+1)*defaultPageCols < l.Cols {
		a.Table.ColPage++
		a.gen++
	}
}

// PrevColPage 翻到左侧上一列页。
func (a *App) PrevColPage(l TableLayout) {
	if a.Table.ColPage > 0 {
		a.Table.ColPage--
		a.gen++
	}
}

// SetFixedDim 把某个前导维固定到指定下标。
func (a *App) SetFixedDim(i int, v uint64) {
	a.ensureFixed(i + 1)
	if a.Table.Fixed[i] == v {
		return
	}
	a.Table.Fixed[i] = v
	a.gen++
}

// DataTable 构建数据视图的表格（任务 5.1–5.6）。
func DataTable(c *ui.Context, app *App) {
	l := app.Layout()
	if !l.OK {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, l.Reason).TextColor(c.Theme().TextMuted)
		})
		return
	}
	if l.Scalar {
		scalarValue(c, app, l)
		return
	}

	app.ensureTable(l)
	ui.Column(c).Fill().Gap(8).Children(func() {
		tableToolbar(c, app, l)
		if app.tableErr != nil {
			readFailure(c, app, app.tableErr)
		}
		tableBody(c, app, l)
		pager(c, app, l)
	})
}

// scalarValue 显示秩为 0 的单个值。
func scalarValue(c *ui.Context, app *App, l TableLayout) {
	app.ensureTable(l)
	ui.Column(c).Fill().Padding(24).Gap(8).Children(func() {
		ui.Text(c, "标量值").TextColor(c.Theme().TextMuted).FontSize(12)
		switch d := app.tableData.(type) {
		case *hdf5.Slice:
			if len(d.Values) > 0 {
				ui.Text(c, formatNumber(app.datatype(), d.Values[0])).FontSize(20).Bold()
				return
			}
			ui.Text(c, "（无值）").TextColor(c.Theme().TextMuted)
		default:
			if app.tableErr != nil {
				readFailure(c, app, app.tableErr)
				return
			}
			ui.Text(c, "（无值）").TextColor(c.Theme().TextMuted)
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

// tableToolbar 显示维度选择器（秩 ≥ 3）与页信息。
func tableToolbar(c *ui.Context, app *App, l TableLayout) {
	if len(l.Leading) == 0 {
		return
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "固定前导维").FontSize(12).TextColor(c.Theme().TextMuted)
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
			ui.Text(c, "（空数据集）").TextColor(c.Theme().TextMuted)
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
			ui.Text(c, "正在读取…").TextColor(c.Theme().TextMuted)
		})
		return
	}

	// 列：首列是行号，其后是当前列页的列号。
	cols := make([]ui.TableColumn, 0, cc+1)
	cols = append(cols, ui.TableColumn{Title: "", RowHeader: true, Width: 64, Fixed: true})
	for j := 0; j < cc; j++ {
		cols = append(cols, ui.TableColumn{Title: strconv.Itoa(cs + j), Width: 110, Align: ui.End})
	}

	dt := app.datatype()
	app.TableList.Selected = nil
	ui.Table(c, &app.TableList, cols, rc, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		idx := row*cc + (col - 1)
		if idx < 0 || idx >= len(data.Values) {
			ui.Text(c, "—").TextColor(c.Theme().TextMuted)
			return
		}
		ui.Text(c, formatNumber(dt, data.Values[idx])).SingleLine()
	}).Grow(1)
}

// failedTable 在读取失败时画出受影响区域的占位表（任务 5.6）。
//
// 只标注本页范围内的单元格，视图其它部分（树、元数据、翻页）照常可用。
func failedTable(c *ui.Context, app *App, l TableLayout, rs, rc, cs, cc int) {
	cols := make([]ui.TableColumn, 0, cc+1)
	cols = append(cols, ui.TableColumn{Title: "", RowHeader: true, Width: 64, Fixed: true})
	for j := 0; j < cc; j++ {
		cols = append(cols, ui.TableColumn{Title: strconv.Itoa(cs + j), Width: 110, Align: ui.End})
	}
	app.TableList.Selected = nil
	ui.Table(c, &app.TableList, cols, rc, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		ui.Text(c, failureCell).FontSize(12).TextColor(c.Theme().TextMuted)
	}).Grow(1)
}

// stringsTable 以单列展示字符串数据集。
func stringsTable(c *ui.Context, app *App, l TableLayout) {
	data, ok := app.tableData.(*hdf5.Strings)
	if !ok || data == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").TextColor(c.Theme().TextMuted)
		})
		return
	}
	cols := []ui.TableColumn{
		{Title: "", RowHeader: true, Width: 64, Fixed: true},
		{Title: "值", Width: 320},
	}
	n := min(defaultPageRows, len(data.Values)-app.Table.RowPage*defaultPageRows)
	if n < 0 {
		n = 0
	}
	rs := app.Table.RowPage * defaultPageRows
	ui.Table(c, &app.TableList, cols, n, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		idx := rs + row
		if idx < 0 || idx >= len(data.Values) {
			ui.Text(c, "—").TextColor(c.Theme().TextMuted)
			return
		}
		ui.Text(c, truncateCell(data.Values[idx])).SingleLine()
	}).Grow(1)
}

// compoundTable 以每成员一列展示复合数据集。
func compoundTable(c *ui.Context, app *App, l TableLayout) {
	data, ok := app.tableData.(*hdf5.Compound)
	if !ok || data == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").TextColor(c.Theme().TextMuted)
		})
		return
	}
	cols := make([]ui.TableColumn, 0, len(data.Members)+1)
	cols = append(cols, ui.TableColumn{Title: "", RowHeader: true, Width: 64, Fixed: true})
	for _, m := range data.Members {
		cols = append(cols, ui.TableColumn{Title: m, Width: 140})
	}
	rs := app.Table.RowPage * defaultPageRows
	n := min(defaultPageRows, len(data.Rows)-rs)
	if n < 0 {
		n = 0
	}
	ui.Table(c, &app.TableList, cols, n, func(row, col int) {
		if col == 0 {
			ui.Text(c, strconv.Itoa(rs+row)).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		idx := rs + row
		if idx < 0 || idx >= len(data.Rows) {
			ui.Text(c, "—").TextColor(c.Theme().TextMuted)
			return
		}
		name := data.Members[col-1]
		ui.Text(c, formatAny(data.Rows[idx][name])).SingleLine()
	}).Grow(1)
}

// pager 显示翻页控件与页信息。
func pager(c *ui.Context, app *App, l TableLayout) {
	rows := l.Rows
	if l.Kind != readNumeric {
		rows = pageRowTotal(app, l)
	}
	rowPages := PageCount(rows)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Button(c, "上一页").OnClick(func() { app.PrevPage(l) })
		ui.Textf(c, "第 %d / %d 页（共 %d 行）", app.Table.RowPage+1, rowPages, rows).
			FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Button(c, "下一页").OnClick(func() { app.NextPage(l) })
		if l.Rank >= 2 && l.Cols > defaultPageCols {
			ui.Spacer(c).Grow(1)
			ui.Button(c, "左侧列").OnClick(func() { app.PrevColPage(l) })
			ui.Textf(c, "列 %d–%d / %d", app.Table.ColPage*defaultPageCols+1,
				min((app.Table.ColPage+1)*defaultPageCols, l.Cols), l.Cols).
				FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Button(c, "右侧列").OnClick(func() { app.NextColPage(l) })
		}
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
	ui.Column(c).Gap(4).Padding(8, 12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "⚠").TextColor(c.Theme().TextMuted)
			ui.Text(c, "该区域读取失败，单元格显示为不可用：").FontSize(12).
				TextColor(c.Theme().TextMuted)
			ui.Spacer(c).Grow(1)
			label := "详情"
			if app.errDetail {
				label = "收起"
			}
			ui.Button(c, label).OnClick(func() { app.errDetail = !app.errDetail })
		})
		if app.errDetail {
			ui.Text(c, err.Error()).FontSize(12).TextColor(c.Theme().TextMuted)
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
