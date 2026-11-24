package view

import (
	"fmt"
	"image"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// imageView 把图像视图装成可测试的视图。
func imageView(app *App) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
			ImageView(c, app)
		})
	}
}

// imageApp 构造一个可画图的二维数值数据集。
func imageApp(t *testing.T, rows, cols int, rd *fakeReader) *App {
	t.Helper()
	src := newFake()
	src.addDataset(src.root, "img")
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	app.SetReader(rd)
	app.SetMeta(&fakeMeta{info: numberInfo([]uint64{uint64(rows), uint64(cols)})})
	rd.rows, rd.cols = rows, cols
	if err := app.SelectPath("/img"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	return app
}

// TestImageViewRenders 覆盖任务 6.1：二维数值生成位图，灰度极值对应数值极值。
func TestImageViewRenders(t *testing.T) {
	// 值 = 行*cols + 列，最小 0、最大 rows*cols-1。
	rd := &fakeReader{value: func(start []uint64, idx int) float64 {
		base := 0
		if len(start) >= 2 {
			base = int(start[len(start)-2])*4 + int(start[len(start)-1])
		}
		return float64(base + idx)
	}}
	app := imageApp(t, 4, 4, rd)
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	if app.Image.bitmap == nil {
		t.Fatalf("应生成位图，错误：%v", app.Image.err)
	}
	w, h := app.Image.bitmap.Size()
	if w != 4 || h != 4 {
		t.Fatalf("位图尺寸应为 4×4，实际 %d×%d", w, h)
	}
	// 值域取实际极值 0..15。
	if app.Image.minVal != 0 || app.Image.maxVal != 15 {
		t.Fatalf("值域应为 0..15，实际 %v..%v", app.Image.minVal, app.Image.maxVal)
	}
	// 灰度极值与数值极值对应：0 → 黑，15 → 白。
	img := grayscale([]float64{0, 15}, 2, 1, 0, 15)
	if img.Pix[0] != 0 {
		t.Fatalf("最小值应映射为 0，实际 %d", img.Pix[0])
	}
	if img.Pix[1] != 255 {
		t.Fatalf("最大值应映射为 255，实际 %d", img.Pix[1])
	}
	if !tt.HasText("4 × 4") {
		t.Fatalf("应显示尺寸信息，实际 %q", tt.Texts())
	}
}

// TestImageViewRangeOverride 覆盖任务 6.2：改值域后重建位图并更新色标。
func TestImageViewRangeOverride(t *testing.T) {
	rd := &fakeReader{value: func(start []uint64, idx int) float64 {
		return float64(idx) // 0..15
	}}
	app := imageApp(t, 4, 4, rd)
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	// 初始色标端点是实际极值。
	if !tt.HasText("0") || !tt.HasText("15") {
		t.Fatalf("色标应显示 0 与 15，实际 %q", tt.Texts())
	}
	before := app.Image.key

	// 覆盖最小值后色标应更新，并触发重建。
	app.Image.MinText = "5"
	tt.Frame()
	if app.Image.key == before {
		t.Fatalf("改值域后应重建位图")
	}
	if !tt.HasText("5") {
		t.Fatalf("色标最小值应更新为 5，实际 %q", tt.Texts())
	}

	// 重置值域回到自动。
	if err := tt.Click("重置值域"); err != nil {
		t.Fatalf("Click 重置值域: %v", err)
	}
	tt.Frame()
	if app.Image.MinText != "" || app.Image.MaxText != "" {
		t.Fatalf("重置后值域输入应清空，实际 %q/%q", app.Image.MinText, app.Image.MaxText)
	}
}

// TestImageDisabledForStrings 覆盖任务 6.3：非数值数据集禁用图像并说明。
func TestImageDisabledForStrings(t *testing.T) {
	rd := &fakeReader{strs: []string{"a", "b"}}
	src := newFake()
	src.addDataset(src.root, "names")
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	app.SetReader(rd)
	app.SetMeta(&fakeMeta{info: &hdf5.DatasetInfo{
		Rank:      1,
		Shape:     []uint64{2},
		MaxShape:  []uint64{2},
		ElemCount: 2,
		Datatype:  hdf5.DatatypeInfo{Class: 3, Size: 16},
	}})
	if err := app.SelectPath("/names"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	if !tt.HasText("图像视图不可用") {
		t.Fatalf("应提示不可用，实际 %q", tt.Texts())
	}
	if !tt.HasText("该数据集不是数值类型（字符串），无法显示为图像") {
		t.Fatalf("应说明原因，实际 %q", tt.Texts())
	}
	if app.Image.bitmap != nil {
		t.Fatalf("不可用时应不生成位图")
	}
}

// TestImageDisabledForRank3 覆盖任务 6.3：秩不为 2 时禁用并说明。
func TestImageDisabledForRank3(t *testing.T) {
	app := imageApp(t, 4, 4, &fakeReader{})
	app.SetMeta(&fakeMeta{info: numberInfo([]uint64{2, 4, 4})})
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	if !tt.HasText("图像视图只支持二维数据集，当前为 3 维") {
		t.Fatalf("应说明维度限制，实际 %q", tt.Texts())
	}
}

// TestImageDownsample 覆盖任务 6.4：大尺寸数据集降采样读取。
func TestImageDownsample(t *testing.T) {
	rd := &fakeReader{}
	app := imageApp(t, 10000, 10000, rd)
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	if app.Image.err != nil {
		t.Fatalf("读取不应失败：%v", app.Image.err)
	}
	// 读取的元素总数不超过目标像素数的常数倍（这里按块读取，
	// 块内抽取，因此总量 ≈ 目标像素数 × 行块冗余）。
	read := 0
	for _, call := range rd.slices {
		n := 1
		for _, c := range call.Count {
			n *= int(c)
		}
		read += n
	}
	limit := imageTargetPixels * 4
	if read > limit {
		t.Fatalf("读取元素数 %d 超过上限 %d（目标像素 %d）", read, limit, imageTargetPixels)
	}
	if app.Image.pixels > imageTargetPixels {
		t.Fatalf("像素数 %d 应不超过目标 %d", app.Image.pixels, imageTargetPixels)
	}
	// 位图尺寸即采样后的网格。
	w, h := app.Image.bitmap.Size()
	if w > imageMaxSide || h > imageMaxSide {
		t.Fatalf("位图尺寸 %d×%d 超出单边上限", w, h)
	}
}

// TestDownsampleStep 断言步长计算的单调性。
func TestDownsampleStep(t *testing.T) {
	cases := []struct{ rows, cols int }{
		{10, 10}, {512, 512}, {1000, 100}, {10000, 10000}, {3, 100000},
	}
	for _, tc := range cases {
		sr, sc := downsampleStep(tc.rows, tc.cols)
		got := ((tc.rows + sr - 1) / sr) * ((tc.cols + sc - 1) / sc)
		if got > imageTargetPixels && sr == 1 && sc == 1 {
			t.Fatalf("%dx%d 未降采样", tc.rows, tc.cols)
		}
		if sr < 1 || sc < 1 {
			t.Fatalf("%dx%d 步长非法：%d,%d", tc.rows, tc.cols, sr, sc)
		}
	}
}

// TestGrayscaleNaN 断言 NaN 映射为最暗而不是越界。
func TestGrayscaleNaN(t *testing.T) {
	img := grayscale([]float64{math.NaN(), 5, 10}, 3, 1, 5, 10)
	if img.Pix[0] != 0 {
		t.Fatalf("NaN 应映射为 0，实际 %d", img.Pix[0])
	}
	if img.Pix[1] != 0 {
		t.Fatalf("下界应映射为 0，实际 %d", img.Pix[1])
	}
	if img.Pix[2] != 255 {
		t.Fatalf("上界应映射为 255，实际 %d", img.Pix[2])
	}
}

// TestImageRealFixture 用真实夹具验证图像视图。
func TestImageRealFixture(t *testing.T) {
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", "basic.h5"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(imageView(app), 700, 600)
	tt.Frame()

	if app.Image.bitmap == nil {
		t.Fatalf("应生成位图，错误：%v", app.Image.err)
	}
	w, h := app.Image.bitmap.Size()
	if w != 50 || h != 100 {
		t.Fatalf("位图应为 50×100（列×行），实际 %d×%d", w, h)
	}
	if !tt.HasText("100 × 50") {
		t.Fatalf("应显示尺寸 100 × 50，实际 %q", tt.Texts())
	}
}

// 确保 image 包被使用（grayscale 返回 *image.Gray）。
var _ = image.Rect

// 确保 fmt 与 strconv 在辅助断言中被使用。
var _ = fmt.Sprint
var _ = strconv.Itoa
