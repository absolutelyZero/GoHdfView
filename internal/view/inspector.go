package view

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// inspectorW 是右侧检查器的宽度（设计稿 280）。
const inspectorW = 280

// Inspector 画右侧检查器：概览、值域分布、属性与来源。
//
// 它只读 app 已经算好的数据，不改变任何状态；
// 没有选中对象时整栏留空。
func Inspector(c *ui.Context, app *App) {
	col := ui.Column(c).Width(inspectorW).Shrink(0).FillHeight().
		Background(ColPanel)
	col.Draw(func(p *ui.Painter, r ui.Rect) { VLine(p, r, ColLine) })
	col.Children(func() {
		head := ui.Row(c).Shrink(0).Height(38).AlignItems(ui.Center).
			Padding(0, 14).Gap(8)
		head.Draw(func(p *ui.Painter, r ui.Rect) { Hairline(p, r, ColLineSoft) })
		head.Children(func() {
			ui.Text(c, "检查器").FontSize(11).TextColor(ColMute).
				LetterSpacing(0.4).SingleLine()
			ui.Spacer(c).Grow(1)
			ui.Icon(c, IconInfo).Size(12, 12).TextColor(ColMute)
		})

		n := app.Current
		if n == nil {
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(14, 16).Gap(18).Children(func() {
				overview(c, n)
				valueRange(c, app, n)
				inspectorAttrs(c, n)
				source(c, n)
			})
		})
	})
}

// inspectorTitle 是检查器里的小节标题。
func inspectorTitle(c *ui.Context, title string) {
	ui.Text(c, title).Font(FontMono).FontSize(10.5).TextColor(ColMute).
		LetterSpacing(0.6).SingleLine()
}

// kv 是检查器里的「名称 …… 值」一行，值靠右且等宽。
func kv(c *ui.Context, name, value string) {
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, name).FontSize(11.5).TextColor(ColMute).SingleLine()
		ui.Spacer(c).Grow(1)
		ui.Text(c, value).Font(FontMono).FontSize(11.5).TextColor(ColCell).
			SingleLine()
	})
}

// overview 是数据集的概要：类型、维度、元素数、存储与压缩。
func overview(c *ui.Context, n *hdf5.Node) {
	ui.Column(c).Shrink(0).Gap(8).Children(func() {
		inspectorTitle(c, "概览")
		if n.Kind != hdf5.KindDataset {
			kv(c, "类型", n.Kind.String())
			kv(c, "路径", n.Path)
			return
		}
		info, err := n.Info()
		if err != nil {
			ui.Text(c, "读取失败："+err.Error()).FontSize(11.5).TextColor(ColMute)
			return
		}
		kv(c, "数据类型", typeLabel(info.Datatype))
		kv(c, "维度", dimsLabel(info.Shape))
		kv(c, "元素数", groupDigits(info.ElemCount))
		kv(c, "存储", info.Layout.ClassText())
		kv(c, "压缩", filtersLabel(info.Layout.Filters))
	})
}

// typeLabel 是类型标签，优先用解析库给的名字（如 float64），
// 没有名字时按类别与元素大小推出常见写法。
func typeLabel(dt hdf5.DatatypeInfo) string {
	if dt.Tag != "" {
		return dt.Tag
	}
	bits := dt.Size * 8
	switch {
	case dt.IsFloat() && bits > 0:
		return "float" + strconv.FormatUint(bits, 10)
	case dt.IsInteger() && bits > 0:
		if dt.Signed {
			return "int" + strconv.FormatUint(bits, 10)
		}
		return "uint" + strconv.FormatUint(bits, 10)
	}
	return dt.ClassText()
}

// TypeChip 返回标题旁的类型标签：数据集给数据类型，其它给种类。
func TypeChip(n *hdf5.Node) string {
	if n.Kind != hdf5.KindDataset {
		return n.Kind.String()
	}
	info, err := n.Info()
	if err != nil {
		return n.Kind.String()
	}
	return typeLabel(info.Datatype)
}

// dimsLabel 把维度写成「100 × 50」，标量写「标量」。
func dimsLabel(shape []uint64) string {
	if len(shape) == 0 {
		return "标量"
	}
	s := ""
	for i, d := range shape {
		if i > 0 {
			s += " × "
		}
		s += strconv.FormatUint(d, 10)
	}
	return s
}

// filtersLabel 把过滤器写成一行，没有时写「无」。
func filtersLabel(fs []hdf5.FilterInfo) string {
	if len(fs) == 0 {
		return "无"
	}
	s := ""
	for i, f := range fs {
		if i > 0 {
			s += " · "
		}
		s += f.Describe()
	}
	return s
}

// groupDigits 给整数加窄空格分位：5000 → 5 000。
func groupDigits(v uint64) string {
	s := strconv.FormatUint(v, 10)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += " "
		}
		out += string(r)
	}
	return out
}

// valueRange 画当前页数值的分布直方图与值域端点。
func valueRange(c *ui.Context, app *App, n *hdf5.Node) {
	if n.Kind != hdf5.KindDataset {
		return
	}
	lo, hi, bins, ok := pageHistogram(app, 16)
	if !ok {
		return
	}
	ui.Column(c).Shrink(0).Gap(8).Children(func() {
		inspectorTitle(c, "值域")
		box := ui.Box(c).Shrink(0).Height(56)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			max := 0.0
			for _, v := range bins {
				if v > max {
					max = v
				}
			}
			if max <= 0 {
				return
			}
			gap := float32(2)
			w := (r.W - gap*float32(len(bins)-1)) / float32(len(bins))
			for i, v := range bins {
				h := float32(v/max) * r.H
				if h < 1 {
					h = 1
				}
				col := ui.Hex("#232A2F")
				if v/max >= 0.6 {
					col = ColAccent
				}
				p.Fill(ui.Rect{
					X: r.X + float32(i)*(w+gap),
					Y: r.Y + r.H - h,
					W: w,
					H: h,
				}, col, 1)
			}
		})
		ui.Row(c).Shrink(0).Children(func() {
			ui.Text(c, formatFloatValue(lo)).Font(FontMono).FontSize(10.5).
				TextColor(ColMute).SingleLine()
			ui.Spacer(c).Grow(1)
			ui.Text(c, formatFloatValue(hi)).Font(FontMono).FontSize(10.5).
				TextColor(ColMute).SingleLine()
		})
	})
}

// pageHistogram 把当前页的数值切成 n 段并统计频数。
//
// 只读已经载入内存的这一页，不再触发任何读取。
func pageHistogram(app *App, n int) (lo, hi float64, bins []float64, ok bool) {
	data, isSlice := app.tableData.(*hdf5.Slice)
	if !isSlice || data == nil || n <= 0 {
		return 0, 0, nil, false
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range data.Values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if math.IsInf(lo, 1) || math.IsInf(hi, -1) {
		return 0, 0, nil, false
	}
	bins = make([]float64, n)
	if hi == lo {
		bins[0] = 1
		return lo, hi, bins, true
	}
	step := (hi - lo) / float64(n)
	for _, v := range data.Values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		i := int((v - lo) / step)
		if i < 0 {
			i = 0
		}
		if i >= n {
			i = n - 1
		}
		bins[i]++
	}
	return lo, hi, bins, true
}

// inspectorAttrs 是检查器里的属性摘要，最多列 8 条。
func inspectorAttrs(c *ui.Context, n *hdf5.Node) {
	attrs, err := n.Attributes()
	if err != nil {
		return
	}
	ui.Column(c).Shrink(0).Gap(8).Children(func() {
		inspectorTitle(c, "属性")
		if len(attrs) == 0 {
			ui.Text(c, "无属性").FontSize(11.5).TextColor(ColMute).SingleLine()
			return
		}
		shown := len(attrs)
		if shown > 8 {
			shown = 8
		}
		for _, a := range attrs[:shown] {
			value := a.Value
			if a.Error != "" {
				value = "读取失败"
			} else if v, truncated := truncateValue(value); truncated {
				value = v
			}
			kv(c, a.Name, value)
		}
		if len(attrs) > shown {
			ui.Text(c, "另有 "+strconv.Itoa(len(attrs)-shown)+" 条").
				FontSize(10.5).TextColor(ColFaint).SingleLine()
		}
	})
}

// source 是检查器里的来源信息：路径与对象地址。
func source(c *ui.Context, n *hdf5.Node) {
	ui.Column(c).Shrink(0).Gap(8).Children(func() {
		inspectorTitle(c, "来源")
		ui.Text(c, n.Path).Font(FontMono).FontSize(11.5).TextColor(ColCell).
			SingleLine()
		ui.Text(c, addressText(n.ID.Address)).Font(FontMono).FontSize(10.5).
			TextColor(ColMute).SingleLine()
	})
}
