package view

import (
	"github.com/egoist/mygo/ui"
)

// Attributes 是「属性」标签页的完整视图。
func Attributes(c *ui.Context, app *App) {
	n := app.Current
	if n == nil {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "请选择一个对象").FontSize(12).TextColor(ColMute)
		})
		return
	}
	scroll := ui.Scroll(c).Grow(1)
	scroll.Children(func() {
		body := ui.Column(c).Padding(18, 20).Gap(16)
		body.Children(func() {
			head := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(8)
			head.Children(func() {
				ui.Text(c, n.Path).FontSize(13).TextColor(ColText).SingleLine()
				Chip(c, n.Kind.String())
			})
			AttributeList(c, app, n)
		})
	})
}
