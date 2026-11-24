package hdf5

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"

	libhdf5 "github.com/scigolib/hdf5"
)

// DefaultReadLimit 是单次读取的默认上限：64 MiB 的 float64 值，
// 约等于 800 万个元素。超过此量的请求会被拒绝并要求缩小选区。
const DefaultReadLimit uint64 = 64 << 20

// hdf5Signature 是 HDF5 超级块签名。
var hdf5Signature = []byte{0x89, 'H', 'D', 'F', '\r', '\n', 0x1a, '\n'}

// hdf4Signature 是 HDF4 文件签名（0x0E 0x03 0x13 0x01）。
var hdf4Signature = []byte{0x0e, 0x03, 0x13, 0x01}

// Document 是一个已打开文档的只读句柄。
//
// 同一时刻只持有一个底层文件句柄；Close 之后所有读取返回 ErrClosed。
// 读取方法内部串行化：解析库不保证 *File 的并发安全，
// 上层可以放心从工作 goroutine 调用。
type Document struct {
	path  string
	size  int64
	limit uint64

	mu     sync.Mutex
	file   *libhdf5.File
	root   *Node
	closed bool
}

// Open 以只读方式打开 path。
//
// 失败时返回的错误一定包裹 ErrNotHDF5、ErrCorrupt、ErrUnsupported
// 或 ErrTooLarge 之一，调用方用 errors.Is 判断类别。
func Open(p string) (*Document, error) {
	info, err := os.Stat(p)
	if err != nil {
		// 保留操作系统错误的文本，但不归入任何解析类哨兵：
		// 「文件不存在/无权读取」与「内容无法解析」是两类问题。
		return nil, fmt.Errorf("打开文件失败：%w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("打开文件失败：%s 是目录", p)
	}
	if err := checkSignature(p, info.Size()); err != nil {
		return nil, err
	}

	f, err := libhdf5.Open(p)
	if err != nil {
		return nil, classify(err)
	}

	doc := &Document{path: p, size: info.Size(), limit: DefaultReadLimit, file: f}
	doc.root = doc.wrapGroup(f.Root(), "", "/")
	return doc, nil
}

// checkSignature 在交给解析库之前自行识别签名，
// 以便把「不是 HDF5」「HDF4」与「损坏」区分开：
// 解析库对签名缺失只返回一句无哨兵的纯文本错误。
func checkSignature(p string, size int64) error {
	f, err := os.Open(p)
	if err != nil {
		return fmt.Errorf("打开文件失败：%w", err)
	}
	defer f.Close()

	if size == 0 {
		return fmt.Errorf("%w：%s 是空文件", ErrNotHDF5, p)
	}

	head := make([]byte, 8)
	if _, err := f.ReadAt(head, 0); err == nil {
		if len(hdf4Signature) <= len(head) && string(head[:len(hdf4Signature)]) == string(hdf4Signature) {
			return fmt.Errorf("%w：检测到 HDF4 签名，仅支持 HDF5", ErrNotHDF5)
		}
	}

	// HDF5 允许在文件前放 user block，签名位于 512、1024、2048… 的 2 的幂偏移处。
	for off := int64(0); off < size && off <= 1<<20; off = nextUserBlock(off) {
		buf := make([]byte, len(hdf5Signature))
		if _, err := f.ReadAt(buf, off); err != nil {
			break
		}
		if string(buf) == string(hdf5Signature) {
			return nil
		}
	}
	return fmt.Errorf("%w：%s 中找不到 HDF5 签名", ErrNotHDF5, p)
}

// nextUserBlock 返回下一个可能的 user block 偏移：0 → 512 → 1024 → 2048 …
func nextUserBlock(off int64) int64 {
	if off == 0 {
		return 512
	}
	return off * 2
}

// classify 把解析库的错误归入本包的哨兵。
func classify(err error) error {
	switch {
	case errors.Is(err, libhdf5.ErrNotSupported):
		return fmt.Errorf("%w：%v", ErrUnsupported, err)
	case errors.Is(err, libhdf5.ErrCorrupt):
		return fmt.Errorf("%w：%v", ErrCorrupt, err)
	case errors.Is(err, libhdf5.ErrLimit):
		return fmt.Errorf("%w：%v", ErrTooLarge, err)
	case errors.Is(err, libhdf5.ErrNeedsRepack):
		return fmt.Errorf("%w：%v", ErrUnsupported, err)
	case errors.Is(err, libhdf5.ErrNoData):
		return fmt.Errorf("%w：%v", ErrUnsupported, err)
	}
	// 解析库把「签名不匹配」写成纯文本，且把硬链接环写成 "object graph cycle"。
	// 两者都没有哨兵，只能按文本识别；放在最后以免遮蔽上面的哨兵判断。
	msg := err.Error()
	if strings.Contains(msg, "not an HDF5 file") {
		return fmt.Errorf("%w：%v", ErrNotHDF5, err)
	}
	if strings.Contains(msg, "object graph cycle") {
		return fmt.Errorf("%w：组之间存在硬链接环，无法展开对象树", ErrUnsupported)
	}
	return fmt.Errorf("%w：%v", ErrCorrupt, err)
}

// Path 返回文档的完整路径。
func (d *Document) Path() string { return d.path }

// Size 返回文件字节数。
func (d *Document) Size() int64 { return d.size }

// SetReadLimit 设置单次读取上限（字节）。传入 0 表示恢复默认值。
func (d *Document) SetReadLimit(bytes uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if bytes == 0 {
		bytes = DefaultReadLimit
	}
	d.limit = bytes
	if d.file != nil {
		d.file.SetReadLimit(bytes)
	}
}

// ReadLimit 返回当前单次读取上限。
func (d *Document) ReadLimit() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.limit
}

// Root 返回根组节点；文档已关闭时返回 nil。
func (d *Document) Root() *Node {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	return d.root
}

// Close 释放底层句柄。可重复调用。
func (d *Document) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.root = nil
	if d.file != nil {
		err := d.file.Close()
		d.file = nil
		return err
	}
	return nil
}

// wrapGroup 把解析库的组对象转换成本层节点。
func (d *Document) wrapGroup(g *libhdf5.Group, name, p string) *Node {
	if name == "" {
		name = "/"
	}
	return &Node{ID: NodeID{Address: 0, Kind: KindGroup, Path: p}, Name: name, Path: p, Kind: KindGroup, group: g, doc: d}
}

// Children 返回节点的直接子节点。
//
// 解析库在打开文件时已把链接表读入内存，因此本方法不触发额外 I/O；
// 但它只返回一层，不会递归，符合「树按需构建」的要求。
// 软链接与外部链接被解析库在加载阶段跳过，不会出现在结果中（见 docs/verification.md）。
func (n *Node) Children() ([]*Node, error) {
	if n.doc == nil {
		return nil, ErrClosed
	}
	if n.Kind != KindGroup || n.group == nil {
		return nil, nil
	}
	n.doc.mu.Lock()
	defer n.doc.mu.Unlock()
	if n.doc.closed {
		return nil, ErrClosed
	}

	objs := n.group.Children()
	out := make([]*Node, 0, len(objs))
	for _, obj := range objs {
		switch o := obj.(type) {
		case *libhdf5.Group:
			childPath := joinPath(n.Path, o.Name())
			out = append(out, &Node{
				ID:    NodeID{Kind: KindGroup, Path: childPath},
				Name:  o.Name(),
				Path:  childPath,
				Kind:  KindGroup,
				group: o,
				doc:   n.doc,
			})
		case *libhdf5.Dataset:
			childPath := joinPath(n.Path, o.Name())
			out = append(out, &Node{
				ID:   NodeID{Address: o.Address(), Kind: KindDataset, Path: childPath},
				Name: o.Name(),
				Path: childPath,
				Kind: KindDataset,
				ds:   o,
				doc:  n.doc,
			})
		case *libhdf5.NamedDatatype:
			childPath := joinPath(n.Path, o.Name())
			out = append(out, &Node{
				ID:   NodeID{Kind: KindNamedType, Path: childPath},
				Name: o.Name(),
				Path: childPath,
				Kind: KindNamedType,
				typ:  o,
				doc:  n.doc,
			})
		}
	}
	return out, nil
}

// joinPath 拼接父子路径；父为根（"/"）时不产生双斜杠。
func joinPath(parent, name string) string {
	if parent == "/" || parent == "" {
		return "/" + name
	}
	return parent + "/" + name
}

// NodeByPath 按完整路径查找节点，用于面包屑导航。
func (d *Document) NodeByPath(p string) (*Node, error) {
	d.mu.Lock()
	root := d.root
	closed := d.closed
	d.mu.Unlock()
	if closed || root == nil {
		return nil, ErrClosed
	}
	if p == "" || p == "/" {
		return root, nil
	}

	cur := root
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			continue
		}
		kids, err := cur.Children()
		if err != nil {
			return nil, err
		}
		var next *Node
		for _, k := range kids {
			if k.Name == seg {
				next = k
				break
			}
		}
		if next == nil {
			return nil, fmt.Errorf("%w：路径 %s 不存在", ErrNotFound, p)
		}
		cur = next
	}
	return cur, nil
}

// Info 返回数据集的元数据。对非数据集节点返回 ErrNotDataset。
func (n *Node) Info() (*DatasetInfo, error) {
	if n.Kind != KindDataset || n.ds == nil {
		return nil, ErrNotDataset
	}
	n.doc.mu.Lock()
	defer n.doc.mu.Unlock()
	if n.doc.closed {
		return nil, ErrClosed
	}

	shape, err := n.ds.Shape()
	if err != nil {
		return nil, classify(err)
	}
	maxShape, err := n.ds.MaxShape()
	if err != nil {
		return nil, classify(err)
	}
	dt, err := n.ds.Datatype()
	if err != nil {
		return nil, classify(err)
	}
	layout, err := n.ds.Layout()
	if err != nil {
		return nil, classify(err)
	}

	info := &DatasetInfo{
		Rank:     len(shape),
		Shape:    shape,
		Datatype: convertDatatype(dt),
		Layout:   convertLayout(layout),
	}
	// 无限维在解析库中用 libhdf5.Unlimited 表示；本层用 Unlimited 标记，
	// 这样视图层可以直接显示 "unlimited"。
	info.MaxShape = make([]uint64, len(maxShape))
	info.Unlimited = make([]bool, len(maxShape))
	for i, m := range maxShape {
		if m == libhdf5.Unlimited {
			info.Unlimited[i] = true
			continue
		}
		info.MaxShape[i] = m
	}
	if len(maxShape) == 0 {
		info.MaxShape = nil
		info.Unlimited = nil
	}

	count, overflow := elemCount(shape)
	info.ElemCount = count
	info.Overflow = overflow
	return info, nil
}

// MaxUnlimited 报告第 i 维的最大值是否为无限。
func (di *DatasetInfo) MaxUnlimited(i int) bool {
	if di == nil || i < 0 || i >= len(di.Unlimited) {
		return false
	}
	return di.Unlimited[i]
}

// EnumName 把枚举数据集的取值映射为成员名。
func (di *DatasetInfo) EnumName(v float64) (string, bool) {
	if di == nil || di.Datatype.Class != libhdf5.ClassEnum {
		return "", false
	}
	for _, m := range di.Datatype.EnumMembers {
		if di.Datatype.Signed {
			if int64(v) == m.Value {
				return m.Name, true
			}
			continue
		}
		if uint64(v) == uint64(m.Value) {
			return m.Name, true
		}
	}
	return "", false
}

// convertDatatype 把解析库的数据类型描述转成本层结构。
func convertDatatype(dt libhdf5.DatatypeInfo) DatatypeInfo {
	out := DatatypeInfo{
		Class:          dt.Class,
		Size:           dt.Size,
		Order:          dt.Order,
		Signed:         dt.Signed,
		Padding:        dt.Padding,
		Charset:        dt.Charset,
		VariableString: dt.VariableString,
		ArrayDims:      append([]uint64(nil), dt.ArrayDims...),
		Tag:            dt.Tag,
	}
	if len(dt.Members) > 0 {
		out.Members = make([]CompoundMember, len(dt.Members))
		for i, m := range dt.Members {
			out.Members[i] = CompoundMember{Name: m.Name, Offset: m.Offset, Type: convertDatatype(m.Type)}
		}
	}
	if len(dt.EnumMembers) > 0 {
		out.EnumMembers = make([]EnumMember, len(dt.EnumMembers))
		for i, m := range dt.EnumMembers {
			out.EnumMembers[i] = EnumMember{Name: m.Name, Value: m.Value}
		}
	}
	if dt.Base != nil {
		base := convertDatatype(*dt.Base)
		out.Base = &base
	}
	return out
}

// convertLayout 把解析库的布局描述转成本层结构。
func convertLayout(l libhdf5.LayoutInfo) LayoutInfo {
	out := LayoutInfo{Class: l.Class, ChunkDims: append([]uint64(nil), l.ChunkDims...)}
	if len(l.Filters) > 0 {
		out.Filters = make([]FilterInfo, len(l.Filters))
		for i, f := range l.Filters {
			out.Filters[i] = FilterInfo{
				ID:         f.ID,
				Name:       f.Name,
				Optional:   f.Optional,
				ClientData: append([]uint32(nil), f.ClientData...),
				Known:      f.ID >= 1 && f.ID <= 6,
			}
		}
	}
	return out
}

// elemCount 计算维度乘积，溢出时返回 MaxUint64 并置 overflow。
func elemCount(shape []uint64) (uint64, bool) {
	if len(shape) == 0 {
		return 1, false // 标量恰好一个元素
	}
	acc := uint64(1)
	for _, d := range shape {
		hi, lo := bits.Mul64(acc, d)
		if hi != 0 {
			return math.MaxUint64, true
		}
		acc = lo
	}
	return acc, false
}

// GroupInfo 是组的元数据视图的数据来源。
type GroupInfo struct {
	MemberCount    int
	AttributeCount int
}

// GroupInfo 返回组的成员数与属性数。
func (n *Node) GroupInfo() (*GroupInfo, error) {
	if n.Kind != KindGroup || n.group == nil {
		return nil, ErrNotGroup
	}
	kids, err := n.Children()
	if err != nil {
		return nil, err
	}
	info := &GroupInfo{MemberCount: len(kids)}

	n.doc.mu.Lock()
	defer n.doc.mu.Unlock()
	if n.doc.closed {
		return nil, ErrClosed
	}
	attrs, err := n.group.Attributes()
	if err != nil {
		// 传统 SNOD 组没有属性；属性读取失败不应让整个元数据视图不可用。
		info.AttributeCount = 0
		return info, nil
	}
	info.AttributeCount = len(attrs)
	return info, nil
}

// Attributes 返回节点上附加的属性。
//
// 单个属性读取失败不会中断整个列表：该条目的 Error 字段记录原因，
// 其余属性照常显示（对应「单对象降级不影响整份文件」的要求）。
func (n *Node) Attributes() ([]AttributeInfo, error) {
	n.doc.mu.Lock()
	defer n.doc.mu.Unlock()
	if n.doc.closed {
		return nil, ErrClosed
	}

	var out []AttributeInfo
	switch n.Kind {
	case KindDataset:
		if n.ds == nil {
			return nil, nil
		}
		attrs, err := n.ds.Attributes()
		if err != nil {
			return nil, classify(err)
		}
		out = make([]AttributeInfo, 0, len(attrs))
		for _, a := range attrs {
			out = append(out, convertAttr(a.Name, uint8(a.Datatype.Class), a.Datatype.Size, a.Datatype.IsVariableString(), a.ReadValue))
		}
	case KindGroup:
		if n.group == nil {
			return nil, nil
		}
		attrs, err := n.group.Attributes()
		if err != nil {
			return nil, classify(err)
		}
		out = make([]AttributeInfo, 0, len(attrs))
		for _, a := range attrs {
			out = append(out, convertAttr(a.Name, uint8(a.Datatype.Class), a.Datatype.Size, a.Datatype.IsVariableString(), a.ReadValue))
		}
	}
	return out, nil
}

// convertAttr 组装一条属性视图记录。
//
// 参数逐个取自解析库的 Attribute，避免在本包暴露其内部类型；
// read 是 Attribute.ReadValue 方法值。
func convertAttr(name string, class uint8, size uint32, vlen bool, read func() (any, error)) AttributeInfo {
	ai := AttributeInfo{Name: name, TypeName: attrTypeName(class, size, vlen)}
	v, err := read()
	if err != nil {
		ai.Error = classify(err).Error()
		return ai
	}
	ai.Raw = v
	ai.Value, ai.IsArray, ai.ElemNum = formatAttrValue(v)
	return ai
}

// attrTypeName 生成属性的类型描述文本。
func attrTypeName(class uint8, size uint32, vlen bool) string {
	var name string
	switch class {
	case 0:
		name = "整数"
	case 1:
		name = "浮点"
	case 2:
		name = "时间"
	case 3:
		name = "字符串"
	case 4:
		name = "位域"
	case 5:
		name = "不透明"
	case 6:
		name = "复合"
	case 7:
		name = "引用"
	case 8:
		name = "枚举"
	case 9:
		name = "变长"
	case 10:
		name = "数组"
	case 11:
		name = "复数"
	default:
		name = fmt.Sprintf("类型%d", class)
	}
	if vlen {
		return name + "（变长）"
	}
	if class == 3 {
		return fmt.Sprintf("%s（%d 字节）", name, size)
	}
	return fmt.Sprintf("%s（%d 字节）", name, size)
}

// formatAttrValue 把属性值转成单行文本。
func formatAttrValue(v any) (string, bool, int) {
	if v == nil {
		return "", false, 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() != reflect.Uint8 {
		n := rv.Len()
		parts := make([]string, n)
		for i := 0; i < n; i++ {
			parts[i] = formatScalar(rv.Index(i).Interface())
		}
		return strings.Join(parts, ", "), true, n
	}
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		return string(rv.Bytes()), false, 0
	}
	return formatScalar(v), false, 0
}

// formatScalar 格式化单个标量值。
func formatScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return formatFloat(x)
	case float32:
		return formatFloat(float64(x))
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatFloat 用最短往返表示格式化浮点；非有限值用 HDFView 风格写法。
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "+Inf"
	case math.IsInf(f, -1):
		return "-Inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// ReadSlice 读取一个矩形切片。
//
// 返回值始终是展平的 float64 序列（解析库内部统一转换）。
// count 的元素数超过当前读取上限时返回 ErrTooLarge，不分配内存；
// 越界返回 ErrOutOfBounds。
func (d *Document) ReadSlice(node *Node, start, count []uint64) (*Slice, error) {
	if node == nil || node.Kind != KindDataset || node.ds == nil {
		return nil, ErrNotDataset
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}

	shape, err := node.ds.Shape()
	if err != nil {
		return nil, classify(err)
	}
	if len(start) != len(shape) || len(count) != len(shape) {
		return nil, fmt.Errorf("%w：请求 %d 维，数据集 %d 维", ErrOutOfBounds, len(start), len(shape))
	}
	for i := range shape {
		if count[i] > shape[i] || start[i] > shape[i]-count[i] {
			return nil, fmt.Errorf("%w：第 %d 维 start=%d count=%d 超出 %d",
				ErrOutOfBounds, i, start[i], count[i], shape[i])
		}
	}

	want, overflow := elemCount(count)
	if overflow || want > d.limit/8 {
		return nil, fmt.Errorf("%w：请求 %d 个元素（%d 字节），上限 %d 字节",
			ErrTooLarge, want, want*8, d.limit)
	}

	raw, err := node.ds.ReadSlice(start, count)
	if err != nil {
		return nil, classify(err)
	}

	vals, ok := toFloat64Slice(raw)
	if !ok {
		return nil, fmt.Errorf("%w：解析库返回了非数值结果 %T", ErrUnsupported, raw)
	}
	return &Slice{
		Start:  append([]uint64(nil), start...),
		Count:  append([]uint64(nil), count...),
		Shape:  shape,
		Values: vals,
	}, nil
}

// toFloat64Slice 把解析库的切片结果统一成 []float64。
func toFloat64Slice(raw any) ([]float64, bool) {
	switch v := raw.(type) {
	case []float64:
		return v, true
	case []float32:
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x)
		}
		return out, true
	case []int32:
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x)
		}
		return out, true
	case []int64:
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x)
		}
		return out, true
	case nil:
		return nil, true
	}
	return nil, false
}

// ReadStrings 读取字符串数据集的全部值。
// 变长字符串走 ReadVLenBytes 路径（定长路径不接受变长类型）。
func (d *Document) ReadStrings(node *Node) (*Strings, error) {
	if node == nil || node.Kind != KindDataset || node.ds == nil {
		return nil, ErrNotDataset
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}

	dt, err := node.ds.Datatype()
	if err != nil {
		return nil, classify(err)
	}
	if dt.VariableString || dt.Class == libhdf5.ClassVLen {
		raw, err := node.ds.ReadVLenBytes()
		if err != nil {
			return nil, classify(err)
		}
		out := make([]string, len(raw))
		for i, b := range raw {
			out[i] = string(b)
		}
		return &Strings{Values: out}, nil
	}

	vals, err := node.ds.ReadStrings()
	if err != nil {
		return nil, classify(err)
	}
	return &Strings{Values: vals}, nil
}

// ReadCompound 读取复合数据集。
//
// 解析库返回的每个元素是 map[string]any；成员顺序取自数据类型描述。
func (d *Document) ReadCompound(node *Node) (*Compound, error) {
	if node == nil || node.Kind != KindDataset || node.ds == nil {
		return nil, ErrNotDataset
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}

	dt, err := node.ds.Datatype()
	if err != nil {
		return nil, classify(err)
	}
	members := make([]string, 0, len(dt.Members))
	for _, m := range dt.Members {
		members = append(members, m.Name)
	}

	raw, err := node.ds.ReadCompound()
	if err != nil {
		return nil, classify(err)
	}
	rows := make([]map[string]any, len(raw))
	for i, cv := range raw {
		row := make(map[string]any, len(cv))
		for k, v := range cv {
			row[k] = v
		}
		rows[i] = row
	}
	return &Compound{Members: members, Rows: rows}, nil
}

// HasParent 报告 p 是否位于该节点之下，用于面包屑判断。
func (n *Node) HasParent(p string) bool {
	return strings.HasPrefix(n.Path, strings.TrimSuffix(p, "/")+"/")
}

// BaseName 返回路径的最后一段。
func BaseName(p string) string { return path.Base(p) }
