package view

import (
	"sync"

	"github.com/egoist/mygo/ui"
)

// A · 仪器（ui/ui.pen）设计稿的色板。界面代码只引用这里的常量，
// 便于整体调色而不触碰任何业务逻辑。
var (
	ColBg       = ui.Hex("#0D0F11") // 工作区底色
	ColPanel    = ui.Hex("#101316") // 顶栏 / 侧栏 / 状态栏
	ColPanel2   = ui.Hex("#12161A") // 面板上的浮层
	ColLine     = ui.Hex("#1E2226") // 主要分隔线
	ColLineSoft = ui.Hex("#171B1F") // 次要分隔线
	ColText     = ui.Hex("#E7EAED")
	ColName     = ui.Hex("#C6CBD1") // 树行名称
	ColDim      = ui.Hex("#9AA1A8")
	ColMute     = ui.Hex("#6E767D")
	ColFaint    = ui.Hex("#4C545C")
	ColAccent   = ui.Hex("#E0A64A") // 琥珀强调色
	ColSel      = ui.Hex("#1B2024")
	ColChip     = ui.Hex("#151A1E")
	ColChipLine = ui.Hex("#232A2F")
	ColHeadBg   = ui.Hex("#0C0F11") // 表头底色
	ColRowLine  = ui.Hex("#161B1F") // 表格行分隔线
	ColCell     = ui.Hex("#BCC3C9") // 单元格文字
	ColGutter   = ui.Hex("#565E66") // 表头行号槽
	ColWarnBg   = ui.Hex("#1C1610")
)

// FontMono 是设计稿的等宽字体，缺字时回退到 Consolas。
const FontMono = "JetBrains Mono, Consolas, monospace"

var (
	themeOnce sync.Once
	themeDark *ui.Theme
)

// Theme 返回 A · 仪器的深色主题：界面各处只调用 c.SetTheme(Theme())，
// 让 mygo 自带控件（输入框、下拉框、滚动条、面包屑）也落在同一套配色上。
func Theme() *ui.Theme {
	themeOnce.Do(func() {
		t := ui.DarkTheme()
		t.Background = ColBg
		t.Surface = ColPanel2
		t.SurfaceHover = ui.Hex("#1B2126")
		t.SurfacePressed = ui.Hex("#232A2F")
		t.Border = ColLine
		t.Text = ColText
		t.TextMuted = ColDim
		// 列表与树的选中行直接取主题强调色，因此这里放设计稿的选中底色；
		// 琥珀色强调由界面代码显式使用 ColAccent。
		t.Accent = ColSel
		t.AccentHover = ui.Hex("#232A2F")
		t.AccentPressed = ui.Hex("#2A3036")
		t.AccentText = ColText
		t.Selection = ui.RGBA(224, 166, 74, 0.22)
		t.Focus = ui.RGBA(224, 166, 74, 0.55)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.20)
		t.Radius = 5
		t.Spacing = 4
		t.FontSize = 13
		themeDark = t
	})
	return themeDark
}

// 界面用的细线图标：描边取 currentColor，尺寸由调用方指定。
var (
	IconChevron     = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 6l6 6l-6 6"/></svg>`))
	IconChevronDown = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 9l6 6l6-6"/></svg>`))
	IconFolder      = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.7" d="M3.5 7.5h5.2l1.8 2h9.9v9.2a1 1 0 0 1-1 1H4.5a1 1 0 0 1-1-1z"/></svg>`))
	IconDataset     = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.7" d="M4 5.5h16v13H4zM4 10h16M9.5 10v8.5"/></svg>`))
	IconType        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.7" d="M5 7h14M12 7v10M9 17h6"/></svg>`))
	IconLink        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="M10 14l4-4M9 15l-2 2a3 3 0 0 1-4-4l2-2M15 9l2-2a3 3 0 0 1 4 4l-2 2"/></svg>`))
	IconTable       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.7" d="M4 5.5h16v13H4zM4 10h16M10 10v8.5"/></svg>`))
	IconImage       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.7" d="M4 5.5h16v13H4zM4 15l4.5-4.5 3.5 3.5 3-3L20 15"/></svg>`))
	IconExpand      = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="m7 20 5-5 5 5M7 4l5 5 5-5"/></svg>`))
	IconCollapse    = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.7" d="m7 15 5 5 5-5M7 9l5-5 5 5"/></svg>`))
	IconCopy        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.7" d="M9 9h10v10H9zM15 5H5v10"/></svg>`))
	IconMore        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="5.5" cy="12" r="1.4" fill="currentColor"/><circle cx="12" cy="12" r="1.4" fill="currentColor"/><circle cx="18.5" cy="12" r="1.4" fill="currentColor"/></svg>`))
	IconOpen        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linejoin="round" stroke-width="1.8" d="M3.5 8.5h6l1.8 2h9.2v8a1 1 0 0 1-1 1H4.5a1 1 0 0 1-1-1z"/></svg>`))
	IconSearch      = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="2" d="M14.5 14.5L20 20m-4-10a6 6 0 1 1-12 0a6 6 0 0 1 12 0Z"/></svg>`))
	IconInfo        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.7" d="M12 11v6M12 7.6v.1"/><circle cx="12" cy="12" r="8.5" fill="none" stroke="currentColor" stroke-width="1.7"/></svg>`))
	IconLocate      = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="6" fill="none" stroke="currentColor" stroke-width="1.7"/><path fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.7" d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3"/></svg>`))
)

// Hairline 在矩形底边画一条 1px 分隔线。
func Hairline(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, c, 0)
}

// VLine 在矩形右边画一条 1px 分隔线。
func VLine(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, c, 0)
}

// GhostButton 是设计稿里的描边小按钮。
func GhostButton(c *ui.Context, label string, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Padding(4, 10).Radius(5).Border(1, ColLine)
	b.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(ColDim).SingleLine()
	})
	if onClick != nil {
		b.OnClick(onClick)
	}
	return b
}

// AccentButton 是设计稿里的琥珀实心按钮。
func AccentButton(c *ui.Context, label string, onClick func()) ui.Element {
	b := ui.ButtonBase(c).Padding(4, 12).Radius(5).Background(ColAccent)
	b.Children(func() {
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(ColBg).SingleLine()
	})
	if onClick != nil {
		b.OnClick(onClick)
	}
	return b
}

// Chip 是标题旁的类型标签。
func Chip(c *ui.Context, label string) ui.Element {
	e := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(2, 7).Radius(3).
		Background(ColChip).Border(1, ColChipLine)
	e.Children(func() {
		ui.Text(c, label).Font(FontMono).FontSize(10.5).TextColor(ColDim).SingleLine()
	})
	return e
}
