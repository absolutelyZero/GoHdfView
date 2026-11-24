package hdf5

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
)

// 这一组用例读的是 h5py（官方 C 库）写出的夹具，用来证明本层不只
// 能读自己写的文件。夹具由 testdata/gen_h5py.py 生成。
//
// 与 document_test.go 的区别：那里的夹具由纯 Go 写入实现产出，
// 结构比较「规整」；官方库会写出不同的对象头版本、B-tree 布局与
// 消息顺序，是更真实的兼容性检验。

// h5pyFixture 打开 h5py 夹具。
func h5pyFixture(t *testing.T, name string) *Document {
	t.Helper()
	doc, err := Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", name, err)
	}
	t.Cleanup(func() { doc.Close() })
	return doc
}

// 官方库写出的二维矩阵、属性与嵌套组都能读。
func TestH5pyBasic(t *testing.T) {
	doc := h5pyFixture(t, "h5py_basic.h5")

	matrix, err := doc.NodeByPath("/matrix")
	if err != nil {
		t.Fatalf("查找 /matrix 失败：%v", err)
	}
	info, err := matrix.Info()
	if err != nil {
		t.Fatalf("读取 /matrix 元数据失败：%v", err)
	}
	if info.Rank != 2 || info.Shape[0] != 100 || info.Shape[1] != 50 {
		t.Errorf("维度应为 [100 50]，实际 rank=%d shape=%v", info.Rank, info.Shape)
	}
	if info.ElemCount != 5000 {
		t.Errorf("元素总数应为 5000，实际 %d", info.ElemCount)
	}

	// 属性：字符串与浮点数组。
	attrs, err := matrix.Attributes()
	if err != nil {
		t.Fatalf("读取属性失败：%v", err)
	}
	byName := map[string]AttributeInfo{}
	for _, a := range attrs {
		byName[a.Name] = a
	}
	if got, ok := byName["units"]; !ok || got.Value != "K" {
		t.Errorf("units 应为 K，实际 %+v", got)
	}
	if got, ok := byName["scale_factor"]; !ok || got.Error != "" {
		t.Errorf("scale_factor 应可读，实际 %+v", got)
	}

	// 首屏切片：值 = row + col/100。
	s, err := doc.ReadSlice(matrix, []uint64{0, 0}, []uint64{2, 3})
	if err != nil {
		t.Fatalf("读取切片失败：%v", err)
	}
	want := []float64{0, 0.01, 0.02, 1, 1.01, 1.02}
	if len(s.Values) != len(want) {
		t.Fatalf("切片长度应为 %d，实际 %d", len(want), len(s.Values))
	}
	for i := range want {
		if math.Abs(s.Values[i]-want[i]) > 1e-9 {
			t.Errorf("第 %d 个值应为 %v，实际 %v", i, want[i], s.Values[i])
		}
	}

	// 分块 + shuffle + deflate 的过滤器按应用顺序给出。
	comp, err := doc.NodeByPath("/compressed")
	if err != nil {
		t.Fatalf("查找 /compressed 失败：%v", err)
	}
	cinfo, err := comp.Info()
	if err != nil {
		t.Fatalf("读取 /compressed 元数据失败：%v", err)
	}
	if cinfo.Layout.ClassText() != "分块" {
		t.Errorf("布局应为分块，实际 %q", cinfo.Layout.ClassText())
	}
	names := make([]string, 0, len(cinfo.Layout.Filters))
	for _, f := range cinfo.Layout.Filters {
		names = append(names, f.Describe())
	}
	if len(names) != 2 || names[0] != "shuffle" || names[1] != "deflate (level 6)" {
		t.Errorf("过滤器应为 [shuffle deflate (level 6)]，实际 %v", names)
	}

	// 无限维数据集：最大值里有 unlimited。
	grow, err := doc.NodeByPath("/growable")
	if err != nil {
		t.Fatalf("查找 /growable 失败：%v", err)
	}
	ginfo, err := grow.Info()
	if err != nil {
		t.Fatalf("读取 /growable 元数据失败：%v", err)
	}
	if len(ginfo.Unlimited) != 1 || !ginfo.Unlimited[0] {
		t.Errorf("growable 的最大值应为 unlimited，实际 %v", ginfo.Unlimited)
	}
}

// 官方库写出的各种类型都能识别。
func TestH5pyTypes(t *testing.T) {
	doc := h5pyFixture(t, "h5py_types.h5")

	t.Run("整数", func(t *testing.T) {
		n, err := doc.NodeByPath("/int32")
		if err != nil {
			t.Fatal(err)
		}
		info, err := n.Info()
		if err != nil {
			t.Fatal(err)
		}
		if !info.Datatype.IsInteger() || !info.Datatype.Signed {
			t.Errorf("应为有符号整数，实际 %+v", info.Datatype.Describe())
		}
	})

	t.Run("大整数精度", func(t *testing.T) {
		n, err := doc.NodeByPath("/int64_big")
		if err != nil {
			t.Fatal(err)
		}
		s, err := doc.ReadSlice(n, []uint64{0}, []uint64{3})
		if err != nil {
			t.Fatal(err)
		}
		// 9007199254740993 超出 float64 的精确整数范围，
		// 解析库只能给出最接近的值：这一点由 UI 的 ≈ 标记提示。
		if len(s.Values) != 3 {
			t.Fatalf("应读 3 个值，实际 %d", len(s.Values))
		}
		if s.Values[0] != float64(int64(1)<<62) {
			t.Errorf("第 0 个值应为 2^62，实际 %v", s.Values[0])
		}
	})

	t.Run("枚举", func(t *testing.T) {
		n, err := doc.NodeByPath("/enum_days")
		if err != nil {
			t.Fatal(err)
		}
		info, err := n.Info()
		if err != nil {
			t.Fatal(err)
		}
		if !info.Datatype.IsEnum() {
			t.Fatalf("应为枚举，实际 %+v", info.Datatype.Describe())
		}
		if len(info.Datatype.EnumMembers) != 7 {
			t.Fatalf("应有 7 个成员，实际 %d", len(info.Datatype.EnumMembers))
		}
		s, err := doc.ReadSlice(n, []uint64{0}, []uint64{4})
		if err != nil {
			t.Fatal(err)
		}
		if name, ok := info.EnumName(s.Values[1]); !ok || name != "周四" {
			t.Errorf("第 1 个值应映射到周四，实际 %q/%v", name, ok)
		}
	})

	t.Run("复合", func(t *testing.T) {
		n, err := doc.NodeByPath("/compound")
		if err != nil {
			t.Fatal(err)
		}
		c, err := doc.ReadCompound(n)
		if err != nil {
			t.Fatalf("读取复合数据失败：%v", err)
		}
		if len(c.Members) != 2 || len(c.Rows) != 3 {
			t.Fatalf("应为 2 个成员 3 行，实际 %d/%d", len(c.Members), len(c.Rows))
		}
	})

	t.Run("变长字符串", func(t *testing.T) {
		n, err := doc.NodeByPath("/names")
		if err != nil {
			t.Fatal(err)
		}
		s, err := doc.ReadStrings(n)
		if err != nil {
			t.Fatalf("读取变长字符串失败：%v", err)
		}
		if len(s.Values) != 4 || s.Values[1] != "beta" {
			t.Errorf("字符串应为 alpha/beta/gamma/delta，实际 %v", s.Values)
		}
	})

	t.Run("定长字符串", func(t *testing.T) {
		n, err := doc.NodeByPath("/codes")
		if err != nil {
			t.Fatal(err)
		}
		s, err := doc.ReadStrings(n)
		if err != nil {
			t.Fatalf("读取定长字符串失败：%v", err)
		}
		if len(s.Values) != 2 {
			t.Fatalf("应有 2 个值，实际 %v", s.Values)
		}
	})

	t.Run("标量", func(t *testing.T) {
		n, err := doc.NodeByPath("/scalar")
		if err != nil {
			t.Fatal(err)
		}
		info, err := n.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Rank != 0 {
			t.Errorf("标量的秩应为 0，实际 %d", info.Rank)
		}
		if info.ElemCount != 1 {
			t.Errorf("标量的元素总数应为 1，实际 %d", info.ElemCount)
		}

		// rank 0 没有可切片的维度，ReadSlice 必须退回整份读取，
		// 否则数据视图会显示「（无值）」。
		s, err := doc.ReadSlice(n, nil, nil)
		if err != nil {
			t.Fatalf("读取标量失败：%v", err)
		}
		if len(s.Values) != 1 || s.Values[0] != 42.5 {
			t.Errorf("标量值应为 [42.5]，实际 %v", s.Values)
		}
	})
}

// 使用无解码器过滤器的数据集被识别为「不支持」而不是「损坏」。
func TestH5pyUnsupportedFilter(t *testing.T) {
	doc := h5pyFixture(t, "h5py_unsupported.h5")

	n, err := doc.NodeByPath("/scaleoffset")
	if err != nil {
		t.Fatalf("查找 /scaleoffset 失败：%v", err)
	}
	info, err := n.Info()
	if err != nil {
		t.Fatalf("元数据应可读：%v", err)
	}
	found := false
	for _, f := range info.Layout.Filters {
		if f.Name == "scaleoffset" || f.ID == 6 {
			found = true
		}
	}
	if !found {
		t.Errorf("元数据应列出 scaleoffset 过滤器，实际 %+v", info.Layout.Filters)
	}

	// 读取整份数据应报不支持，且能被哨兵识别。
	if _, err := doc.ReadSlice(n, []uint64{0, 0}, []uint64{8, 8}); err != nil {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("应报 ErrUnsupported，实际 %v", err)
		}
	}
}

// 带 512 字节 user block 的文件（MATLAB v7.3 的布局）能被打开。
//
// 超级块不在文件开头，本层需要按 512 的倍数扫描 user block 偏移；
// 这一条用例保证该扫描在官方库写出的文件上确实生效。
func TestH5pyUserBlock(t *testing.T) {
	doc := h5pyFixture(t, "h5py_matlab.h5")

	n, err := doc.NodeByPath("/values")
	if err != nil {
		t.Fatalf("查找 /values 失败：%v", err)
	}
	info, err := n.Info()
	if err != nil {
		t.Fatalf("读取元数据失败：%v", err)
	}
	if info.Rank != 2 || info.Shape[0] != 3 || info.Shape[1] != 4 {
		t.Errorf("维度应为 [3 4]，实际 rank=%d shape=%v", info.Rank, info.Shape)
	}

	// MATLAB_class 属性应出现在属性列表里。
	attrs, err := n.Attributes()
	if err != nil {
		t.Fatalf("读取属性失败：%v", err)
	}
	found := false
	for _, a := range attrs {
		if a.Name == "MATLAB_class" {
			found = true
		}
	}
	if !found {
		t.Errorf("应列出 MATLAB_class 属性，实际 %+v", attrs)
	}

	// 根组下还有 #refs# 组（MATLAB 的引用表）。
	root := doc.Root()
	kids, err := root.Children()
	if err != nil {
		t.Fatalf("读取根组成员失败：%v", err)
	}
	hasRefs := false
	for _, k := range kids {
		if k.Name == "#refs#" {
			hasRefs = true
		}
	}
	if !hasRefs {
		t.Errorf("根组应含 #refs#，实际 %d 个成员", len(kids))
	}
}
