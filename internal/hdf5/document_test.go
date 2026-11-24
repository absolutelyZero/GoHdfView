package hdf5

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	libhdf5 "github.com/scigolib/hdf5"
)

// fmtWrap 把解析库的哨兵包一层，模拟真实调用链上的错误形状。
func fmtWrap(err error) error { return fmt.Errorf("读取失败: %w", err) }

// errText 构造只有文本、没有哨兵的错误，用于测试 classify 的兜底分支。
type textErr string

func (e textErr) Error() string { return string(e) }

func errText(s string) error { return textErr(s) }

// fixture 返回 testdata 下的夹具路径。
func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("夹具 %s 不存在，请先运行 go run internal/hdf5/testdata/gen.go: %v", name, err)
	}
	return p
}

// openFixture 打开夹具并在测试结束时关闭。
func openFixture(t *testing.T, name string) *Document {
	t.Helper()
	doc, err := Open(fixture(t, name))
	if err != nil {
		t.Fatalf("打开 %s 失败: %v", name, err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	return doc
}

// findChild 按名字在父节点的子节点中查找。
func findChild(t *testing.T, parent *Node, name string) *Node {
	t.Helper()
	kids, err := parent.Children()
	if err != nil {
		t.Fatalf("读取 %s 的子节点失败: %v", parent.Path, err)
	}
	for _, k := range kids {
		if k.Name == name {
			return k
		}
	}
	t.Fatalf("%s 下找不到子节点 %q，实际有 %v", parent.Path, name, names(kids))
	return nil
}

func names(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Name
	}
	return out
}

func TestOpenNotHDF5(t *testing.T) {
	_, err := Open(fixture(t, "not_hdf5.txt"))
	if !errors.Is(err, ErrNotHDF5) {
		t.Fatalf("纯文本应归类为 ErrNotHDF5，实际: %v", err)
	}
}

func TestOpenMissingFile(t *testing.T) {
	_, err := Open(filepath.Join("testdata", "不存在.h5"))
	if err == nil {
		t.Fatal("打开不存在的文件应返回错误")
	}
	// 文件不存在是操作系统层面的问题，不归入解析类哨兵。
	for _, s := range []error{ErrNotHDF5, ErrCorrupt, ErrUnsupported, ErrTooLarge} {
		if errors.Is(err, s) {
			t.Fatalf("不存在的文件不应归类为 %v，实际: %v", s, err)
		}
	}
}

func TestOpenDirectory(t *testing.T) {
	if _, err := Open("testdata"); err == nil {
		t.Fatal("打开目录应返回错误")
	}
}

func TestOpenTruncated(t *testing.T) {
	_, err := Open(fixture(t, "truncated.h5"))
	if err == nil {
		t.Fatal("截断文件应返回错误")
	}
	// 截断的 HDF5 仍带签名，因此归入 ErrCorrupt 或 ErrUnsupported，而不是 ErrNotHDF5。
	if !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("截断文件应归类为 ErrCorrupt 或 ErrUnsupported，实际: %v", err)
	}
}

func TestOpenHDF4Signature(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "old.hdf")
	// HDF4 签名 + 足够的填充，确保失败原因来自签名判定而非文件过短。
	data := append([]byte{0x0e, 0x03, 0x13, 0x01}, make([]byte, 512)...)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(p)
	if !errors.Is(err, ErrNotHDF5) {
		t.Fatalf("HDF4 应归类为 ErrNotHDF5，实际: %v", err)
	}
}

func TestOpenEmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.h5")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(p); !errors.Is(err, ErrNotHDF5) {
		t.Fatalf("空文件应归类为 ErrNotHDF5，实际: %v", err)
	}
}

func TestCloseRepeatable(t *testing.T) {
	doc, err := Open(fixture(t, "basic.h5"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != nil {
		t.Fatalf("首次 Close 失败: %v", err)
	}
	if err := doc.Close(); err != nil {
		t.Fatalf("重复 Close 应返回 nil，实际: %v", err)
	}
	if doc.Root() != nil {
		t.Fatal("关闭后 Root 应返回 nil")
	}
	if _, err := doc.ReadSlice(&Node{}, nil, nil); !errors.Is(err, ErrNotDataset) {
		t.Fatalf("关闭后读取应失败，实际: %v", err)
	}
}

func TestOpenReadLimitDefault(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	if got := doc.ReadLimit(); got != DefaultReadLimit {
		t.Fatalf("默认读取上限应为 %d，实际 %d", DefaultReadLimit, got)
	}
	doc.SetReadLimit(1 << 20)
	if got := doc.ReadLimit(); got != 1<<20 {
		t.Fatalf("设置后应为 %d，实际 %d", uint64(1<<20), got)
	}
	doc.SetReadLimit(0)
	if got := doc.ReadLimit(); got != DefaultReadLimit {
		t.Fatalf("传 0 应恢复默认值 %d，实际 %d", DefaultReadLimit, got)
	}
}

func TestPathAndSize(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	if doc.Path() != filepath.Join("testdata", "basic.h5") {
		t.Fatalf("Path 不符: %s", doc.Path())
	}
	info, err := os.Stat(doc.Path())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Size() != info.Size() {
		t.Fatalf("Size 不符: %d != %d", doc.Size(), info.Size())
	}
}

func TestRootAndChildren(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	root := doc.Root()
	if root == nil {
		t.Fatal("Root 不应为 nil")
	}
	if root.Path != "/" {
		t.Fatalf("根路径应为 /，实际 %q", root.Path)
	}
	if root.Kind != KindGroup {
		t.Fatalf("根应为组，实际 %v", root.Kind)
	}
	if !root.IsContainer() {
		t.Fatal("根组应可展开")
	}

	kids, err := root.Children()
	if err != nil {
		t.Fatal(err)
	}
	got := names(kids)
	for _, want := range []string{"matrix", "results"} {
		if !contains(got, want) {
			t.Fatalf("根组应包含 %q，实际 %v", want, got)
		}
	}

	matrix := findChild(t, root, "matrix")
	if matrix.Kind != KindDataset {
		t.Fatalf("matrix 应为数据集，实际 %v", matrix.Kind)
	}
	if matrix.IsContainer() {
		t.Fatal("数据集不应可展开")
	}
	if matrix.Path != "/matrix" {
		t.Fatalf("路径应为 /matrix，实际 %q", matrix.Path)
	}
	if matrix.ID.Address == 0 {
		t.Fatal("数据集的 NodeID 应带对象地址")
	}

	// 数据集的子节点为空，且不报错。
	dsKids, err := matrix.Children()
	if err != nil {
		t.Fatalf("数据集取子节点不应报错: %v", err)
	}
	if len(dsKids) != 0 {
		t.Fatalf("数据集不应有子节点，实际 %v", names(dsKids))
	}

	// 组的地址在解析库中不可得，靠路径区分。
	results := findChild(t, root, "results")
	if results.ID.Path != "/results" {
		t.Fatalf("组应靠路径区分，实际 %q", results.ID.Path)
	}
}

func TestEmptyGroupChildren(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	empty := findChild(t, findChild(t, doc.Root(), "results"), "empty")
	kids, err := empty.Children()
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 0 {
		t.Fatalf("空组不应有子节点，实际 %v", names(kids))
	}
}

func TestNestedChildren(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	run1 := findChild(t, findChild(t, doc.Root(), "results"), "run-1")
	temp := findChild(t, run1, "temperature")
	if temp.Path != "/results/run-1/temperature" {
		t.Fatalf("嵌套路径不符: %q", temp.Path)
	}
	if temp.Name != "temperature" {
		t.Fatalf("Name 应只有最后一段，实际 %q", temp.Name)
	}
}

func TestDimensionSummary(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	matrix := findChild(t, doc.Root(), "matrix")
	if got := matrix.DimensionSummary(); got != "[100×50]" {
		t.Fatalf("维度概要应为 [100×50]，实际 %q", got)
	}
	// 组没有维度概要。
	if got := doc.Root().DimensionSummary(); got != "" {
		t.Fatalf("组不应有维度概要，实际 %q", got)
	}
}

func TestFormatShape(t *testing.T) {
	cases := []struct {
		shape []uint64
		want  string
	}{
		{nil, "[标量]"},
		{[]uint64{}, "[标量]"},
		{[]uint64{10}, "[10]"},
		{[]uint64{100, 50}, "[100×50]"},
		{[]uint64{2, 3, 4}, "[2×3×4]"},
	}
	for _, c := range cases {
		if got := FormatShape(c.shape); got != c.want {
			t.Errorf("FormatShape(%v) = %q，期望 %q", c.shape, got, c.want)
		}
	}
}

func TestDatasetInfoMatrix(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	info, err := findChild(t, doc.Root(), "matrix").Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Rank != 2 {
		t.Fatalf("秩应为 2，实际 %d", info.Rank)
	}
	if len(info.Shape) != 2 || info.Shape[0] != 100 || info.Shape[1] != 50 {
		t.Fatalf("维度应为 [100 50]，实际 %v", info.Shape)
	}
	if info.ElemCount != 5000 {
		t.Fatalf("元素总数应为 5000，实际 %d", info.ElemCount)
	}
	if info.Overflow {
		t.Fatal("元素总数不应溢出")
	}
	// 夹具没有写 maxdims，解析库回落到当前维度。
	if len(info.MaxShape) != 2 || info.MaxShape[0] != 100 || info.MaxShape[1] != 50 {
		t.Fatalf("最大维度应回落到当前维度，实际 %v", info.MaxShape)
	}
	for i := 0; i < 2; i++ {
		if info.MaxUnlimited(i) {
			t.Fatalf("第 %d 维不应为无限", i)
		}
	}
	if info.Datatype.Class != libhdf5.ClassFloat {
		t.Fatalf("类型应为 float，实际 %v", info.Datatype.Class)
	}
	if info.Datatype.Size != 8 {
		t.Fatalf("元素大小应为 8，实际 %d", info.Datatype.Size)
	}
	if info.Layout.Class != libhdf5.LayoutContiguous {
		t.Fatalf("布局应为 contiguous，实际 %v", info.Layout.Class)
	}
	if len(info.Layout.Filters) != 0 {
		t.Fatalf("不应有过滤器，实际 %v", info.Layout.Filters)
	}
}

func TestDatasetInfoUnlimited(t *testing.T) {
	doc := openFixture(t, "big.h5")
	info, err := findChild(t, doc.Root(), "wide").Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Rank != 2 {
		t.Fatalf("秩应为 2，实际 %d", info.Rank)
	}
	if info.ElemCount != 200000 {
		t.Fatalf("元素总数应为 200000，实际 %d", info.ElemCount)
	}
	if info.Layout.Class != libhdf5.LayoutChunked {
		t.Fatalf("布局应为 chunked，实际 %v", info.Layout.Class)
	}
	if len(info.Layout.ChunkDims) != 2 || info.Layout.ChunkDims[0] != 100 {
		t.Fatalf("分块尺寸应为 [100 100]，实际 %v", info.Layout.ChunkDims)
	}
}

func TestDatasetInfoFilters(t *testing.T) {
	doc := openFixture(t, "compressed.h5")
	info, err := findChild(t, doc.Root(), "compressed").Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Layout.Class != libhdf5.LayoutChunked {
		t.Fatalf("布局应为 chunked，实际 %v", info.Layout.Class)
	}
	if len(info.Layout.Filters) != 2 {
		t.Fatalf("应有 2 个过滤器，实际 %v", info.Layout.Filters)
	}
	// 顺序即应用顺序：shuffle 在前，deflate 在后。
	if info.Layout.Filters[0].ID != libhdf5.FilterShuffle {
		t.Fatalf("第一个过滤器应为 shuffle，实际 %v", info.Layout.Filters[0].ID)
	}
	if info.Layout.Filters[1].ID != libhdf5.FilterDeflate {
		t.Fatalf("第二个过滤器应为 deflate，实际 %v", info.Layout.Filters[1].ID)
	}
	for _, f := range info.Layout.Filters {
		if !f.Known {
			t.Fatalf("过滤器 %v 应被识别为已知", f.ID)
		}
	}
	// deflate 的 level 6 存在客户端参数里。
	if len(info.Layout.Filters[1].ClientData) == 0 || info.Layout.Filters[1].ClientData[0] != 6 {
		t.Fatalf("deflate 的级别应为 6，实际 %v", info.Layout.Filters[1].ClientData)
	}
}

func TestDatasetInfoCompound(t *testing.T) {
	doc := openFixture(t, "types.h5")
	info, err := findChild(t, doc.Root(), "compound").Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Datatype.Class != libhdf5.ClassCompound {
		t.Fatalf("类型应为 compound，实际 %v", info.Datatype.Class)
	}
	if len(info.Datatype.Members) != 2 {
		t.Fatalf("应有 2 个成员，实际 %d", len(info.Datatype.Members))
	}
	if info.Datatype.Members[0].Name != "id" || info.Datatype.Members[0].Offset != 0 {
		t.Fatalf("第一个成员应为 id@0，实际 %+v", info.Datatype.Members[0])
	}
	if info.Datatype.Members[1].Name != "value" || info.Datatype.Members[1].Offset != 8 {
		t.Fatalf("第二个成员应为 value@8，实际 %+v", info.Datatype.Members[1])
	}
}

func TestDatasetInfoEnum(t *testing.T) {
	doc := openFixture(t, "types.h5")
	node := findChild(t, doc.Root(), "enum_days")
	info, err := node.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Datatype.Class != libhdf5.ClassEnum {
		t.Fatalf("类型应为 enum，实际 %v", info.Datatype.Class)
	}
	if len(info.Datatype.EnumMembers) != 7 {
		t.Fatalf("应有 7 个枚举成员，实际 %d", len(info.Datatype.EnumMembers))
	}
	if got, ok := info.EnumName(2); !ok || got != "周三" {
		t.Fatalf("值 2 应映射为 周三，实际 %q ok=%v", got, ok)
	}
	if _, ok := info.EnumName(99); ok {
		t.Fatal("未知值不应命中成员表")
	}
}

func TestInfoNotDataset(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	if _, err := doc.Root().Info(); !errors.Is(err, ErrNotDataset) {
		t.Fatalf("对组调用 Info 应返回 ErrNotDataset，实际: %v", err)
	}
}

func TestGroupInfo(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	root := doc.Root()
	gi, err := root.GroupInfo()
	if err != nil {
		t.Fatal(err)
	}
	if gi.MemberCount != 2 {
		t.Fatalf("根组成员数应为 2，实际 %d", gi.MemberCount)
	}
	if gi.AttributeCount != 0 {
		t.Fatalf("根组不应有属性，实际 %d", gi.AttributeCount)
	}

	matrix := findChild(t, root, "matrix")
	if _, err := matrix.GroupInfo(); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("对数据集调用 GroupInfo 应返回 ErrNotGroup，实际: %v", err)
	}
}

func TestAttributesDataset(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	matrix := findChild(t, doc.Root(), "matrix")
	attrs, err := matrix.Attributes()
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 2 {
		t.Fatalf("应有 2 个属性，实际 %d", len(attrs))
	}
	byName := map[string]AttributeInfo{}
	for _, a := range attrs {
		byName[a.Name] = a
	}
	units, ok := byName["units"]
	if !ok {
		t.Fatalf("缺少 units 属性，实际 %v", attrs)
	}
	if units.Value != "K" {
		t.Fatalf("units 值应为 K，实际 %q", units.Value)
	}
	if units.IsArray {
		t.Fatal("units 不应是数组")
	}
	if units.Error != "" {
		t.Fatalf("units 读取不应报错: %s", units.Error)
	}

	scale, ok := byName["scale_factor"]
	if !ok {
		t.Fatalf("缺少 scale_factor 属性，实际 %v", attrs)
	}
	if !scale.IsArray {
		t.Fatal("scale_factor 应是数组")
	}
	if scale.ElemNum != 2 {
		t.Fatalf("scale_factor 应有 2 个元素，实际 %d", scale.ElemNum)
	}
	if scale.Value != "1.5, 2.5" {
		t.Fatalf("scale_factor 值不符: %q", scale.Value)
	}
}

func TestAttributesEmpty(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	empty := findChild(t, findChild(t, doc.Root(), "results"), "empty")
	attrs, err := empty.Attributes()
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 0 {
		t.Fatalf("空组不应有属性，实际 %v", attrs)
	}
}

func TestReadSliceMatrix(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	matrix := findChild(t, doc.Root(), "matrix")
	s, err := doc.ReadSlice(matrix, []uint64{0, 0}, []uint64{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Values) != 6 {
		t.Fatalf("应读到 6 个值，实际 %d", len(s.Values))
	}
	// matrix[r][c] = r + c/100
	want := []float64{0, 0.01, 0.02, 1, 1.01, 1.02}
	for i, w := range want {
		if diff := s.Values[i] - w; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("第 %d 个值应为 %v，实际 %v", i, w, s.Values[i])
		}
	}
	if len(s.Shape) != 2 || s.Shape[0] != 100 {
		t.Fatalf("Shape 应为数据集整体维度，实际 %v", s.Shape)
	}
	if len(s.Start) != 2 || s.Start[0] != 0 || s.Start[1] != 0 {
		t.Fatalf("Start 不符: %v", s.Start)
	}
	if len(s.Count) != 2 || s.Count[0] != 2 || s.Count[1] != 3 {
		t.Fatalf("Count 不符: %v", s.Count)
	}
}

func TestReadSliceOutOfBounds(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	matrix := findChild(t, doc.Root(), "matrix")

	if _, err := doc.ReadSlice(matrix, []uint64{99, 0}, []uint64{2, 1}); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("越界应返回 ErrOutOfBounds，实际: %v", err)
	}
	if _, err := doc.ReadSlice(matrix, []uint64{0, 0}, []uint64{101, 1}); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("count 超出维度应返回 ErrOutOfBounds，实际: %v", err)
	}
	if _, err := doc.ReadSlice(matrix, []uint64{0}, []uint64{1}); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("维度数不匹配应返回 ErrOutOfBounds，实际: %v", err)
	}
}

func TestReadSliceTooLarge(t *testing.T) {
	doc := openFixture(t, "big.h5")
	wide := findChild(t, doc.Root(), "wide")
	doc.SetReadLimit(1024) // 128 个 float64

	if _, err := doc.ReadSlice(wide, []uint64{0, 0}, []uint64{10, 100}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("超过上限应返回 ErrTooLarge，实际: %v", err)
	}
	// 上限之内的请求照常成功。
	if _, err := doc.ReadSlice(wide, []uint64{0, 0}, []uint64{1, 10}); err != nil {
		t.Fatalf("上限内的请求应成功: %v", err)
	}
}

func TestReadSliceNotDataset(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	if _, err := doc.ReadSlice(doc.Root(), []uint64{0}, []uint64{1}); !errors.Is(err, ErrNotDataset) {
		t.Fatalf("对组读取应返回 ErrNotDataset，实际: %v", err)
	}
}

func TestReadSliceBigIntegers(t *testing.T) {
	doc := openFixture(t, "types.h5")
	node := findChild(t, doc.Root(), "uint64_big")
	s, err := doc.ReadSlice(node, []uint64{0}, []uint64{2})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Values) != 2 {
		t.Fatalf("应读到 2 个值，实际 %d", len(s.Values))
	}
	// 2^63 是 2 的幂，float64 可精确表示。
	if s.Values[0] != math.Pow(2, 63) {
		t.Fatalf("第一个值应为 2^63，实际 %v", s.Values[0])
	}
	// MaxUint64 超出 float64 精确范围，必然损失精度——视图层负责标注。
	if s.Values[1] <= 0 {
		t.Fatalf("无符号大值不应变为负数，实际 %v", s.Values[1])
	}
}

func TestReadStrings(t *testing.T) {
	doc := openFixture(t, "strings.h5")
	node := findChild(t, doc.Root(), "names")
	s, err := doc.ReadStrings(node)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "gamma", "delta"}
	if len(s.Values) != len(want) {
		t.Fatalf("应读到 %d 个字符串，实际 %d", len(want), len(s.Values))
	}
	for i, w := range want {
		if s.Values[i] != w {
			t.Fatalf("第 %d 个字符串应为 %q，实际 %q", i, w, s.Values[i])
		}
	}
}

func TestReadStringsNotDataset(t *testing.T) {
	doc := openFixture(t, "strings.h5")
	if _, err := doc.ReadStrings(doc.Root()); !errors.Is(err, ErrNotDataset) {
		t.Fatalf("对组读取应返回 ErrNotDataset，实际: %v", err)
	}
}

func TestReadCompound(t *testing.T) {
	doc := openFixture(t, "types.h5")
	node := findChild(t, doc.Root(), "compound")
	c, err := doc.ReadCompound(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Members) != 2 || c.Members[0] != "id" || c.Members[1] != "value" {
		t.Fatalf("成员应为 [id value]，实际 %v", c.Members)
	}
	if len(c.Rows) != 3 {
		t.Fatalf("应有 3 行，实际 %d", len(c.Rows))
	}
	if got := toInt(c.Rows[1]["id"]); got != 2 {
		t.Fatalf("第二行 id 应为 2，实际 %v", c.Rows[1]["id"])
	}
	if got := toFloat(c.Rows[2]["value"]); got != 3.75 {
		t.Fatalf("第三行 value 应为 3.75，实际 %v", c.Rows[2]["value"])
	}
}

func TestNodeByPath(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	n, err := doc.NodeByPath("/results/run-1/temperature")
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "temperature" {
		t.Fatalf("路径查找结果不符: %q", n.Name)
	}
	if _, err := doc.NodeByPath("/不存在"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的路径应返回 ErrNotFound，实际: %v", err)
	}
	root, err := doc.NodeByPath("/")
	if err != nil || root != doc.Root() {
		t.Fatalf("根路径应返回根节点: %v %v", root, err)
	}
}

func TestHasParent(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	n, err := doc.NodeByPath("/results/run-1/temperature")
	if err != nil {
		t.Fatal(err)
	}
	if !n.HasParent("/results") {
		t.Fatal("/results 应是其祖先")
	}
	if n.HasParent("/results/run-1/temperature") {
		t.Fatal("自身不应算作祖先")
	}
	if n.HasParent("/matrix") {
		t.Fatal("/matrix 不是其祖先")
	}
}

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
		{1.5, "1.5"},
		{0, "0"},
		{-2.25, "-2.25"},
	}
	for _, c := range cases {
		if got := formatFloat(c.in); got != c.want {
			t.Errorf("formatFloat(%v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestElemCountOverflow(t *testing.T) {
	count, overflow := elemCount([]uint64{1 << 40, 1 << 40})
	if !overflow {
		t.Fatal("超大维度应判定为溢出")
	}
	if count != math.MaxUint64 {
		t.Fatalf("溢出时应返回 MaxUint64，实际 %d", count)
	}
	if c, o := elemCount(nil); o || c != 1 {
		t.Fatalf("标量元素数应为 1，实际 %d overflow=%v", c, o)
	}
}

func TestClassifySentinels(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{fmtWrap(libhdf5.ErrNotSupported), ErrUnsupported},
		{fmtWrap(libhdf5.ErrCorrupt), ErrCorrupt},
		{fmtWrap(libhdf5.ErrLimit), ErrTooLarge},
		{fmtWrap(libhdf5.ErrNeedsRepack), ErrUnsupported},
		{fmtWrap(libhdf5.ErrNoData), ErrUnsupported},
	}
	for _, c := range cases {
		got := classify(c.in)
		if !errors.Is(got, c.want) {
			t.Errorf("classify(%v) 应包裹 %v，实际: %v", c.in, c.want, got)
		}
	}
}

func TestClassifyTextFallbacks(t *testing.T) {
	if got := classify(errText("not an HDF5 file")); !errors.Is(got, ErrNotHDF5) {
		t.Fatalf("签名错误应归类为 ErrNotHDF5，实际: %v", got)
	}
	cycle := classify(errText("object graph cycle at address 0x10"))
	if !errors.Is(cycle, ErrUnsupported) {
		t.Fatalf("硬链接环应归类为 ErrUnsupported，实际: %v", cycle)
	}
	if got := classify(errText("随便什么错")); !errors.Is(got, ErrCorrupt) {
		t.Fatalf("未知错误应归入 ErrCorrupt，实际: %v", got)
	}
}

func TestBaseName(t *testing.T) {
	if got := BaseName("/results/run-1/temperature"); got != "temperature" {
		t.Fatalf("BaseName 不符: %q", got)
	}
	if got := BaseName("/"); got != "/" {
		t.Fatalf("根的 BaseName 应为 /，实际 %q", got)
	}
}

func TestJoinPath(t *testing.T) {
	cases := []struct{ parent, name, want string }{
		{"/", "a", "/a"},
		{"", "a", "/a"},
		{"/a", "b", "/a/b"},
		{"/a/b", "c", "/a/b/c"},
	}
	for _, c := range cases {
		if got := joinPath(c.parent, c.name); got != c.want {
			t.Errorf("joinPath(%q, %q) = %q，期望 %q", c.parent, c.name, got, c.want)
		}
	}
}

func TestNodeKindString(t *testing.T) {
	cases := map[NodeKind]string{
		KindGroup:        "组",
		KindDataset:      "数据集",
		KindNamedType:    "命名类型",
		KindSoftLink:     "软链接",
		KindExternalLink: "外部链接",
		NodeKind(0):      "未知",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q，期望 %q", k, got, want)
		}
	}
}

// --- 小工具 ---

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	}
	return -1
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	}
	return -1
}

// TestReadStrided 验证步长读取：只解码抽取到的像素（任务 6.4）。
func TestReadStrided(t *testing.T) {
	doc := openFixture(t, "basic.h5")
	node, err := doc.NodeByPath("/matrix")
	if err != nil {
		t.Fatalf("NodeByPath: %v", err)
	}

	// 取 10 行 × 5 列，行步长 10、列步长 10。
	sl, err := doc.ReadStrided(node, 0, 0, 10, 5, 10, 10)
	if err != nil {
		t.Fatalf("ReadStrided: %v", err)
	}
	if got := len(sl.Values); got != 50 {
		t.Fatalf("应返回 50 个值，实际 %d", got)
	}
	// 夹具值 = 行 + 列/100：第一个像素 (0,0)=0，第二个 (0,10)=0.1。
	if sl.Values[0] != 0 {
		t.Fatalf("首个像素应为 0，实际 %v", sl.Values[0])
	}
	if math.Abs(sl.Values[1]-0.1) > 1e-9 {
		t.Fatalf("第二像素应为 0.1，实际 %v", sl.Values[1])
	}
	// 第二行第一列应是第 10 行：(10,0)=10。
	if sl.Values[5] != 10 {
		t.Fatalf("第二行首像素应为 10，实际 %v", sl.Values[5])
	}

	// 越界应报 ErrOutOfBounds。
	if _, err := doc.ReadStrided(node, 95, 0, 10, 5, 10, 10); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("越界应报 ErrOutOfBounds，实际 %v", err)
	}
	// 三维数据集应被拒绝（/cube 在 scalar.h5 里）。
	doc3 := openFixture(t, "scalar.h5")
	cube, err := doc3.NodeByPath("/cube")
	if err != nil {
		t.Fatalf("NodeByPath: %v", err)
	}
	if _, err := doc3.ReadStrided(cube, 0, 0, 1, 1, 1, 1); !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("三维数据集应报 ErrOutOfBounds，实际 %v", err)
	}
}
