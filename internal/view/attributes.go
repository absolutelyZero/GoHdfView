package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// attrValueLimit 是属性值在列表里显示的最大字符数。
const attrValueLimit = 120

// AttributeList 列出对象上的属性（任务 4.4）。
func AttributeList(c *ui.Context, app *App, n *hdf5.Node) {
	attrs, err := n.Attributes()
	if err != nil {
		app.Err = err
		section(c, "属性", func() {
			ui.Text(c, "读取属性失败："+err.Error()).FontSize(11.5).TextColor(ColMute)
		})
		return
	}
	if len(attrs) == 0 {
		section(c, "属性", func() {
			ui.Text(c, "无属性").FontSize(11.5).TextColor(ColMute).SingleLine()
		})
		return
	}
	section(c, "属性", func() {
		for _, a := range attrs {
			attributeRow(c, a)
		}
	})
}

// attributeRow 画一条属性：名称、类型与值。
func attributeRow(c *ui.Context, a hdf5.AttributeInfo) {
	box := ui.Column(c).Shrink(0).Gap(3).Padding(6, 10).Radius(4).
		Background(ColPanel2)
	box.Children(func() {
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.Name).FontSize(12).FontWeight(600).TextColor(ColText).SingleLine()
			ui.Text(c, a.TypeName).Font(FontMono).FontSize(10.5).
				TextColor(ColMute).SingleLine()
		})
		if a.Error != "" {
			ui.Text(c, "读取失败："+a.Error).FontSize(11.5).TextColor(ColMute).SingleLine()
			return
		}
		text, truncated := truncateValue(a.Value)
		ui.Text(c, text).Font(FontMono).FontSize(11.5).TextColor(ColCell).SingleLine()
		if truncated {
			ui.Text(c, "值过长，已截断（共 "+itoa(a.ElemNum)+" 个元素）").
				FontSize(10.5).TextColor(ColFaint).SingleLine()
		}
	})
}

// truncateValue 按字符截断过长的属性值。
func truncateValue(s string) (string, bool) {
	r := []rune(s)
	if len(r) <= attrValueLimit {
		return s, false
	}
	return string(r[:attrValueLimit]) + "…", true
}
