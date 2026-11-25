package view

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// ImageView 构建图像视图（任务 6.1–6.3）。
func ImageView(c *ui.Context, app *App) {
	l := app.ImageLayoutFor()
	if !l.OK {
		ui.Column(c).Fill().Center().Padding(24).Gap(6).Children(func() {
			ui.Text(c, "图像视图不可用").FontSize(13).TextColor(ColText).SingleLine()
			ui.Text(c, l.Reason).FontSize(12).TextColor(ColMute)
		})
		return
	}

	app.loadImage(l)
	ui.Column(c).Fill().Children(func() {
		imageToolbar(c, app, l)
		if app.Image.err != nil {
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 20).Gap(8).
				Background(ColWarnBg).Children(func() {
				ui.Text(c, "⚠").FontSize(12).TextColor(ColAccent).SingleLine()
				ui.Text(c, "图像读取失败："+app.Image.err.Error()).
					FontSize(11.5).TextColor(ColDim).Grow(1).SingleLine()
			})
		}
		imageBody(c, app, l)
		colorbar(c, app)
	})
}

// imageToolbar 提供值域覆盖输入（任务 6.2）。
func imageToolbar(c *ui.Context, app *App, l ImageLayout) {
	stepRow, stepCol := downsampleStep(l.Rows, l.Cols)
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 20).Gap(8)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
	bar.Children(func() {
		ui.Text(c, "最小值").FontSize(11).TextColor(ColMute).SingleLine()
		ui.TextInput(c, &app.Image.MinText).Width(104).FontSize(11.5)
		ui.Text(c, "最大值").FontSize(11).TextColor(ColMute).SingleLine()
		ui.TextInput(c, &app.Image.MaxText).Width(104).FontSize(11.5)
		GhostButton(c, "重置值域", func() {
			app.Image.MinText = ""
			app.Image.MaxText = ""
			app.Image.key = "" // 强制重建
		})
		ui.Spacer(c).Grow(1)
		ui.Textf(c, "%d × %d（步长 %d×%d，共 %d 像素）",
			l.Rows, l.Cols, stepRow, stepCol, app.Image.pixels).
			Font(FontMono).FontSize(10.5).TextColor(ColMute).SingleLine()
	})
}

// imageBody 显示位图。
func imageBody(c *ui.Context, app *App, l ImageLayout) {
	if app.Image.bitmap == nil {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "正在读取…").FontSize(12).TextColor(ColMute)
		})
		return
	}
	w, h := app.Image.bitmap.Size()
	dw, dh := displaySize(w, h, imageDisplaySide)
	ui.Column(c).Fill().Grow(1).Center().Children(func() {
		// Pixelated 让放大保持最近邻，格点边界清晰可辨。
		ui.Image(c, app.Image.bitmap.Pixelated()).Width(dw).Height(dh)
	})
}

// imageDisplaySide 是图像在窗口里最长边的目标像素数。
//
// 小数据集（例如 100×50）直接按原尺寸画会缩成一小块，
// 所以放大到便于观察的尺寸；放大用最近邻，不引入插值。
const imageDisplaySide = 560

// displaySize 在保持长宽比的前提下把图像缩放到最长边为 side。
func displaySize(w, h, side int) (float32, float32) {
	if w <= 0 || h <= 0 {
		return float32(side), float32(side)
	}
	longest := max(w, h)
	scale := float64(side) / float64(longest)
	return float32(math.Round(float64(w) * scale)), float32(math.Round(float64(h) * scale))
}

// colorbar 画一条从最小值到最大值的灰阶色标（design.md 决策 5：
// 色标用文本与背景色块，不引入图表控件）。
func colorbar(c *ui.Context, app *App) {
	if !app.Image.hasRange {
		return
	}
	lo, hi := app.imageRange(app.Image.minVal, app.Image.maxVal)
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(8, 20).Gap(8)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
	bar.Children(func() {
		ui.Text(c, formatFloatValue(lo)).Font(FontMono).FontSize(10.5).
			TextColor(ColMute).SingleLine()
		// 16 级灰阶色块拼成色标。
		strip := ui.Row(c).Shrink(0).AlignItems(ui.Stretch).Radius(2)
		strip.Draw(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, ColLine, 2, 1) })
		strip.Children(func() {
			for i := 0; i < 16; i++ {
				g := uint8(i * 17)
				ui.Box(c).Width(14).Height(12).Background(ui.RGB(g, g, g))
			}
		})
		ui.Text(c, formatFloatValue(hi)).Font(FontMono).FontSize(10.5).
			TextColor(ColMute).SingleLine()
	})
}

// formatFloatValue 格式化色标端点值。
func formatFloatValue(v float64) string {
	return strconv.FormatFloat(v, 'g', 6, 64)
}
