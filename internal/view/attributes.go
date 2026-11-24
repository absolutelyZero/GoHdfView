package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// attrValueLimit 是属性值在列表内的显示上限（字符数）。
//
// 超过上限时截断并提示可在详情中查看完整值，
// 避免一个长数组把行高撑开（dataset-metadata 的第二个 scenario）。
const attrValueLimit = 120

// AttributeList 构建属性列表（任务 4.4）。
//
// 属性只读：这里不提供任何编辑、新增或删除入口，
// 点击行只会选中以便复制（dataset-metadata 的「属性不可编辑」）。
func AttributeList(c *ui.Context, app *App, n *hdf5.Node) {
	attrs, err := n.Attributes()
	if err != nil {
		app.Err = err
		section(c, "属性", func() {
			ui.Text(c, "读取属性失败："+err.Error()).TextColor(c.Theme().TextMuted)
		})
		return
	}
	section(c, "属性", func() {
		if len(attrs) == 0 {
			ui.Text(c, "无属性").TextColor(c.Theme().TextMuted)
			return
		}
		for _, a := range attrs {
			attributeRow(c, app, a)
		}
	})
}

// attributeRow 画一行属性：名称、类型、值。
func attributeRow(c *ui.Context, app *App, a hdf5.AttributeInfo) {
	ui.Column(c).Gap(2).Padding(4, 0).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.Name).Bold().FontSize(12).SingleLine()
			ui.Text(c, a.TypeName).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
		})
		if a.Error != "" {
			ui.Text(c, "读取失败："+a.Error).FontSize(12).TextColor(c.Theme().TextMuted)
			return
		}
		text, truncated := truncateValue(a.Value)
		ui.Text(c, text).FontSize(12).SingleLine()
		if truncated {
			ui.Text(c, "值过长，已截断（共 "+itoa(a.ElemNum)+" 个元素）").FontSize(11).TextColor(c.Theme().TextMuted)
		}
	})
}

// truncateValue 按字符数截断属性值。
func truncateValue(v string) (string, bool) {
	runes := []rune(v)
	if len(runes) <= attrValueLimit {
		return v, false
	}
	return string(runes[:attrValueLimit]) + "…", true
}
