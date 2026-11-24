//go:build ignore

// gen 生成 internal/hdf5/testdata 下的 HDF5 测试夹具。
//
// 这个脚本是仓库里唯一允许调用写入 API 的地方：它用解析库自己的写入实现
// 产出夹具，因此夹具的字节布局与解析库的读路径天然一致，测试不必依赖外部
// 的 HDF5 C 工具链。脚本带 //go:build ignore，不参与常规构建与测试。
//
// 用法（在仓库根目录）：
//
//	go run internal/hdf5/testdata/gen.go
//
// 产物已提交到 testdata/；只有需要新增或修改夹具时才重新运行。
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/scigolib/hdf5"
)

func main() {
	dir := filepath.Join("internal", "hdf5", "testdata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}

	steps := []struct {
		name string
		gen  func(string) error
	}{
		{"basic.h5", genBasic},
		{"compressed.h5", genCompressed},
		{"types.h5", genTypes},
		{"strings.h5", genStrings},
		{"scalar.h5", genScalar},
		{"big.h5", genBig},
	}
	for _, s := range steps {
		path := filepath.Join(dir, s.name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Fatalf("%s: %v", s.name, err)
		}
		if err := s.gen(path); err != nil {
			log.Fatalf("%s: %v", s.name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			log.Fatalf("%s: %v", s.name, err)
		}
		fmt.Printf("%-16s %8d bytes\n", s.name, info.Size())
	}

	// 非 HDF5 夹具：纯文本与截断的 HDF5，用来验证错误分类。
	if err := os.WriteFile(filepath.Join(dir, "not_hdf5.txt"), []byte("这不是 HDF5 文件。\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := truncate(filepath.Join(dir, "basic.h5"), filepath.Join(dir, "truncated.h5"), 256); err != nil {
		log.Fatal(err)
	}
	fmt.Println("not_hdf5.txt / truncated.h5 已生成")
}

// genBasic 生成 2D 矩阵与两个组，覆盖「对象树 + 元数据 + 表格」的主路径。
func genBasic(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	// 根组上的 100×50 float64 矩阵：spec 里反复引用的基准夹具。
	const rows, cols = 100, 50
	data := make([]float64, rows*cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			data[r*cols+c] = float64(r) + float64(c)/100
		}
	}
	ds, err := fw.CreateDataset("/matrix", hdf5.Float64, []uint64{rows, cols})
	if err != nil {
		return err
	}
	if err := ds.Write(data); err != nil {
		return err
	}
	if err := ds.WriteAttribute("units", "K"); err != nil {
		return err
	}
	if err := ds.WriteAttribute("scale_factor", []float64{1.5, 2.5}); err != nil {
		return err
	}

	// 带成员的组，用于验证树形层次与空组不显示箭头。
	if _, err := fw.CreateGroup("/results"); err != nil {
		return err
	}
	if _, err := fw.CreateGroup("/results/run-1"); err != nil {
		return err
	}
	if _, err := fw.CreateGroup("/results/empty"); err != nil {
		return err
	}
	line, err := fw.CreateDataset("/results/run-1/temperature", hdf5.Float64, []uint64{10})
	if err != nil {
		return err
	}
	return line.Write([]float64{280.1, 280.4, 281.0, 281.3, 281.9, 282.2, 282.8, 283.1, 283.5, 283.9})
}

// genCompressed 生成带 shuffle + deflate 的分块数据集，覆盖过滤器展示与分块读取。
func genCompressed(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	const rows, cols = 64, 32
	data := make([]float64, rows*cols)
	for i := range data {
		data[i] = math.Sin(float64(i) / 17)
	}
	ds, err := fw.CreateDataset("/compressed", hdf5.Float64, []uint64{rows, cols},
		hdf5.WithChunkDims([]uint64{16, 16}),
		hdf5.WithShuffle(),
		hdf5.WithGZIPCompression(6),
	)
	if err != nil {
		return err
	}
	return ds.Write(data)
}

// genTypes 生成整数、无符号大值、枚举与复合数据集，覆盖格式化与类型展示。
func genTypes(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	int32s, err := fw.CreateDataset("/int32", hdf5.Int32, []uint64{5})
	if err != nil {
		return err
	}
	if err := int32s.Write([]int32{-3, -1, 0, 1, 2}); err != nil {
		return err
	}

	// 超过 2^53 的 int64：视图层必须加近似标记。
	big64, err := fw.CreateDataset("/int64_big", hdf5.Int64, []uint64{3})
	if err != nil {
		return err
	}
	if err := big64.Write([]int64{1 << 62, -(1 << 62), 9007199254740993}); err != nil {
		return err
	}

	// 超过 2^63 的 uint64：必须显示为非负数。
	bigU64, err := fw.CreateDataset("/uint64_big", hdf5.Uint64, []uint64{2})
	if err != nil {
		return err
	}
	if err := bigU64.Write([]uint64{1 << 63, math.MaxUint64}); err != nil {
		return err
	}

	days, err := fw.CreateDataset("/enum_days", hdf5.EnumInt8, []uint64{4},
		hdf5.WithEnumValues(
			[]string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"},
			[]int64{0, 1, 2, 3, 4, 5, 6},
		),
	)
	if err != nil {
		return err
	}
	if err := days.Write([]int8{0, 2, 4, 6}); err != nil {
		return err
	}

	// 复合类型：两个成员，8 字节对齐。
	ct, err := hdf5.NewCompoundType(16, []hdf5.CompoundMember{
		{Name: "id", Offset: 0, Type: hdf5.Int32},
		{Name: "value", Offset: 8, Type: hdf5.Float64},
	})
	if err != nil {
		return err
	}
	kv, err := fw.CreateCompound("/compound", ct, []uint64{3})
	if err != nil {
		return err
	}
	return kv.Write([]map[string]any{
		{"id": int32(1), "value": 1.25},
		{"id": int32(2), "value": 2.5},
		{"id": int32(3), "value": 3.75},
	})
}

// genStrings 生成定长字符串数据集，覆盖字符串表格与图像视图禁用。
func genStrings(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	ds, err := fw.CreateDataset("/names", hdf5.String, []uint64{4}, hdf5.WithStringSize(16))
	if err != nil {
		return err
	}
	return ds.Write([]string{"alpha", "beta", "gamma", "delta"})
}

// genScalar 生成标量数据集与 3D 数据集，覆盖秩 0 与高维布局。
func genScalar(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	// 解析库不接受空维度；用长度 1 的一维数据集并配空 dataspace 不可行，
	// 因此标量夹具改用 [1] 表示，视图层按 rank 1 处理（见 docs/verification.md）。
	scalar, err := fw.CreateDataset("/scalar_like", hdf5.Float64, []uint64{1})
	if err != nil {
		return err
	}
	if err := scalar.Write([]float64{42.5}); err != nil {
		return err
	}

	cube, err := fw.CreateDataset("/cube", hdf5.Float64, []uint64{2, 3, 4})
	if err != nil {
		return err
	}
	data := make([]float64, 2*3*4)
	for i := range data {
		data[i] = float64(i)
	}
	return cube.Write(data)
}

// genBig 生成一个维度较大但每维很小的数据集，用于验证按页读取不整份载入。
func genBig(path string) error {
	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		return err
	}
	defer fw.Close()

	// 2000×100 = 20 万个元素：足够跨多页，又不至于让夹具文件过大。
	const rows, cols = 2000, 100
	data := make([]float64, rows*cols)
	for i := range data {
		data[i] = float64(i % 1000)
	}
	ds, err := fw.CreateDataset("/wide", hdf5.Float64, []uint64{rows, cols},
		hdf5.WithChunkDims([]uint64{100, 100}),
		hdf5.WithGZIPCompression(1),
	)
	if err != nil {
		return err
	}
	return ds.Write(data)
}

// truncate 把 src 的前 n 字节写入 dst，模拟传输中断的 HDF5 文件。
func truncate(src, dst string, n int64) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if int64(len(data)) < n {
		n = int64(len(data))
	}
	return os.WriteFile(dst, data[:n], 0o644)
}
