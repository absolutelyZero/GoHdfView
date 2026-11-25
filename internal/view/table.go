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

// quickLocateMax 是逐维输入框的上限。
//
// 秩超过这个值时，底栏改用一个逗号分隔的输入框，避免控件过多。
const quickLocateMax = 4

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
	a.cellDetail = ""
	a.locateBoxes = [quickLocateMax]string{}
	a.locateText = ""
	a.locateErr = ""
	a.locateReady = false
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
				err = tooLargeString(total)
			} else {
				data, err = rd.ReadStrings(node)
			}
		case readCompound:
			if total > stringElemLimit {
				err = tooLargeString(total)
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

// axisName 返回输入框的轴名，D0 是最外层维度。
func axisName(i int) string { return "D" + strconv.Itoa(i) }

// Locate 把完整坐标换算成行、列页号，并更新被固定的前导维。
//
// 坐标长度必须等于数据集秩，每个值必须是该维的合法下标。
// 目标页与当前页完全相同时不推进代号，避免缓存失效与重复读取。
func (a *App) Locate(l TableLayout, coords []uint64) error {
	if !l.OK {
		return errors.New("当前对象不可用")
	}
	if l.Scalar {
		return errors.New("标量没有可分页的坐标")
	}
	if len(coords) != l.Rank {
		return fmt.Errorf("坐标需要 %d 个值，实际 %d 个", l.Rank, len(coords))
	}
	for i, v := range coords {
		if i >= len(l.Dims) {
			return fmt.Errorf("第 %d 维不存在", i+1)
		}
		if v >= l.Dims[i] {
			return fmt.Errorf("第 %d 维下标 %d 越界（范围 0–%d）", i+1, v, l.Dims[i]-1)
		}
	}

	var row, col uint64
	switch {
	case l.Rank >= 2:
		row, col = coords[l.Rank-2], coords[l.Rank-1]
	case l.Rank == 1:
		row = coords[0]
	}
	rowPage := int(row / uint64(defaultPageRows))
	colPage := int(col / uint64(defaultPageCols))

	lead := len(l.Leading)
	changed := rowPage != a.Table.RowPage || colPage != a.Table.ColPage
	a.ensureFixed(lead)
	for i := 0; i < lead; i++ {
		if a.Table.Fixed[i] != coords[i] {
			changed = true
		}
		a.Table.Fixed[i] = coords[i]
	}
	a.Table.RowPage = rowPage
	a.Table.ColPage = colPage
	if changed {
		a.gen++
	}
	return nil
}

// ParseCoords 把逗号或空白分隔的坐标文本解析成下标切片。
//
// 空段会被忽略（例如结尾多写一个逗号），但解析结果的数量必须等于秩。
func ParseCoords(s string, rank int) ([]uint64, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]uint64, 0, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个坐标 %q 不是合法下标", i+1, p)
		}
		out = append(out, v)
	}
	if len(out) != rank {
		return nil, fmt.Errorf("坐标需要 %d 个值，实际 %d 个", rank, len(out))
	}
	return out, nil
}
