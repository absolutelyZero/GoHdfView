package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// section 画一个带标题的元数据区块。
func section(c *ui.Context, title string, body func()) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, title).Bold().FontSize(13)
		ui.Column(c).Gap(4).Children(body)
	})
}

// row 画一行「名称：值」。
func row(c *ui.Context, name, value string) {
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Text(c, name).Width(120).Shrink(0).TextColor(c.Theme().TextMuted).SingleLine()
		ui.Text(c, value).Grow(1).SingleLine()
	})
}

// Metadata 构建右侧的元数据视图（任务 4.1–4.3、4.5）。
//
// 组与数据集共用同一套区块结构：组显示成员数与属性，
// 数据集显示数据空间、类型、布局与过滤器。
func Metadata(c *ui.Context, app *App) {
	n := app.Current
	if n == nil {
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, "请选择一个对象").TextColor(c.Theme().TextMuted)
		})
		return
	}

	ui.Scroll(c).Children(func() {
		ui.Column(c).Padding(16).Gap(18).Children(func() {
			ui.Text(c, n.Path).FontSize(15).Bold().SingleLine()
			ui.Text(c, n.Kind.String()).FontSize(12).TextColor(c.Theme().TextMuted)

			switch n.Kind {
			case hdf5.KindGroup:
				groupMetadata(c, app, n)
			case hdf5.KindDataset:
				datasetMetadata(c, app, n)
			default:
				otherMetadata(c, app, n)
			}
			AttributeList(c, app, n)
		})
	})
}

// groupMetadata 显示组的对象地址、成员数与属性数（任务 4.5）。
func groupMetadata(c *ui.Context, app *App, n *hdf5.Node) {
	info, err := n.GroupInfo()
	if err != nil {
		app.Err = err
		section(c, "组信息", func() {
			ui.Text(c, "读取失败："+err.Error()).TextColor(c.Theme().TextMuted)
		})
		return
	}
	section(c, "组信息", func() {
		row(c, "对象地址", addressText(n.ID.Address))
		row(c, "成员数", itoa(info.MemberCount))
		row(c, "属性数", itoa(info.AttributeCount))
	})
}

// datasetMetadata 显示数据空间、类型与布局三个区块。
func datasetMetadata(c *ui.Context, app *App, n *hdf5.Node) {
	info, err := n.Info()
	if err != nil {
		app.Err = err
		section(c, "数据集信息", func() {
			ui.Text(c, "读取失败："+err.Error()).TextColor(c.Theme().TextMuted)
		})
		return
	}

	section(c, "数据空间", func() {
		row(c, "秩", itoa(info.Rank))
		row(c, "维度", orNone(hdf5.FormatDims(info.Shape)))
		row(c, "最大值", orNone(hdf5.FormatMaxDims(info.MaxShape, info.Unlimited)))
		count := itoa64(info.ElemCount)
		if info.Overflow {
			count = "超出可表示范围"
		}
		row(c, "元素总数", count)
	})

	section(c, "数据类型", func() {
		dt := info.Datatype
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
			ui.Text(c, "成员").TextColor(c.Theme().TextMuted).FontSize(12)
			for _, m := range dt.Members {
				row(c, "  "+m.Name, itoa64(m.Offset)+" 字节处 · "+m.Type.Describe())
			}
		}
		if len(dt.EnumMembers) > 0 {
			ui.Text(c, "枚举成员").TextColor(c.Theme().TextMuted).FontSize(12)
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
		ui.Text(c, "过滤器（按应用顺序）").TextColor(c.Theme().TextMuted).FontSize(12)
		for i, f := range info.Layout.Filters {
			row(c, "  "+itoa(i+1), f.Describe())
		}
	})
}

// otherMetadata 显示命名类型、链接等非组非数据集的对象的元数据。
func otherMetadata(c *ui.Context, app *App, n *hdf5.Node) {
	section(c, "对象信息", func() {
		row(c, "种类", n.Kind.String())
		row(c, "对象地址", addressText(n.ID.Address))
		if n.LinkInfo != "" {
			row(c, "链接目标", n.LinkInfo)
		}
	})
}

// DataAvailable 报告当前对象是否可显示数据视图。
//
// 组与命名类型没有数据；视图据此把「数据」标签置灰（任务 4.5）。
func (a *App) DataAvailable() bool {
	return a.Current != nil && a.Current.Kind == hdf5.KindDataset
}

func addressText(addr uint64) string {
	if addr == 0 {
		return "（解析库未提供地址）"
	}
	return "0x" + itoa64Hex(addr)
}

func orNone(s string) string {
	if s == "" {
		return "（空）"
	}
	return s
}
