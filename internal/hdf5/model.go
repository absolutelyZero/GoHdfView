package hdf5

import (
	"strings"

	libhdf5 "github.com/scigolib/hdf5"
)

// NodeKind 区分对象树中节点的种类。
type NodeKind uint8

// 节点种类。软链接与外部链接保留在模型中，
// 以便对象树把它们显示为不可展开的叶子，而不是静默丢弃。
const (
	KindGroup        NodeKind = iota + 1 // 组
	KindDataset                          // 数据集
	KindNamedType                        // 命名数据类型
	KindSoftLink                         // 软链接
	KindExternalLink                     // 外部链接
)

// String 返回节点种类的中文名称，用于对象树标签与元数据视图。
func (k NodeKind) String() string {
	switch k {
	case KindGroup:
		return "组"
	case KindDataset:
		return "数据集"
	case KindNamedType:
		return "命名类型"
	case KindSoftLink:
		return "软链接"
	case KindExternalLink:
		return "外部链接"
	default:
		return "未知"
	}
}

// NodeID 是对象树中节点的稳定身份。
//
// 地址取自 HDF5 对象头地址：同一文件内唯一，且在重新读取时保持不变；
// 路径作为兜底，用于解析库不提供地址的对象（组、命名类型）。
// 不使用完整路径作为唯一身份，因为同一对象可被多个硬链接指向。
type NodeID struct {
	Address uint64
	Kind    NodeKind
	Path    string
}

// Node 是对象树中的一个节点。
//
// 它是值类型视图：Children 在调用时才向解析库取子节点，
// 因此构造整棵树不会触发全量 I/O。
type Node struct {
	ID       NodeID
	Name     string // 对象名（不含路径）
	Path     string // 从根开始的完整路径，根为 "/"
	Kind     NodeKind
	LinkInfo string // 软链接/外部链接的目标描述；其它种类为空

	// ShapeText 是维度概要的缓存，例如 "[100×50]"。
	// 它在节点构造时算好：树每帧都会为可见行取这个字符串，
	// 而 Shape 每次调用都要重新解码数据集描述符。
	ShapeText string

	group *libhdf5.Group
	ds    *libhdf5.Dataset
	typ   *libhdf5.NamedDatatype
	doc   *Document
}

// IsContainer 报告该节点是否可以有子节点。
// 软链接与外部链接一律返回 false：本工具不跟随链接，避免跨文件读取与环。
func (n *Node) IsContainer() bool {
	return n.Kind == KindGroup
}

// DimensionSummary 返回对象树标签用的维度概要，例如 "[100×50]"。
// 非数据集、或读取维度失败时返回空字符串。
//
// 结果在节点构造时缓存到 ShapeText，避免每帧重复解码描述符。
func (n *Node) DimensionSummary() string {
	if n.ShapeText != "" {
		return n.ShapeText
	}
	if n.Kind != KindDataset || n.ds == nil {
		return ""
	}
	shape, err := n.ds.Shape()
	if err != nil {
		return ""
	}
	n.ShapeText = FormatShape(shape)
	return n.ShapeText
}

// FormatShape 把维度切片格式化为 "[100×50]"，标量（rank 0）返回 "[标量]"。
func FormatShape(shape []uint64) string {
	if len(shape) == 0 {
		return "[标量]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, d := range shape {
		if i > 0 {
			b.WriteString("×")
		}
		b.WriteString(formatUint(d))
	}
	b.WriteByte(']')
	return b.String()
}

// DatasetInfo 是数据集元数据视图的数据来源。
//
// MaxShape 与 Unlimited 一一对应：Unlimited[i] 为 true 时该维可无限增长，
// 此时 MaxShape[i] 无意义（解析库用全 1 表示，本层置 0）。
type DatasetInfo struct {
	Rank      int
	Shape     []uint64
	MaxShape  []uint64
	Unlimited []bool
	ElemCount uint64 // 元素总数；维度乘积溢出时置 MaxUint64
	Overflow  bool   // 元素总数是否溢出 uint64

	Datatype DatatypeInfo
	Layout   LayoutInfo
}

// DatatypeInfo 是数据类型视图的数据来源。
type DatatypeInfo struct {
	Class          libhdf5.DatatypeClass
	Size           uint64
	Order          libhdf5.ByteOrder
	Signed         bool
	Padding        libhdf5.StringPadding
	Charset        libhdf5.Charset
	VariableString bool
	Members        []CompoundMember
	EnumMembers    []EnumMember
	ArrayDims      []uint64
	Tag            string
	Base           *DatatypeInfo
}

// CompoundMember 是复合类型的一个成员。
type CompoundMember struct {
	Name   string
	Offset uint64
	Type   DatatypeInfo
}

// EnumMember 是枚举类型的一个成员。
// Value 按解析库约定存放：64 位无符号基类型的值以二进制补码形式存放。
type EnumMember struct {
	Name  string
	Value int64
}

// LayoutInfo 是数据布局视图的数据来源。
type LayoutInfo struct {
	Class     libhdf5.LayoutClass
	ChunkDims []uint64
	Filters   []FilterInfo
}

// FilterInfo 是过滤器管道中的一项。
type FilterInfo struct {
	ID         libhdf5.FilterID
	Name       string
	Optional   bool
	ClientData []uint32
	Known      bool // 解析库是否认识该过滤器 ID
}

// AttributeInfo 是属性视图的一项。
type AttributeInfo struct {
	Name     string
	TypeName string // 人类可读的类型描述
	Value    string // 已格式化的值（数组以逗号分隔，超长由视图层截断）
	Raw      any    // 原始值，供「查看完整值」使用
	IsArray  bool   // 值是否为数组
	ElemNum  int    // 数组元素个数
	Error    string // 读取该属性失败时的说明
}

// Slice 是一次读取的结果。
//
// Values 始终是一维、按 C 序展平的 float64 序列：解析库的切片接口
// 内部统一转换为 float64，因此定长整数与浮点都走这条路径。
// 超出 2^53 的 8 字节整数会损失精度，视图层据此加近似标记。
type Slice struct {
	Start       []uint64
	Count       []uint64
	Shape       []uint64  // 数据集整体维度，便于视图层换算行列
	Values      []float64 // 展平后的值
	Truncated   bool      // 是否因读取上限而被截断
	TruncatedAt uint64    // 截断后实际读取的元素数
}

// Strings 是字符串数据集一次读取的结果。
type Strings struct {
	Values []string
}

// Compound 是复合数据集一次读取的结果：每个元素是一组成员值。
type Compound struct {
	Members []string
	Rows    []map[string]any
}

// EnumValue 是枚举值的可展示形式。
type EnumValue struct {
	Name  string
	Value int64
	Found bool // 是否命中枚举成员表
}

func formatUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
