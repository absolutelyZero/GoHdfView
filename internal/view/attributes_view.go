package view

import (
	"github.com/egoist/mygo/ui"
)

// Attributes 构建独立的属性视图（右侧「属性」标签用）。
//
// 元数据视图内部也会带一份属性列表；这里单独成视图是为了
// 让「属性」可以作为标签独立切换（viewer-shell 的视图切换）。
func Attributes(c *ui.Context, app *App) {
	n := app.Current
	if n == nil {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "请选择一个对象").TextColor(c.Theme().TextMuted)
		})
		return
	}
	ui.Scroll(c).Children(func() {
		ui.Column(c).Padding(16).Gap(12).Children(func() {
			ui.Text(c, n.Path).FontSize(15).Bold().SingleLine()
			ui.Text(c, n.Kind.String()).FontSize(12).TextColor(c.Theme().TextMuted)
			AttributeList(c, app, n)
		})
	})
}
