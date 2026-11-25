package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// crumbH 是面包屑条的高度（设计稿的 Work Head）。
const crumbH = 44

// Breadcrumb 显示当前对象的完整路径，每一级是可点击的分段。
//
// 点击某一段会把该祖先展开并选中（任务 3.4）：
// 分段索引 n 对应路径的前 n+1 段，根路径单独作为第 0 段显示。
func Breadcrumb(c *ui.Context, app *App) {
	if !app.HasDocument() {
		return
	}
	cur := app.Current
	if cur == nil {
		return
	}
	segs := segments(cur.Path)

	bar := ui.Row(c).Shrink(0).Height(crumbH).AlignItems(ui.Center).
		Padding(0, 20).Gap(6)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
	bar.Children(func() {
		crumbButton(c, "/", 12, ColMute, func() { selectPath(app, "/") })
		for i, seg := range segs {
			i, seg := i, seg
			crumbSep(c)
			last := i == len(segs)-1
			col := ColDim
			if last {
				col = ColText
			}
			crumbButton(c, seg, 12, col, func() {
				selectPath(app, "/"+joinSegs(segs[:i+1]))
			})
		}
		ui.Spacer(c).Grow(1)
		ui.Text(c, "Ctrl+O").Font(FontMono).FontSize(10.5).TextColor(ColFaint).SingleLine()
		ui.Icon(c, IconCopy).Size(13, 13).TextColor(ColMute)
		ui.Icon(c, IconMore).Size(14, 14).TextColor(ColMute)
	})
}

// crumbButton 是一个可点的路径分段：文字本身精确等于分段名，
// 便于无障碍标签与点击命中。
func crumbButton(c *ui.Context, label string, size float32, col ui.Color, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Padding(2, 4).Radius(4)
	b.Children(func() {
		ui.Text(c, label).Font(FontMono).FontSize(size).TextColor(col).SingleLine()
	})
	b.OnClick(onClick)
	return b
}

// crumbSep 画分段之间的 "›" 分隔符。
func crumbSep(c *ui.Context) {
	ui.Text(c, "›").FontSize(12).TextColor(ui.Hex("#3A4046")).SingleLine()
}

func selectPath(app *App, target string) {
	if err := app.SelectPath(target); err != nil {
		app.Err = err
	}
}

// joinSegs 用 "/" 连接路径分段。
func joinSegs(segs []string) string {
	out := ""
	for i, s := range segs {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}

// PathText 返回当前对象的完整路径文本，供状态栏复用。
func PathText(n *hdf5.Node) string {
	if n == nil {
		return ""
	}
	return n.Path
}
