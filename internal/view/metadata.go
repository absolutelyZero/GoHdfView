package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// Metadata 展示选中对象的元数据（任务 4.1–4.4）。
func Metadata(c *ui.Context, app *App) {
	n := app.Current
	if n == nil {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "请选择一个对象").FontSize(12).TextColor(ColMute)
		})
		return
	}

	scroll := ui.Scroll(c).Grow(1)
	scroll.Children(func() {
		body := ui.Column(c).Padding(18, 20).Gap(20)
		body.Children(func() {
			head := ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(8)
			head.Children(func() {
				ui.Text(c, n.Path).FontSize(13).TextColor(ColText).SingleLine()
				Chip(c, n.Kind.String())
			})
			switch n.Kind {
			case hdf5.KindGroup:
				groupMetadata(c, app, n)
			case hdf5.KindDataset:
				datasetMetadata(c, app, n)
			default:
				otherMetadata(c, n)
			}
			AttributeList(c, app, n)
		})
	})
}

// section 画一个带标题的元数据区块。
func section(c *ui.Context, title string, body func()) {
	box := ui.Column(c).Shrink(0).Gap(8)
	box.Children(func() {
		ui.Text(c, title).Font(FontMono).FontSize(10.5).TextColor(ColMute).
			LetterSpacing(0.6).SingleLine()
		ui.Column(c).Shrink(0).Gap(6).Children(body)
	})
}

// row 画一行「名称 — 值」。名称列固定宽度，值列占满剩余宽度。
func row(c *ui.Context, name, value string) {
	ui.Row(c).Shrink(0).AlignItems(ui.Start).Gap(12).Children(func() {
		ui.Text(c, name).Width(132).Shrink(0).FontSize(11.5).TextColor(ColMute).SingleLine()
		ui.Text(c, value).Grow(1).FontSize(11.5).TextColor(ColCell).SingleLine()
	})
}

// groupMetadata 显示组信息。
func groupMetadata(c *ui.Context, app *App, n *hdf5.Node) {
	info, err := n.GroupInfo()
	if err != nil {
		app.Err = err
		section(c, "组信息", func() {
			ui.Text(c, "读取失败："+err.Error()).FontSize(11.5).TextColor(ColMute)
		})
		return
	}
	section(c, "组信息", func() {
		row(c, "对象地址", addressText(n.ID.Address))
		row(c, "成员数", itoa(info.MemberCount))
		row(c, "属性数", itoa(info.AttributeCount))
	})
}

// datasetMetadata 显示数据空间、数据类型与存储布局（任务 4.2–4.3）。
func datasetMetadata(c *ui.Context, app *App, n *hdf5.Node) {
	info, err := n.Info()
	if err != nil {
		section(c, "数据集信息", func() {
			ui.Text(c, "读取失败："+err.Error()).FontSize(11.5).TextColor(ColMute)
		})
		return
	}
	dt := info.Datatype

	section(c, "数据空间", func() {
		row(c, "秩", itoa(info.Rank))
		row(c, "维度", orNone(hdf5.FormatDims(info.Shape)))
		row(c, "最大值", orNone(hdf5.FormatMaxDims(info.MaxShape, info.Unlimited)))
		if info.Overflow {
			row(c, "元素总数", "超出可表示范围")
		} else {
			row(c, "元素总数", itoa64(info.ElemCount))
		}
	})

	section(c, "数据类型", func() {
		row(c, "类别", dt.ClassText())
		row(c, "元素大小", dt.SizeText())
		row(c, "字节序", dt.OrderText())
		if s := dt.SignedText(); s != "" {
			row(c, "符号性", s)
		}
		if dt.IsString() {
			if dt.IsVariableString() {
				row(c, "长度", "变长")
			} else {
				row(c, "长度", "定长")
			}
			row(c, "填充方式", dt.PaddingText())
			row(c, "字符集", dt.CharsetText())
		}
		if len(dt.ArrayDims) > 0 {
			row(c, "数组维度", hdf5.FormatDims(dt.ArrayDims))
		}
		if len(dt.Members) > 0 {
			ui.Text(c, "成员").FontSize(11).TextColor(ColFaint).SingleLine()
			for _, m := range dt.Members {
				row(c, "  "+m.Name, itoa64(m.Offset)+" 字节处 · "+m.Type.Describe())
			}
		}
		if len(dt.EnumMembers) > 0 {
			ui.Text(c, "枚举成员").FontSize(11).TextColor(ColFaint).SingleLine()
			for _, m := range dt.EnumMembers {
				row(c, "  "+m.Name, itoa64(uint64(m.Value)))
			}
		}
	})

	section(c, "存储布局", func() {
		row(c, "布局", info.Layout.ClassText())
		if len(info.Layout.ChunkDims) > 0 {
			row(c, "分块尺寸", hdf5.FormatDims(info.Layout.ChunkDims))
		}
		if len(info.Layout.Filters) == 0 {
			row(c, "过滤器", "无")
			return
		}
		ui.Text(c, "过滤器（按应用顺序）").FontSize(11).TextColor(ColFaint).SingleLine()
		for i, f := range info.Layout.Filters {
			row(c, "  "+itoa(i+1), f.Describe())
		}
	})
}

// otherMetadata 显示命名类型、软链接等其它对象的信息。
func otherMetadata(c *ui.Context, n *hdf5.Node) {
	section(c, "对象信息", func() {
		row(c, "种类", n.Kind.String())
		row(c, "对象地址", addressText(n.ID.Address))
		if n.LinkInfo != "" {
			row(c, "链接目标", n.LinkInfo)
		}
	})
}

// DataAvailable 报告当前选中对象是否有数据视图（只有数据集有）。
func (a *App) DataAvailable() bool {
	return a.Current != nil && a.Current.Kind == hdf5.KindDataset
}

// addressText 格式化对象地址；解析库没给地址时明确说明。
func addressText(addr uint64) string {
	if addr == 0 {
		return "（解析库未提供地址）"
	}
	return "0x" + itoa64Hex(addr)
}

// orNone 把空串显示为占位文本。
func orNone(s string) string {
	if s == "" {
		return "（空）"
	}
	return s
}
