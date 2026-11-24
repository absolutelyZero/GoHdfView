package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// glyph 返回节点种类的单字符标记，用于在树中区分类型。
//
// 不用 SVG 图标：每个可见行都构造一份图标代价过高，
// 而单字符标记同样能通过颜色与形状区分种类。
func glyph(k hdf5.NodeKind) string {
	switch k {
	case hdf5.KindGroup:
		return "▣"
	case hdf5.KindDataset:
		return "▦"
	case hdf5.KindNamedType:
		return "◈"
	case hdf5.KindSoftLink:
		return "↗"
	case hdf5.KindExternalLink:
		return "⧉"
	default:
		return "·"
	}
}

// Tree 构建左栏的对象树。
//
// 行内容只有可见行才会被构建（Outline 的语义），
// 因此一万个成员的组也只画屏内的十几行。
func Tree(c *ui.Context, app *App) {
	if !app.HasDocument() {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "尚未打开文件").TextColor(c.Theme().TextMuted)
		})
		return
	}

	// 行号必须按本层的展开规则先算好：框架在构建时才重排行。
	app.applyPending()
	app.Tree.List.Selected = &app.TreeRow
	app.Tree.List.Label = func(row int) string {
		if row < 0 || row >= app.Tree.Rows() {
			return ""
		}
		if n := app.nodes[app.Tree.Item(row)]; n != nil {
			return n.Name
		}
		return ""
	}

	el := ui.Outline(c, &app.Tree, app.roots, app.children, func(id hdf5.NodeID) {
		n := app.nodes[id]
		if n == nil {
			return
		}
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, glyph(n.Kind)).TextColor(c.Theme().TextMuted).SingleLine()
			ui.Text(c, n.Name).SingleLine()
			if s := n.DimensionSummary(); s != "" {
				ui.Text(c, s).TextColor(c.Theme().TextMuted).FontSize(12).SingleLine()
			}
			if n.LinkInfo != "" {
				ui.Text(c, "→ "+n.LinkInfo).TextColor(c.Theme().TextMuted).FontSize(12).SingleLine()
			}
		})
	})
	el.Grow(1)

	// 点击或方向键改变选择后，框架把行号写回 app.TreeRow，
	// 这里再把它映射成当前对象。
	if el.Changed() {
		app.syncRow()
	}
}
