package view

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// Source 是对象树的只读数据来源。
//
// 真实实现包装 *hdf5.Document；测试可以注入假实现，
// 从而在没有真实文件的情况下构造任意形状（含上万成员）的树。
type Source interface {
	// Root 返回根节点；没有文档时返回 nil。
	Root() *hdf5.Node
	// Children 返回节点的直接成员；非容器返回 nil, nil。
	Children(n *hdf5.Node) ([]*hdf5.Node, error)
}

// docSource 把 *hdf5.Document 适配成 Source。
type docSource struct{ doc *hdf5.Document }

func (d docSource) Root() *hdf5.Node { return d.doc.Root() }

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
}

// SetSource 换用新的数据来源，并重置全部树状态。
func (a *App) SetSource(src Source) error {
	a.reset()
	a.src = src
	if src == nil {
		a.doc = nil
		return nil
	}
	root := src.Root()
	if root == nil {
		return fmt.Errorf("%w：文档没有根组", hdf5.ErrCorrupt)
	}
	a.nodes[root.ID] = root
	a.roots = []hdf5.NodeID{root.ID}
	a.Tree.Open.Add(root.ID)
	a.Tree.Generation++
	a.TreeRow = 0
	a.Selected = &root.ID
	a.Current = root
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

// Document 返回当前文档；没有时返回 nil。
func (a *App) Document() *hdf5.Document { return a.doc }

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
	a.Selected = &n.ID
	a.Current = n
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
	a.Selected = &n.ID
	a.Current = n
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
