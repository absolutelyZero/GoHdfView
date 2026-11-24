package hdf5

import (
	"strconv"
	"strings"

	libhdf5 "github.com/scigolib/hdf5"
)

// ClassText 返回数据类型类别的中文名称。
func (d DatatypeInfo) ClassText() string {
	switch d.Class {
	case libhdf5.ClassInteger:
		return "整数"
	case libhdf5.ClassFloat:
		return "浮点"
	case libhdf5.ClassTime:
		return "时间"
	case libhdf5.ClassString:
		return "字符串"
	case libhdf5.ClassBitfield:
		return "位域"
	case libhdf5.ClassOpaque:
		return "不透明"
	case libhdf5.ClassCompound:
		return "复合"
	case libhdf5.ClassReference:
		return "引用"
	case libhdf5.ClassEnum:
		return "枚举"
	case libhdf5.ClassVLen:
		return "变长"
	case libhdf5.ClassArray:
		return "数组"
	case libhdf5.ClassComplex:
		return "复数"
	default:
		return "未知类型"
	}
}

// OrderText 返回字节序的中文名称。
func (d DatatypeInfo) OrderText() string {
	switch d.Order {
	case libhdf5.OrderLittleEndian:
		return "小端"
	case libhdf5.OrderBigEndian:
		return "大端"
	case libhdf5.OrderVAX:
		return "VAX"
	case libhdf5.OrderMixed:
		return "混合"
	case libhdf5.OrderNone:
		return "无"
	default:
		return "未知"
	}
}

// PaddingText 返回定长字符串填充方式的中文名称。
func (d DatatypeInfo) PaddingText() string {
	switch d.Padding {
	case libhdf5.PaddingNullTerm:
		return "空字符结尾"
	case libhdf5.PaddingNullPad:
		return "空字符填充"
	case libhdf5.PaddingSpacePad:
		return "空格填充"
	default:
		return "未知"
	}
}

// CharsetText 返回字符集名称。
func (d DatatypeInfo) CharsetText() string {
	switch d.Charset {
	case libhdf5.CharsetASCII:
		return "ASCII"
	case libhdf5.CharsetUTF8:
		return "UTF-8"
	default:
		return "未知"
	}
}

// SizeText 返回元素字节大小的文本，例如 "4 字节"。
func (d DatatypeInfo) SizeText() string {
	return formatUint(d.Size) + " 字节"
}

// IsString 报告类型是否为字符串（定长或变长）。
//
// 视图层用它决定是否显示填充方式与字符集，
// 从而不必直接依赖解析库的类别常量。
func (d DatatypeInfo) IsString() bool {
	return d.Class == libhdf5.ClassString || d.Class == libhdf5.ClassVLen
}

// IsCompound 报告类型是否为复合类型。
func (d DatatypeInfo) IsCompound() bool { return d.Class == libhdf5.ClassCompound }

// IsEnum 报告类型是否为枚举。
func (d DatatypeInfo) IsEnum() bool { return d.Class == libhdf5.ClassEnum }

// IsInteger 报告类型是否为整数（含枚举与位域，它们的底层都是整数）。
func (d DatatypeInfo) IsInteger() bool {
	switch d.Class {
	case libhdf5.ClassInteger, libhdf5.ClassEnum, libhdf5.ClassBitfield:
		return true
	}
	return false
}

// IsFloat 报告类型是否为浮点。
func (d DatatypeInfo) IsFloat() bool { return d.Class == libhdf5.ClassFloat }

// IsNumeric 报告类型能否以数值方式展示（也就能画成图像）。
func (d DatatypeInfo) IsNumeric() bool { return d.IsInteger() || d.IsFloat() }

// IsVariableString 报告类型是否为变长字符串。
func (d DatatypeInfo) IsVariableString() bool {
	return d.VariableString || d.Class == libhdf5.ClassVLen
}

// SignedText 返回符号性文本；非整数类别返回空字符串。
func (d DatatypeInfo) SignedText() string {
	switch d.Class {
	case libhdf5.ClassInteger, libhdf5.ClassEnum:
		if d.Signed {
			return "有符号"
		}
		return "无符号"
	default:
		return ""
	}
}

// Describe 返回一行的类型描述，用于元数据视图的类型摘要。
func (d DatatypeInfo) Describe() string {
	var b strings.Builder
	b.WriteString(d.ClassText())
	b.WriteString(" · ")
	b.WriteString(d.SizeText())
	if s := d.SignedText(); s != "" {
		b.WriteString(" · ")
		b.WriteString(s)
	}
	if d.Class == libhdf5.ClassString || d.Class == libhdf5.ClassVLen {
		if d.VariableString || d.Class == libhdf5.ClassVLen {
			b.WriteString(" · 变长")
		} else {
			b.WriteString(" · 定长")
		}
	}
	return b.String()
}

// LayoutText 返回存储布局类别的中文名称。
func (l LayoutInfo) ClassText() string {
	switch l.Class {
	case libhdf5.LayoutCompact:
		return "紧凑"
	case libhdf5.LayoutContiguous:
		return "连续"
	case libhdf5.LayoutChunked:
		return "分块"
	case libhdf5.LayoutVirtual:
		return "虚拟"
	default:
		return "未知布局"
	}
}

// Describe 返回过滤器的可读描述。
//
// 已知过滤器带上影响解码结果的参数（deflate 的压缩级别）；
// 未知 ID 保留原始编号与客户端参数，并显式标注「未知过滤器」，
// 而不是静默丢弃（dataset-metadata 的第三个 scenario）。
func (f FilterInfo) Describe() string {
	if !f.Known {
		var b strings.Builder
		b.WriteString("filter(")
		b.WriteString(strconv.FormatUint(uint64(f.ID), 10))
		b.WriteString(")（未知过滤器")
		if len(f.ClientData) > 0 {
			b.WriteString("，参数 ")
			for i, v := range f.ClientData {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(strconv.FormatUint(uint64(v), 10))
			}
		}
		b.WriteString("）")
		return b.String()
	}
	name := f.Name
	if name == "" {
		name = f.ID.String()
	}
	switch f.ID {
	case libhdf5.FilterDeflate:
		if len(f.ClientData) > 0 {
			return "deflate (level " + strconv.FormatUint(uint64(f.ClientData[0]), 10) + ")"
		}
		return "deflate"
	case libhdf5.FilterShuffle:
		return "shuffle"
	case libhdf5.FilterFletcher32:
		return "fletcher32"
	case libhdf5.FilterSZIP:
		return "szip"
	case libhdf5.FilterNBit:
		return "nbit"
	case libhdf5.FilterScaleOffset:
		return "scaleoffset"
	default:
		return name
	}
}

// FormatDims 把维度格式化为 "[100, 50]"；空维度返回空字符串（标量）。
func FormatDims(dims []uint64) string {
	if len(dims) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, d := range dims {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(formatUint(d))
	}
	b.WriteByte(']')
	return b.String()
}

// FormatMaxDims 把最大维度格式化为 "[unlimited, 50]"。
// 无限维显示为 "unlimited"，其余按数值显示。
func FormatMaxDims(dims []uint64, unlimited []bool) string {
	if len(dims) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, d := range dims {
		if i > 0 {
			b.WriteString(", ")
		}
		if i < len(unlimited) && unlimited[i] {
			b.WriteString("unlimited")
			continue
		}
		b.WriteString(formatUint(d))
	}
	b.WriteByte(']')
	return b.String()
}
