package view

import (
	"fmt"
	"image"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// 图像视图的固定参数。
const (
	// imageTargetPixels 是目标像素数上限：降采样后按它估算步长，
	// 保证一次读取的元素数是常数级而不是随数据集线性增长（任务 6.4）。
	imageTargetPixels = 512 * 512
	// imageMaxSide 是单边像素上限，避免细长数据集把位图拉得过大。
	imageMaxSide = 2048
	// imagePageRows 是降采样时每批读取的行数，控制单次分配量。
	imagePageRows = 64
)

// ImageState 是图像视图的跨帧状态。
type ImageState struct {
	// Min 与 Max 是值域覆盖；为空表示使用当前切片的实际极值。
	MinText string
	MaxText string
	// minVal 与 maxVal 是最近一次映射使用的实际值域。
	minVal float64
	maxVal float64
	// hasRange 报告 minVal/maxVal 是否有效。
	hasRange bool
	// bitmap 是最近一次渲染的位图。
	bitmap *ui.Bitmap
	// key 是最近一次渲染对应的请求，用于判断是否需要重建。
	key string
	// pixels 是最近一次映射的像素数，供测试断言降采样规模。
	pixels int
	// err 是最近一次读取失败的原因。
	err error
}

// ImageLayout 描述当前对象能否画成图像。
type ImageLayout struct {
	// OK 为 false 时 Reason 说明原因。
	OK     bool
	Reason string
	// Rows 与 Cols 是数据集的二维维度。
	Rows, Cols int
	// Kind 是数据形态；只有 readNumeric 才能画图。
	Kind readKind
}

// ImageLayoutFor 判断当前对象能否进入图像视图（任务 6.3）。
func (a *App) ImageLayoutFor() ImageLayout {
	n := a.Current
	if n == nil {
		return ImageLayout{Reason: "请选择一个对象"}
	}
	if n.Kind != hdf5.KindDataset {
		return ImageLayout{Reason: "只有数据集可以显示为图像"}
	}
	info, err := a.info(n)
	if err != nil {
		return ImageLayout{Reason: "读取数据集描述失败：" + err.Error()}
	}
	if info.Datatype.IsString() || info.Datatype.IsCompound() {
		return ImageLayout{
			Reason: "该数据集不是数值类型（" + info.Datatype.ClassText() + "），无法显示为图像",
		}
	}
	if !info.Datatype.IsNumeric() {
		return ImageLayout{
			Reason: "该数据集不是数值类型（" + info.Datatype.ClassText() + "），无法显示为图像",
		}
	}
	if info.Rank != 2 {
		return ImageLayout{
			Reason: "图像视图只支持二维数据集，当前为 " +
				strconv.Itoa(info.Rank) + " 维",
		}
	}
	return ImageLayout{
		OK:   true,
		Rows: int(info.Shape[0]),
		Cols: int(info.Shape[1]),
		Kind: readNumeric,
	}
}

// resetImage 清空图像状态；切换对象时调用。
func (a *App) resetImage() {
	a.Image = ImageState{}
}

// imageRange 解析用户输入的值域，非法或为空时回退到实际极值。
func (a *App) imageRange(minV, maxV float64) (float64, float64) {
	lo, hi := minV, maxV
	if v, err := strconv.ParseFloat(a.Image.MinText, 64); err == nil {
		lo = v
	}
	if v, err := strconv.ParseFloat(a.Image.MaxText, 64); err == nil {
		hi = v
	}
	if !(hi > lo) {
		// 值域退化（或输入非法）：退回实际极值，避免除零。
		return minV, maxV
	}
	return lo, hi
}

// downsampleStep 计算降采样的步长，使像素数不超过 imageTargetPixels。
func downsampleStep(rows, cols int) (stepRow, stepCol int) {
	stepRow, stepCol = 1, 1
	for (rows/stepRow)*(cols/stepCol) > imageTargetPixels {
		if rows/stepRow >= cols/stepCol {
			stepRow++
		} else {
			stepCol++
		}
	}
	return
}

// loadImage 读取（按需降采样的）数据并生成位图。
//
// 采样走 ReadStrided 的 Hyperslab stride：只解码落在网格上的元素，
// 读取量 = 像素数，与数据集总量无关（任务 6.4）。早期实现按块读再在块内
// 抽样，在 10000×10000 上会退化成整份读取，已弃用。
func (a *App) loadImage(l ImageLayout) {
	rd := a.reader()
	if rd == nil || a.Current == nil {
		return
	}

	stepRow, stepCol := downsampleStep(l.Rows, l.Cols)
	outRows := (l.Rows + stepRow - 1) / stepRow
	outCols := (l.Cols + stepCol - 1) / stepCol
	key := a.Current.ID.Path + "|" + strconv.Itoa(stepRow) + "x" +
		strconv.Itoa(stepCol) + "|" + a.Image.MinText + "|" + a.Image.MaxText
	if a.Image.key == key && a.Image.bitmap != nil && a.Image.err == nil {
		return // 同一请求已渲染，无需重建
	}

	// 用步长读取只解码目标像素：读取量 = 像素数，与数据集规模无关。
	sl, err := rd.ReadStrided(a.Current, 0, 0,
		uint64(outRows), uint64(outCols), uint64(stepRow), uint64(stepCol))
	a.Image.key = key
	a.Image.err = err
	if err != nil || sl == nil {
		a.Image.bitmap = nil
		return
	}
	vals := sl.Values
	if len(vals) == 0 {
		a.Image.bitmap = nil
		a.Image.err = fmt.Errorf("%w：数据集没有可显示的值", hdf5.ErrUnsupported)
		return
	}

	minV, maxV := vals[0], vals[0]
	for _, v := range vals {
		if math.IsNaN(v) {
			continue
		}
		minV = math.Min(minV, v)
		maxV = math.Max(maxV, v)
	}
	a.Image.minVal, a.Image.maxVal, a.Image.hasRange = minV, maxV, true
	lo, hi := a.imageRange(minV, maxV)
	img := grayscale(vals, outCols, outRows, lo, hi)
	a.Image.pixels = len(vals)
	if a.Image.bitmap == nil {
		a.Image.bitmap = ui.NewBitmap(img)
	} else {
		a.Image.bitmap.Update(img)
	}
}

// grayscale 把数值序列映射成灰度图。
//
// 越界值被夹到端点；NaN 映射为最暗（0），因为它在数据里通常表示无效区域。
func grayscale(vals []float64, cols, rows int, lo, hi float64) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, cols, rows))
	span := hi - lo
	if !(span > 0) {
		span = 1
	}
	for i := 0; i < rows*cols && i < len(vals); i++ {
		v := vals[i]
		var g uint8
		switch {
		case math.IsNaN(v):
			g = 0
		case v <= lo:
			g = 0
		case v >= hi:
			g = 255
		default:
			g = uint8(math.Round((v - lo) / span * 255))
		}
		img.Pix[i] = g
	}
	return img
}
