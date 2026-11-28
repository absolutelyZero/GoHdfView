package view

import (
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// Source 是对象树的只读数据来源。
//
// 真实实现包装 *hdf5.Document；测试可以注入假实现，
// 从而在没有真实文件的情况下构造任意形状（含上万成员）的树。
type Source interface {
	// Roots 返回所有文件根节点；没有文档时返回 nil。
	Roots() []*hdf5.Node
	// Children 返回节点的直接成员；非容器返回 nil, nil。
	Children(n *hdf5.Node) ([]*hdf5.Node, error)
}

// Reader 是表格视图的只读数据来源。
//
// 真实实现是 *hdf5.Document；测试注入假实现，
// 从而记录每次请求的 start/count，或制造读取错误与延迟。
type Reader interface {
	ReadSlice(node *hdf5.Node, start, count []uint64) (*hdf5.Slice, error)
	ReadStrings(node *hdf5.Node) (*hdf5.Strings, error)
	ReadCompound(node *hdf5.Node) (*hdf5.Compound, error)
	ReadStrided(node *hdf5.Node, rowStart, colStart, rowCount, colCount, rowStride, colStride uint64) (*hdf5.Slice, error)
}

// Meta 是数据集描述（维度、类型、布局）的来源。
//
// 真实实现直接调 *hdf5.Node.Info；测试注入假实现，
// 从而在没有真实文件时构造任意维度（含超大与 3 维）的数据集。
type Meta interface {
	Info(n *hdf5.Node) (*hdf5.DatasetInfo, error)
}

// nodeMeta 是 Meta 的默认实现。
type nodeMeta struct{}

func (nodeMeta) Info(n *hdf5.Node) (*hdf5.DatasetInfo, error) { return n.Info() }

// docSource 把 *hdf5.Document 适配成 Source。
type docSource struct{ doc *hdf5.Document }

func (d docSource) Roots() []*hdf5.Node { return []*hdf5.Node{d.doc.Root()} }

func (d docSource) Children(n *hdf5.Node) ([]*hdf5.Node, error) { return n.Children() }

// expandBudget 限制「全部展开」一次性加载的节点数。
//
// 展开是显式操作，允许比浏览更重的读取；但没有任何上限时，
// 一个宽而深的文件会让界面卡住，所以超过预算就停止展开。
const expandBudget = 20000

// maxDepth 是遍历的深度上限，仅用于防御性保护：
// 解析库已拒绝硬链接环，正常文件不会触到。
const maxDepth = 64

// App 保存跨帧存活的状态。
//
// MyGo 按帧重建视图，控件是 build-pass 的临时值，
// 因此树的行、展开集合、当前对象都必须留在结构体里（design.md 决策 1）。
type App struct {
	src Source
	doc *hdf5.Document

	// nodes 与 kids 是对象树的内存索引：按 NodeID 取节点，
	// 以及每个已加载容器缓存下来的子节点 ID 列表。
	nodes  map[hdf5.NodeID]*hdf5.Node
	kids   map[hdf5.NodeID][]hdf5.NodeID
	loaded map[hdf5.NodeID]bool
	roots  []hdf5.NodeID

	// Tree 是对象树的控件状态：Open 是展开集合，List.Selected 是行选择。
	Tree ui.OutlineState[hdf5.NodeID]
	// TreeRow 是当前选中的行；-1 表示树中没有高亮行。
	TreeRow int

	// Selected 是当前对象的身份，Current 是它的节点。
	// 二者不随树的行消失而清空：折叠祖先组后右侧仍显示原对象。
	Selected *hdf5.NodeID
	Current  *hdf5.Node

	// pending 是等待定位的目标，在下一帧构建前换算成行号。
	pending *hdf5.NodeID

	// LoadCalls 统计向 Source 取子节点的次数，供性能守卫测试断言。
	LoadCalls int

	// Err 是最近一次读取失败的原因，由视图显示为提示。
	Err error

	// rd 是表格读取来源覆盖；为 nil 时按节点所属文档路由。
	rd Reader
	// meta 是数据集描述覆盖；为 nil 时直接问节点。
	meta Meta
	// multiRd / multiMeta 由多文件组合 Source 注入，按节点所属文档路由。
	multiRd   Reader
	multiMeta Meta

	// async 是读取调度器，invalidate 是「请重绘」的回调。
	// 两者都可由 main.go 注入；未注入时读取同步执行、结果直接生效。
	async      Async
	invalidate func()

	// csvExporter 由 main.go 注入：接收范围并异步保存 view 生成的 CSV。
	// 为 nil 时视图不显示导出入口（如测试环境）。
	csvExporter func(CSVExportScope, CSVWriteFunc)

	// tableDetacher 由 main.go 注入：把当前数据表在独立窗口打开。
	// 为 nil 时视图不显示「独立窗口」入口。
	tableDetacher func()

	// gen 是表格请求的代号，每次换对象或翻页自增。
	// 迟到的读取结果因代号不符被丢弃（任务 5.7）。
	gen uint64

	// Table 是分页状态，TableList 是表格控件的选择状态。
	Table     TableState
	TableList ui.ListState

	// 快速定位的输入状态：秩不超过 quickLocateMax 时逐维写入 locateBoxes，
	// 否则用 locateText 保存逗号分隔的坐标。
	locateBoxes [quickLocateMax]string
	locateText  string
	locateErr   string
	locateReady bool

	// tableKey 是当前已就绪结果对应的请求，
	// inflight 是正在读取的请求（未完成时为 nil）。
	//
	// tmu 保护 inflight 与「结果是否已写回」的判定：
	// 后台 goroutine 写、UI 线程读，两者不能无锁共享。
	tmu      sync.Mutex
	tableKey tableKey
	inflight *tableKey

	// pageCache 保存已读过的页，避免来回翻页重复读取（任务 5.2）。
	// 键是请求的文本指纹，容量受 pageCacheLimit 限制。
	pageCache map[string]pageEntry
	pageOrder []string

	// tableData 是最近一次读取成功的数据（*hdf5.Slice / *hdf5.Strings / *hdf5.Compound）。
	tableData any
	// tableErr 是最近一次读取失败的原因。
	tableErr error

	// curInfo 是当前数据集描述的快照，由 Layout 每帧刷新。
	// 布局与单元格格式化都要用它，缓存一份避免同一帧解码两次。
	curInfo *hdf5.DatasetInfo
	// errDetail 控制错误详情是否展开（任务 5.6 的「详情入口」）。
	errDetail bool
	// cellDetail 是被选中单元格的完整文本。字符串数据集在单元格里
	// 截断显示，点开后在这里看全文（spec 的「变长字符串数据集」）。
	cellDetail string

	// Image 是图像视图的状态。
	Image ImageState
}

// NewApp 创建空状态的应用。
func NewApp() *App {
	a := &App{TreeRow: -1}
	a.reset()
	return a
}

func (a *App) reset() {
	a.nodes = make(map[hdf5.NodeID]*hdf5.Node)
	a.kids = make(map[hdf5.NodeID][]hdf5.NodeID)
	a.loaded = make(map[hdf5.NodeID]bool)
	a.roots = nil
	a.Tree = ui.OutlineState[hdf5.NodeID]{}
	a.TreeRow = -1
	a.Selected = nil
	a.Current = nil
	a.pending = nil
	a.LoadCalls = 0
	a.Err = nil
	a.Table = TableState{}
	a.TableList = ui.ListState{}
	a.locateBoxes = [quickLocateMax]string{}
	a.locateText = ""
	a.locateErr = ""
	a.locateReady = false
	a.tableKey = tableKey{}
	a.inflight = nil
	a.pageCache = make(map[string]pageEntry)
	a.pageOrder = nil
	a.tableData = nil
	a.tableErr = nil
	a.curInfo = nil
	a.errDetail = false
	a.cellDetail = ""
	a.Image = ImageState{}
	a.gen = 0
}

// SetSource 换用新的数据来源，并重置全部树状态。
func (a *App) SetSource(src Source) error {
	a.reset()
	a.src = src
	if src == nil {
		a.doc = nil
		a.rd = nil
		a.multiRd = nil
		a.multiMeta = nil
		return nil
	}
	roots := src.Roots()
	if len(roots) == 0 {
		a.Tree.Generation++
		return nil
	}
	if roots[0] == nil {
		return fmt.Errorf("%w：文档没有根组", hdf5.ErrCorrupt)
	}
	for _, root := range roots {
		a.nodes[root.ID] = root
		a.roots = append(a.roots, root.ID)
		a.Tree.Open.Add(root.ID)
	}
	a.Tree.Generation++
	a.TreeRow = 0
	a.Selected = &roots[0].ID
	a.Current = roots[0]
	a.resetTable()
	a.resetImage()
	return nil
}

// AddDocument 把一个已打开文档加入统一树：登记根节点、选择它并装上按文件路由的读取器。
func (a *App) AddDocument(doc *hdf5.Document) error {
	if doc == nil || doc.Root() == nil {
		return fmt.Errorf("%w：文档没有根组", hdf5.ErrCorrupt)
	}
	ms, ok := a.src.(*multiSource)
	if !ok {
		ms = newMultiSource()
		a.src = ms
	}
	ms.add(doc)
	if a.multiRd == nil {
		a.multiRd = routedReader{ms}
		a.multiMeta = routedMeta{ms}
	}
	a.AddRoot(doc.Root())
	return nil
}

// AddFileReader 注入某个文件的读取覆盖（测试用）。必须在 AddDocument 之后调用。
func (a *App) AddFileReader(file string, rd Reader) {
	if ms, ok := a.src.(*multiSource); ok {
		ms.setReader(file, rd)
	}
}

// AddFileMeta 注入某个文件的描述覆盖（测试用）。必须在 AddDocument 之后调用。
func (a *App) AddFileMeta(file string, meta Meta) {
	if ms, ok := a.src.(*multiSource); ok {
		ms.setMeta(file, meta)
	}
}

// RemoveDocument 从统一树移除一个文件并释放其路由登记。
func (a *App) RemoveDocument(file string) {
	if ms, ok := a.src.(*multiSource); ok {
		ms.remove(file)
	}
	a.RemoveRoot(file)
}

// AddRoot 把一个新文件的根节点加入统一树，并选中它；不清空其它文件的展开状态。
func (a *App) AddRoot(root *hdf5.Node) {
	if root == nil {
		return
	}
	a.nodes[root.ID] = root
	a.roots = append(a.roots, root.ID)
	a.Tree.Open.Add(root.ID)
	a.Tree.Generation++
	a.Selected = &root.ID
	a.Current = root
	a.pending = &root.ID
	a.resetTable()
	a.resetImage()
}

// RemoveRoot 移除某个文件的所有根节点与索引，并把选中转到剩余文件。
func (a *App) RemoveRoot(file string) {
	var openIDs []hdf5.NodeID
	for id := range a.nodes {
		if id.File != file && a.Tree.Open.Has(id) {
			openIDs = append(openIDs, id)
		}
	}
	var keep []hdf5.NodeID
	for _, id := range a.roots {
		if id.File != file {
			keep = append(keep, id)
		}
	}
	a.roots = keep
	for id := range a.nodes {
		if id.File == file {
			delete(a.nodes, id)
		}
	}
	for id := range a.kids {
		if id.File == file {
			delete(a.kids, id)
		}
	}
	for id := range a.loaded {
		if id.File == file {
			delete(a.loaded, id)
		}
	}
	a.Tree.Open.Clear()
	for _, id := range openIDs {
		a.Tree.Open.Add(id)
	}
	for _, id := range a.roots {
		a.Tree.Open.Add(id)
	}
	a.Tree.Generation++
	if len(a.roots) > 0 {
		next := a.nodes[a.roots[len(a.roots)-1]]
		a.Selected = &next.ID
		a.Current = next
		a.pending = &next.ID
	} else {
		a.Selected = nil
		a.Current = nil
		a.TreeRow = -1
		a.pending = nil
	}
	a.resetTable()
	a.resetImage()
}

// SetRoutedSources 注入按节点所属文档路由的读取与描述来源。
// 单文件场景不调用，仍走 rd/doc 的既有路径。
func (a *App) SetRoutedSources(rd Reader, meta Meta) {
	a.multiRd, a.multiMeta = rd, meta
}

// SetRoots 在保留现有树状态的前提下替换根节点集合。
// 新打开或关闭文件后由 main 调用，避免整棵树被重置。
func (a *App) SetRoots(roots []*hdf5.Node, selected *hdf5.Node) error {
	if len(roots) == 0 {
		a.src = nil
		a.doc = nil
		a.multiRd, a.multiMeta = nil, nil
		a.reset()
		return nil
	}
	a.reset()
	for _, root := range roots {
		a.nodes[root.ID] = root
		a.roots = append(a.roots, root.ID)
		a.Tree.Open.Add(root.ID)
	}
	a.Tree.Generation++
	if selected == nil {
		selected = roots[len(roots)-1]
	}
	a.nodes[selected.ID] = selected
	a.Selected = &selected.ID
	a.Current = selected
	a.TreeRow = 0
	a.pending = &selected.ID
	a.resetTable()
	a.resetImage()
	return nil
}

// SetDocument 以只读方式打开的文件作为数据来源。
func (a *App) SetDocument(doc *hdf5.Document) error {
	a.doc = doc
	if doc == nil {
		return a.SetSource(nil)
	}
	return a.SetSource(docSource{doc: doc})
}

// Document 返回当前选中节点所属的文档；没有时返回 nil。
func (a *App) Document() *hdf5.Document {
	if a.Current == nil {
		return nil
	}
	return a.Current.Document()
}

// SetReader 覆盖表格读取来源；传 nil 恢复使用当前文档。
func (a *App) SetReader(rd Reader) { a.rd = rd }

// SetMeta 覆盖数据集描述来源；传 nil 恢复直接问节点。
func (a *App) SetMeta(m Meta) { a.meta = m }

// SetCSVExporter 注入 CSV 导出处理器；传 nil 禁用导出入口。
func (a *App) SetCSVExporter(fn func(CSVExportScope, CSVWriteFunc)) { a.csvExporter = fn }

// SetTableDetacher 注入独立窗口打开处理器；传 nil 禁用该入口。
func (a *App) SetTableDetacher(fn func()) { a.tableDetacher = fn }

// CanDetachTable 报告是否显示「独立窗口」入口。
func (a *App) CanDetachTable() bool { return a.tableDetacher != nil }

// CanExportCSV 报告当前是否可导出（已注入处理器且当前是数值表）。
func (a *App) CanExportCSV(l TableLayout) bool {
	return a.csvExporter != nil && l.OK && l.Kind == readNumeric && !l.Scalar
}

// info 返回当前对象的描述。
func (a *App) info(n *hdf5.Node) (*hdf5.DatasetInfo, error) {
	if a.meta != nil {
		return a.meta.Info(n)
	}
	if a.multiMeta != nil {
		return a.multiMeta.Info(n)
	}
	return n.Info()
}

// reader 返回当前使用的读取来源。
func (a *App) reader() Reader {
	if a.rd != nil {
		return a.rd
	}
	if a.multiRd != nil {
		return a.multiRd
	}
	if a.doc == nil {
		return nil
	}
	return a.doc
}

// HasDocument 报告是否已打开文档。
func (a *App) HasDocument() bool { return len(a.roots) > 0 }

// children 返回节点的子节点 ID，并把子节点登记进索引。
//
// 只有容器且尚未加载时才向 Source 取一次；之后复用缓存。
// 叶子节点直接返回 nil，因此展开一个含上万数据集的组
// 不会产生上万次 Source 调用（任务 3.6 的常数级断言）。
func (a *App) children(id hdf5.NodeID) []hdf5.NodeID {
	n := a.nodes[id]
	if n == nil || !n.IsContainer() {
		return nil
	}
	if a.loaded[id] {
		if ids := a.kids[id]; len(ids) > 0 {
			return ids
		}
		return nil
	}
	a.loaded[id] = true
	a.LoadCalls++
	kids, err := a.src.Children(n)
	if err != nil {
		a.Err = err
		return nil
	}
	ids := make([]hdf5.NodeID, 0, len(kids))
	for _, k := range kids {
		a.nodes[k.ID] = k
		ids = append(ids, k.ID)
	}
	a.kids[id] = ids
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// visibleIDs 按当前展开集合列出可见节点，顺序与 Outline 的行一致。
func (a *App) visibleIDs() []hdf5.NodeID {
	var out []hdf5.NodeID
	var walk func(ids []hdf5.NodeID, depth int)
	walk = func(ids []hdf5.NodeID, depth int) {
		if depth > maxDepth {
			return
		}
		for _, id := range ids {
			out = append(out, id)
			if a.Tree.Open.Has(id) {
				walk(a.children(id), depth+1)
			}
		}
	}
	walk(a.roots, 0)
	return out
}

// rowOf 返回节点当前所在的行号；不可见时返回 -1。
func (a *App) rowOf(id hdf5.NodeID) int {
	for i, v := range a.visibleIDs() {
		if v == id {
			return i
		}
	}
	return -1
}

// applyPending 把等待定位的节点换算成行号。
//
// 必须在 Outline 构建之前调用：框架在构建时才重排行，
// 所以行号要由本层按同一套展开规则自己算一遍。
func (a *App) applyPending() {
	if a.pending == nil {
		return
	}
	a.TreeRow = a.rowOf(*a.pending)
	a.pending = nil
}

// syncRow 把框架写回的行选择映射成当前对象。
func (a *App) syncRow() {
	if a.TreeRow < 0 || a.TreeRow >= a.Tree.Rows() {
		// 行已不存在（祖先被折叠）：保留当前对象，只是不再高亮。
		return
	}
	id := a.Tree.Item(a.TreeRow)
	n := a.nodes[id]
	if n == nil {
		return
	}
	prev := a.Current
	a.Selected = &n.ID
	a.Current = n
	if prev == nil || prev.ID != n.ID {
		// 换对象：清空表格缓存与分页，避免显示上一个数据集的数据。
		a.resetTable()
		a.resetImage()
	}
}

// Select 把节点设为当前对象，并展开它的祖先使其可见。
//
// 当前对象立即更新，不依赖框架的回调：框架只负责把行高亮到同一处。
// 这样即使目标行因为预算或折叠而不可见，右侧仍显示该对象。
func (a *App) Select(id hdf5.NodeID) {
	n := a.nodes[id]
	if n == nil {
		return
	}
	a.openAncestors(n.Path)
	a.Tree.Generation++
	prev := a.Current
	a.Selected = &n.ID
	a.Current = n
	if prev == nil || prev.ID != n.ID {
		a.resetTable()
		a.resetImage()
	}
	a.pending = &n.ID
}

// SelectPath 按完整路径定位并选中节点。
func (a *App) SelectPath(p string) error {
	n, err := a.find(p)
	if err != nil {
		return err
	}
	a.Select(n.ID)
	return nil
}

// find 按完整路径查找节点，沿途按需加载子节点。
func (a *App) find(p string) (*hdf5.Node, error) {
	if len(a.roots) == 0 {
		return nil, fmt.Errorf("%w：没有打开的文档", hdf5.ErrNotFound)
	}
	cur := a.roots[0]
	if a.Current != nil {
		// 多文件统一树：按当前选中节点所属文件查找，避免路径只在第一个文件生效。
		for _, id := range a.roots {
			r := a.nodes[id]
			if r != nil && r.ID.File == a.Current.ID.File {
				cur = id
				break
			}
		}
	}
	for _, seg := range splitPath(p) {
		n := a.nodes[cur]
		if n == nil || !n.IsContainer() {
			return nil, fmt.Errorf("%w：路径 %s 不存在", hdf5.ErrNotFound, p)
		}
		found := false
		var next hdf5.NodeID
		for _, id := range a.children(cur) {
			if k := a.nodes[id]; k != nil && k.Name == seg {
				next, found = id, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w：路径 %s 不存在", hdf5.ErrNotFound, p)
		}
		cur = next
	}
	n := a.nodes[cur]
	if n == nil {
		return nil, fmt.Errorf("%w：路径 %s 不存在", hdf5.ErrNotFound, p)
	}
	return n, nil
}

// openAncestors 把路径上所有祖先容器加入展开集合，但不加载目标自身。
func (a *App) openAncestors(p string) {
	if len(a.roots) == 0 {
		return
	}
	cur := a.roots[0]
	if a.Current != nil {
		for _, id := range a.roots {
			r := a.nodes[id]
			if r != nil && r.ID.File == a.Current.ID.File {
				cur = id
				break
			}
		}
	}
	a.Tree.Open.Add(cur)
	segs := splitPath(p)
	for i := 0; i < len(segs)-1; i++ {
		found := false
		var next hdf5.NodeID
		for _, id := range a.children(cur) {
			if k := a.nodes[id]; k != nil && k.Name == segs[i] {
				next, found = id, true
				break
			}
		}
		if !found {
			return
		}
		a.Tree.Open.Add(next)
		cur = next
	}
}

// ExpandAll 展开全部组节点，根节点保持可见。
//
// 只加载到 expandBudget 个节点为止；超过预算时保留已展开的部分，
// 避免一个超宽文件把界面拖死。
func (a *App) ExpandAll() {
	if len(a.roots) == 0 {
		return
	}
	a.Tree.Open.Clear()
	queue := append([]hdf5.NodeID(nil), a.roots...)
	visited := 0
	for len(queue) > 0 && visited < expandBudget {
		id := queue[0]
		queue = queue[1:]
		n := a.nodes[id]
		if n == nil || !n.IsContainer() {
			continue
		}
		a.Tree.Open.Add(id)
		queue = append(queue, a.children(id)...)
		visited++
	}
	a.Tree.Generation++
}

// CollapseAll 折叠全部节点，只留下根节点。
func (a *App) CollapseAll() {
	if len(a.roots) == 0 {
		return
	}
	a.Tree.Open.Clear()
	a.Tree.Generation++
}

// splitPath 把 "/a/b" 拆成 ["a", "b"]；根路径返回空切片。
func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// segments 返回用于面包屑展示的路径分段，不含根。
func segments(p string) []string { return splitPath(p) }
