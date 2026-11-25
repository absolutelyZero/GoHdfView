package view

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// 侧栏筛选框的内容与上一次生效的值都是纯界面状态，不参与文档模型。
var (
	treeFilter  string
	treeApplied string
	treeSelID   hdf5.NodeID
)

// Tree 画 A · 仪器风格的侧栏对象树：筛选框 + 26px 行高的树，
// 行内依次是展开箭头、类型图标、名称与维度概要。
func Tree(c *ui.Context, app *App) {
	if !app.HasDocument() {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "尚未打开文件").FontSize(12).TextColor(ColMute)
		})
		return
	}
	app.applyPending()

	treeSearch(c, app)
	if treeFilter != treeApplied {
		treeApplied = treeFilter
		app.Tree.Generation++
	}

	// 记住当前选中的节点，筛选增删行之后按节点而不是行号恢复选中。
	if app.TreeRow >= 0 && app.TreeRow < app.Tree.Rows() {
		treeSelID = app.Tree.Item(app.TreeRow)
	}
	rows := treeRows(app)
	app.TreeRow = -1
	for i, id := range rows {
		if id == treeSelID {
			app.TreeRow = i
			break
		}
	}

	app.Tree.List.Selected = &app.TreeRow
	app.Tree.List.Label = func(row int) string {
		n := app.nodes[app.Tree.Item(row)]
		if n == nil {
			return ""
		}
		return n.Name
	}

	row := -1
	app.Tree.Prefix = func(r ui.OutlineRow) {
		row++
		on := row == app.TreeRow
		// 设计稿的缩进：基础 10px，每层加 15px，箭头与图标之间各留 6px。
		// 行高由这个 26px 的占位撑起：List 的 RowHeight 只作用于表格。
		ui.Box(c).Width(float32(10 + r.Depth*15)).Height(26).Shrink(0)
		if r.Branch {
			a := r.Arrow()
			a.Size(11, 11).Shrink(0).Children(func() {
				ic := IconChevron
				if r.Open {
					ic = IconChevronDown
				}
				ui.Icon(c, ic).Size(11, 11).TextColor(pick(on, ColText, ColMute))
			})
		} else {
			ui.Box(c).Width(11).Height(11).Shrink(0)
		}
		ui.Box(c).Width(6).Height(1).Shrink(0)
		if n := app.nodes[app.Tree.Item(row)]; n != nil {
			kindIcon(c, n, pick(on, ColText, ColMute))
		}
		ui.Box(c).Width(6).Height(1).Shrink(0)
	}

	ui.Column(c).Fill().Padding(6, 0, 10, 0).Children(func() {
		el := ui.Outline(c, &app.Tree, filteredRoots(app), func(id hdf5.NodeID) []hdf5.NodeID {
			return filterKids(app, id)
		}, func(id hdf5.NodeID) {
			n := app.nodes[id]
			if n == nil {
				return
			}
			on := row == app.TreeRow
			ui.Text(c, n.Name).FontSize(12.5).TextColor(pick(on, ColText, ColName)).SingleLine()
			if s := n.DimensionSummary(); s != "" {
				ui.Spacer(c)
				ui.Text(c, s).Font(FontMono).FontSize(10.5).TextColor(ColMute).SingleLine()
			}
			if n.LinkInfo != "" {
				ui.Text(c, "→ "+n.LinkInfo).FontSize(11).TextColor(ColMute).SingleLine()
			}
			ui.Box(c).Width(10).Shrink(0)
		})
		el.Grow(1)
		if el.Changed() {
			app.syncRow()
		}
	})
}

// treeSearch 画侧栏顶部的筛选框，筛选只影响可见行。
func treeSearch(c *ui.Context, app *App) {
	wrap := ui.Column(c).Shrink(0).Padding(10)
	wrap.Children(func() {
		row := ui.Row(c).Shrink(0).Height(32).AlignItems(ui.Center).Padding(0, 10).Gap(8).
			Background(ui.Hex("#0B0E10")).Radius(5).Border(1, ColLine)
		row.Children(func() {
			ui.Icon(c, IconSearch).Size(12, 12).TextColor(ColMute)
			in := ui.TextInputBase(c, &treeFilter)
			in.Grow(1).FontSize(12).TextColor(ColText).Placeholder("按名称筛选").
				PlaceholderColor(ColMute)
			mod, hint := ui.Ctrl, "Ctrl K"
			if runtime.GOOS == "darwin" {
				mod, hint = ui.Cmd, "⌘K"
			}
			ui.Text(c, hint).Font(FontMono).FontSize(10.5).TextColor(ColMute).SingleLine()
			if c.Shortcut(mod, ui.KeyK) {
				in.Focus()
			}
		})
	})
}

// treeRows 返回当前可见的行（筛选时只保留匹配项及其祖先）。
func treeRows(app *App) []hdf5.NodeID {
	if strings.TrimSpace(treeFilter) == "" {
		return app.visibleIDs()
	}
	var out []hdf5.NodeID
	for _, id := range filteredRoots(app) {
		walkFiltered(app, id, 0, &out)
	}
	return out
}

func walkFiltered(app *App, id hdf5.NodeID, depth int, out *[]hdf5.NodeID) {
	*out = append(*out, id)
	if depth > 64 || !app.Tree.Open.Has(id) {
		return
	}
	for _, kid := range filterKids(app, id) {
		walkFiltered(app, kid, depth+1, out)
	}
}

// filteredRoots 在筛选时只保留名称匹配的根与含匹配后代的根。
func filteredRoots(app *App) []hdf5.NodeID {
	if strings.TrimSpace(treeFilter) == "" {
		return app.roots
	}
	var out []hdf5.NodeID
	for _, id := range app.roots {
		if subtreeMatches(app, id, 0) {
			out = append(out, id)
		}
	}
	return out
}

// filterKids 在筛选时只返回匹配的分支与直接匹配的叶子。
func filterKids(app *App, id hdf5.NodeID) []hdf5.NodeID {
	kids := app.children(id)
	if strings.TrimSpace(treeFilter) == "" {
		return kids
	}
	var out []hdf5.NodeID
	for _, kid := range kids {
		if n := app.nodes[kid]; n != nil && (matchName(n.Name) || subtreeMatches(app, kid, 1)) {
			out = append(out, kid)
		}
	}
	return out
}

// subtreeMatches 报告 id 的子树里是否有名称匹配的节点，用于保留祖先行。
func subtreeMatches(app *App, id hdf5.NodeID, depth int) bool {
	if depth > 3 {
		return false
	}
	if n := app.nodes[id]; n != nil && matchName(n.Name) {
		return true
	}
	for _, kid := range app.children(id) {
		if subtreeMatches(app, kid, depth+1) {
			return true
		}
	}
	return false
}

// kindIcon 用设计稿的 12×12 线条图标表示节点类型：
// 组是 folder，二维数据集是 image，其余数据集是 table-2。
func kindIcon(c *ui.Context, n *hdf5.Node, col ui.Color) ui.Element {
	ic := IconTable
	switch n.Kind {
	case hdf5.KindGroup:
		ic = IconFolder
	case hdf5.KindDataset:
		if n.Rank == 2 {
			ic = IconImage
		}
	case hdf5.KindNamedType:
		ic = IconType
	case hdf5.KindSoftLink, hdf5.KindExternalLink:
		ic = IconLink
	}
	return ui.Icon(c, ic).Size(12, 12).TextColor(col)
}

func pick(on bool, a, b ui.Color) ui.Color {
	if on {
		return a
	}
	return b
}

// matchName 判断名称是否满足侧栏筛选条件。
func matchName(name string) bool {
	q := strings.TrimSpace(treeFilter)
	return q == "" || strings.Contains(strings.ToLower(name), strings.ToLower(q))
}
