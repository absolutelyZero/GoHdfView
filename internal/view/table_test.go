package view

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// fakeReader 记录每次读取请求，并可注入错误或延迟。
type fakeReader struct {
	mu     sync.Mutex
	slices []sliceCall
	err    error
	// gate 非空时读取会阻塞到它被关闭，用于模拟慢读取。
	gate chan struct{}
	// value 是返回的数值生成函数，nil 表示用行列下标填充。
	value func(start []uint64, idx int) float64
	// strings 与 compound 是字符串/复合读取的返回值。
	strs   []string
	comp   *hdf5.Compound
	rows   int
	cols   int
	readNo int
}

type sliceCall struct {
	Start  []uint64
	Count  []uint64
	Stride []uint64
}

func (f *fakeReader) ReadSlice(node *hdf5.Node, start, count []uint64) (*hdf5.Slice, error) {
	f.mu.Lock()
	f.readNo++
	f.slices = append(f.slices, sliceCall{Start: append([]uint64(nil), start...), Count: append([]uint64(nil), count...)})
	gate := f.gate
	err := f.err
	f.mu.Unlock()

	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}

	// 按请求形状生成数值：值 = 行下标 + 列下标/100，
	// 便于断言「第 100 行起」这类偏移是否正确。
	total := 1
	for _, c := range count {
		total *= int(c)
	}
	vals := make([]float64, total)
	rows, cols := f.rows, f.cols
	if rows == 0 {
		rows, cols = 1, 1
	}
	for i := range vals {
		r := i / cols
		c := i % cols
		if len(start) >= 2 {
			r += int(start[len(start)-2])
			c += int(start[len(start)-1])
		}
		if f.value != nil {
			vals[i] = f.value(start, i)
			continue
		}
		vals[i] = float64(r) + float64(c)/100
	}
	return &hdf5.Slice{Start: start, Count: count, Values: vals}, nil
}

func (f *fakeReader) ReadStrings(node *hdf5.Node) (*hdf5.Strings, error) {
	f.mu.Lock()
	f.readNo++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &hdf5.Strings{Values: f.strs}, nil
}

func (f *fakeReader) ReadStrided(node *hdf5.Node, rowStart, colStart, rowCount, colCount, rowStride, colStride uint64) (*hdf5.Slice, error) {
	f.mu.Lock()
	f.readNo++
	f.slices = append(f.slices, sliceCall{
		Start:  []uint64{rowStart, colStart},
		Count:  []uint64{rowCount, colCount},
		Stride: []uint64{rowStride, colStride},
	})
	err := f.err
	rows, cols := f.rows, f.cols
	value := f.value
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	vals := make([]float64, int(rowCount*colCount))
	for i := range vals {
		if value != nil {
			vals[i] = value([]uint64{rowStart, colStart}, i)
			continue
		}
		r := int(rowStart) + (i/int(colCount))*int(rowStride)
		c := int(colStart) + (i%int(colCount))*int(colStride)
		vals[i] = float64(r) + float64(c)/100
	}
	return &hdf5.Slice{
		Start:  []uint64{rowStart, colStart},
		Count:  []uint64{rowCount, colCount},
		Shape:  []uint64{uint64(rows), uint64(cols)},
		Values: vals,
	}, nil
}

func (f *fakeReader) ReadCompound(node *hdf5.Node) (*hdf5.Compound, error) {
	f.mu.Lock()
	f.readNo++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.comp, nil
}

// fakeMeta 返回预设的数据集描述。
type fakeMeta struct {
	info *hdf5.DatasetInfo
	err  error
}

func (f *fakeMeta) Info(n *hdf5.Node) (*hdf5.DatasetInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.info, nil
}

// numberInfo 构造一个数值数据集的描述。
func numberInfo(shape []uint64) *hdf5.DatasetInfo {
	total := uint64(1)
	for _, d := range shape {
		total *= d
	}
	unlim := make([]bool, len(shape))
	max := append([]uint64(nil), shape...)
	return &hdf5.DatasetInfo{
		Rank:      len(shape),
		Shape:     shape,
		MaxShape:  max,
		Unlimited: unlim,
		ElemCount: total,
		Datatype: hdf5.DatatypeInfo{
			Class: 1, // 浮点：便于断言 0.01 这类小数值
			Size:  8,
		},
		Layout: hdf5.LayoutInfo{},
	}
}

// tableView 把表格视图装成可测试的视图。
func tableView(app *App) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
			DataTable(c, app)
		})
	}
}

// tableApp 构造一个带假读取器与假描述的应用。
func tableApp(t *testing.T, shape []uint64, rd *fakeReader) *App {
	t.Helper()
	src := newFake()
	src.addDataset(src.root, "data")
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	app.SetReader(rd)
	info := numberInfo(shape)
	app.SetMeta(&fakeMeta{info: info})
	rd.rows, rd.cols = 1, 1
	if len(shape) >= 2 {
		rd.rows, rd.cols = int(shape[len(shape)-2]), int(shape[len(shape)-1])
	}
	// 经路径选中：节点要先被登记进索引，Select 才认。
	if err := app.SelectPath("/data"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	return app
}

// TestTableFirstPage 覆盖任务 5.1：首行显示列号与首个数值。
func TestTableFirstPage(t *testing.T) {
	app := tableApp(t, []uint64{100, 50}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	// 列头是列号，行首是行号。
	if !tt.HasText("0") {
		t.Fatalf("应显示列号 0，实际 %q", tt.Texts())
	}
	if !tt.HasText("49") {
		t.Fatalf("应显示列号 49，实际 %q", tt.Texts())
	}
	// 首个单元格：行 0 列 0 → "0"；行 0 列 1 → "0.01"。
	if !tt.HasText("0.01") {
		t.Fatalf("应显示首个数据行的第二个值 0.01，实际 %q", tt.Texts())
	}
	if !tt.HasText("行 0–99 / 100") {
		t.Fatalf("页信息不正确，实际 %q", tt.Texts())
	}
}

// TestTablePaging 覆盖任务 5.2：翻页请求的 start/count 与显示内容。
func TestTablePaging(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{1000, 50}, rd)
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if len(rd.slices) != 1 {
		t.Fatalf("首屏应只读一次，实际 %d 次", len(rd.slices))
	}
	first := rd.slices[0]
	if fmt.Sprint(first.Start) != "[0 0]" || fmt.Sprint(first.Count) != "[100 50]" {
		t.Fatalf("首屏请求应为 start=[0 0] count=[100 50]，实际 %v %v", first.Start, first.Count)
	}

	if err := tt.Click("下一屏"); err != nil {
		t.Fatalf("Click 下一屏: %v", err)
	}
	tt.Frame()

	if len(rd.slices) != 2 {
		t.Fatalf("翻页应再读一次，实际共 %d 次", len(rd.slices))
	}
	second := rd.slices[1]
	if fmt.Sprint(second.Start) != "[100 0]" || fmt.Sprint(second.Count) != "[100 50]" {
		t.Fatalf("第二页请求应为 start=[100 0] count=[100 50]，实际 %v %v", second.Start, second.Count)
	}
	// 第二页首行应是第 100 行，值 100。
	if !tt.HasText("行 100–199 / 1000") {
		t.Fatalf("页信息应更新，实际 %q", tt.Texts())
	}
	if !tt.HasText("100") {
		t.Fatalf("第二页应显示行号 100，实际 %q", tt.Texts())
	}

	// 翻回上一屏不应重复读取（同一页命中缓存）。
	if err := tt.Click("上一屏"); err != nil {
		t.Fatalf("Click 上一屏: %v", err)
	}
	tt.Frame()
	if len(rd.slices) != 2 {
		t.Fatalf("回到已读页不应再读，实际共 %d 次", len(rd.slices))
	}
}

// TestTableRank1 覆盖任务 5.3：一维数据集只有一列。
func TestTableRank1(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{250}, rd)
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if len(rd.slices) != 1 {
		t.Fatalf("应读一次，实际 %d 次", len(rd.slices))
	}
	if got := fmt.Sprint(rd.slices[0].Start); got != "[0]" {
		t.Fatalf("一维请求 start 应为 [0]，实际 %s", got)
	}
	if got := fmt.Sprint(rd.slices[0].Count); got != "[100]" {
		t.Fatalf("一维请求 count 应为 [100]，实际 %s", got)
	}
	if !tt.HasText("行 0–99 / 250") {
		t.Fatalf("一维页信息不正确，实际 %q", tt.Texts())
	}
	// 一维没有列分页控件。
	if tt.HasText("右侧列") {
		t.Fatalf("一维不应出现列分页控件，实际 %q", tt.Texts())
	}
}

// TestTableRank3 覆盖任务 5.3：三维固定前导维并给出选择器。
func TestTableRank3(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{2, 3, 4}, rd)
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if !tt.HasText("固定前导维") {
		t.Fatalf("三维应出现前导维选择器，实际 %q", tt.Texts())
	}
	if len(rd.slices) != 1 {
		t.Fatalf("应读一次，实际 %d 次", len(rd.slices))
	}
	// 固定第一维为 0，读最后两维全部。
	if got := fmt.Sprint(rd.slices[0].Start); got != "[0 0 0]" {
		t.Fatalf("三维请求 start 应为 [0 0 0]，实际 %s", got)
	}
	if got := fmt.Sprint(rd.slices[0].Count); got != "[1 3 4]" {
		t.Fatalf("三维请求 count 应为 [1 3 4]，实际 %s", got)
	}

	// 切换固定的下标应重新读取，且 start 的首维变化。
	app.SetFixedDim(0, 1)
	tt.Frame()
	if len(rd.slices) != 2 {
		t.Fatalf("切换固定维应重新读取，实际共 %d 次", len(rd.slices))
	}
	if got := fmt.Sprint(rd.slices[1].Start); got != "[1 0 0]" {
		t.Fatalf("固定维 1 时 start 应为 [1 0 0]，实际 %s", got)
	}
}

// TestFormatNumber 覆盖任务 5.4：数值格式化的表驱动断言。
func TestFormatNumber(t *testing.T) {
	float64Type := hdf5.DatatypeInfo{Class: 1, Size: 8} // 浮点
	float32Type := hdf5.DatatypeInfo{Class: 1, Size: 4}
	int32Type := hdf5.DatatypeInfo{Class: 0, Size: 4, Signed: true} // 整数

	cases := []struct {
		name string
		dt   hdf5.DatatypeInfo
		v    float64
		want string
	}{
		{"float64 整数", float64Type, 42, "42"},
		{"float64 小数", float64Type, 3.5, "3.5"},
		{"float32 精度", float32Type, 0.1, "0.1"},
		{"int32 负值", int32Type, -7, "-7"},
		{"NaN", float64Type, math.NaN(), "NaN"},
		{"正无穷", float64Type, math.Inf(1), "+Inf"},
		{"负无穷", float64Type, math.Inf(-1), "-Inf"},
		{"int64 精确边界", hdf5.DatatypeInfo{Class: 0, Size: 8, Signed: true}, 9007199254740992, "9007199254740992"},
		{"int64 超 2^53", hdf5.DatatypeInfo{Class: 0, Size: 8, Signed: true}, 9007199254740994, "9007199254740994 ≈"},
		{"uint64 超 2^63", hdf5.DatatypeInfo{Class: 0, Size: 8, Signed: false}, 9223372036854775808, "9223372036854775808 ≈"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatNumber(tc.dt, tc.v); got != tc.want {
				t.Fatalf("formatNumber(%v) = %q，期望 %q", tc.v, got, tc.want)
			}
		})
	}
}

// TestFormatNumberEnum 断言枚举显示成员名而不是数值。
func TestFormatNumberEnum(t *testing.T) {
	dt := hdf5.DatatypeInfo{
		Class: 8, // 枚举
		Size:  1,
		EnumMembers: []hdf5.EnumMember{
			{Name: "周一", Value: 0},
			{Name: "周二", Value: 1},
		},
	}
	if got := formatNumber(dt, 1); got != "周二" {
		t.Fatalf("枚举应显示成员名，实际 %q", got)
	}
	if got := formatNumber(dt, 9); got != "9" {
		t.Fatalf("未知枚举值应显示数值，实际 %q", got)
	}
}

// TestTableStrings 覆盖任务 5.5：字符串按行展示并截断。
func TestTableStrings(t *testing.T) {
	rd := &fakeReader{strs: []string{"alpha", "beta", strings.Repeat("x", 400)}}
	app := tableApp(t, []uint64{3}, rd)
	app.SetMeta(&fakeMeta{info: &hdf5.DatasetInfo{
		Rank:      1,
		Shape:     []uint64{3},
		MaxShape:  []uint64{3},
		ElemCount: 3,
		Datatype:  hdf5.DatatypeInfo{Class: 3, Size: 16}, // 字符串
	}})
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if !tt.HasText("alpha") {
		t.Fatalf("应显示 alpha，实际 %q", tt.Texts())
	}
	if !tt.HasText("beta") {
		t.Fatalf("应显示 beta，实际 %q", tt.Texts())
	}
	// 超长字符串被截断，结尾是省略号。
	var truncated bool
	for _, s := range tt.Texts() {
		if strings.HasSuffix(s, "…") && strings.HasPrefix(s, "xxx") {
			truncated = true
		}
	}
	if !truncated {
		t.Fatalf("超长字符串应被截断，实际 %q", tt.Texts())
	}
}

// TestTableCompound 覆盖任务 5.5：复合数据集每个成员一列。
func TestTableCompound(t *testing.T) {
	rd := &fakeReader{comp: &hdf5.Compound{
		Members: []string{"id", "value"},
		Rows: []map[string]any{
			{"id": int32(1), "value": 1.5},
			{"id": int32(2), "value": 2.5},
		},
	}}
	app := tableApp(t, []uint64{2}, rd)
	app.SetMeta(&fakeMeta{info: &hdf5.DatasetInfo{
		Rank:      1,
		Shape:     []uint64{2},
		MaxShape:  []uint64{2},
		ElemCount: 2,
		Datatype: hdf5.DatatypeInfo{
			Class: 6, // 复合
			Size:  16,
			Members: []hdf5.CompoundMember{
				{Name: "id", Offset: 0},
				{Name: "value", Offset: 8},
			},
		},
	}})
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if !tt.HasText("id") || !tt.HasText("value") {
		t.Fatalf("列头应为成员名，实际 %q", tt.Texts())
	}
	if !tt.HasText("1.5") {
		t.Fatalf("应显示成员值 1.5，实际 %q", tt.Texts())
	}
}

// TestTableReadFailure 覆盖任务 5.6：读取失败时表格仍渲染并标注不可用。
func TestTableReadFailure(t *testing.T) {
	rd := &fakeReader{err: fmt.Errorf("损坏的分块: %w", hdf5.ErrCorrupt)}
	app := tableApp(t, []uint64{100, 10}, rd)
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if !tt.HasText(failureCell) {
		t.Fatalf("单元格应显示 %q，实际 %q", failureCell, tt.Texts())
	}
	if !tt.HasText("该区域读取失败，单元格显示为不可用：") {
		t.Fatalf("应出现失败提示，实际 %q", tt.Texts())
	}
	// 详情默认收起，点击后展开完整错误文本。
	if tt.HasText("损坏的分块") {
		t.Fatalf("详情默认应收起，实际 %q", tt.Texts())
	}
	if err := tt.Click("详情"); err != nil {
		t.Fatalf("Click 详情: %v", err)
	}
	tt.Frame()
	var found bool
	for _, s := range tt.Texts() {
		if strings.Contains(s, "损坏的分块") {
			found = true
		}
	}
	if !found {
		t.Fatalf("展开详情后应显示错误原因，实际 %q", tt.Texts())
	}
	// 翻页控件仍在，视图未被错误接管。
	if !tt.HasText("下一屏") {
		t.Fatalf("读取失败不应隐藏翻页控件，实际 %q", tt.Texts())
	}
}

// TestTableStaleResultDropped 覆盖任务 5.7：切换对象后旧结果不覆盖新视图。
func TestTableStaleResultDropped(t *testing.T) {
	gate := make(chan struct{})
	rd := &fakeReader{gate: gate}
	app := tableApp(t, []uint64{100, 10}, rd)

	// 第一次读取在后台阻塞。
	app.SetAsync(func(work func()) { go work() })
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()
	if len(rd.slices) != 1 {
		t.Fatalf("应发起一次读取，实际 %d 次", len(rd.slices))
	}

	// 换到另一个数据集，再放行旧读取。
	src := newFake()
	src.addDataset(src.root, "other")
	app.SetSource(src)
	app.SetReader(&fakeReader{})
	app.SetMeta(&fakeMeta{info: numberInfo([]uint64{5, 5})})
	if err := app.SelectPath("/other"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}

	close(gate)
	// 等后台 goroutine 把（过期的）结果写回。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && app.Busy() {
		time.Sleep(5 * time.Millisecond)
	}
	tt.Frame()

	// 过期结果被丢弃：当前对象的数据表应来自新数据集（5×5），
	// 而不是旧的 100×10。
	if app.tableErr != nil {
		t.Fatalf("新数据集读取不应失败：%v", app.tableErr)
	}
	if app.tableKey.node == (hdf5.NodeID{}) {
		t.Fatalf("新数据集应已读入")
	}
	if got := app.tableKey.node.Path; got != "/other" {
		t.Fatalf("表格结果应属于 /other，实际 %s", got)
	}
}

// TestTableLayoutReason 断言非数据集与读取描述失败时的提示。
func TestTableLayoutReason(t *testing.T) {
	src := newFake()
	src.addGroup(src.root, "g")
	src.addDataset(src.root, "bad")
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	if err := app.SelectPath("/g"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}

	if l := app.Layout(); l.OK {
		t.Fatalf("组不应可展示为表格")
	}

	if err := app.SelectPath("/bad"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	app.SetMeta(&fakeMeta{err: errors.New("描述符损坏")})
	if l := app.Layout(); l.OK || !strings.Contains(l.Reason, "描述符损坏") {
		t.Fatalf("描述失败应给出原因，实际 %+v", l)
	}
}

// TestTableHugeDataset 断言超大数据集首屏只读一页，
// 内存不随元素总数线性增长（specs/dataset-data-view 的 100000×10000 场景）。
func TestTableHugeDataset(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{100000, 10000}, rd)
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	if len(rd.slices) != 1 {
		t.Fatalf("首屏应只读一次，实际 %d 次", len(rd.slices))
	}
	got := rd.slices[0]
	if fmt.Sprint(got.Count) != "[100 50]" {
		t.Fatalf("首屏只应请求一页 100×50，实际 %v", got.Count)
	}
	total := 1
	for _, c := range got.Count {
		total *= int(c)
	}
	if total > 5000 {
		t.Fatalf("首屏读取元素数应为一页（≤5000），实际 %d", total)
	}
	if !tt.HasText("行 0–99 / 100000") {
		t.Fatalf("页信息不正确，实际 %q", tt.Texts())
	}
}

// TestTableRealFixture 用真实夹具验证表格读取。
func TestTableRealFixture(t *testing.T) {
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
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	// 夹具值 = 行 + 列/100：首行第二列是 0.01。
	if !tt.HasText("0.01") {
		t.Fatalf("应显示 0.01，实际 %q", tt.Texts())
	}
	if !tt.HasText("行 0–99 / 100") {
		t.Fatalf("页信息不正确，实际 %q", tt.Texts())
	}
}

// TestLocateCoordinates 覆盖任务 1.1：按坐标换算行列页号与固定前导维。
func TestLocateCoordinates(t *testing.T) {
	cases := []struct {
		name      string
		shape     []uint64
		coords    []uint64
		wantRow   int
		wantCol   int
		wantFixed []uint64
	}{
		{"二维中间页", []uint64{10000, 500}, []uint64{250, 137}, 2, 2, nil},
		{"二维末行末列", []uint64{10000, 500}, []uint64{9999, 499}, 99, 9, nil},
		{"三维固定前导维", []uint64{10, 1000, 500}, []uint64{3, 999, 499}, 9, 9, []uint64{3}},
		{"一维", []uint64{250}, []uint64{240}, 2, 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := tableApp(t, tc.shape, &fakeReader{})
			l := app.Layout()
			if !l.OK {
				t.Fatalf("布局应可用，实际 %+v", l)
			}
			if err := app.Locate(l, tc.coords); err != nil {
				t.Fatalf("Locate(%v): %v", tc.coords, err)
			}
			if app.Table.RowPage != tc.wantRow || app.Table.ColPage != tc.wantCol {
				t.Fatalf("页号应为 (%d,%d)，实际 (%d,%d)",
					tc.wantRow, tc.wantCol, app.Table.RowPage, app.Table.ColPage)
			}
			if fmt.Sprint(app.Table.Fixed) != fmt.Sprint(tc.wantFixed) {
				t.Fatalf("固定维应为 %v，实际 %v", tc.wantFixed, app.Table.Fixed)
			}
		})
	}
}

// TestLocateGenOnlyOnPageChange 覆盖任务 1.2：同页定位不推进代号。
func TestLocateGenOnlyOnPageChange(t *testing.T) {
	app := tableApp(t, []uint64{10000, 500}, &fakeReader{})
	l := app.Layout()

	if err := app.Locate(l, []uint64{0, 0}); err != nil {
		t.Fatalf("Locate: %v", err)
	}
	gen := app.gen
	if err := app.Locate(l, []uint64{99, 49}); err != nil {
		t.Fatalf("Locate 同页: %v", err)
	}
	if app.gen != gen {
		t.Fatalf("同页定位不应推进代号，实际 %d → %d", gen, app.gen)
	}
	if err := app.Locate(l, []uint64{100, 50}); err != nil {
		t.Fatalf("Locate 跨页: %v", err)
	}
	if app.gen == gen {
		t.Fatalf("跨页定位应推进代号，实际仍为 %d", app.gen)
	}
}

// TestLocateRejectsBadCoordinates 覆盖任务 1.3：越界坐标被拒绝且页号不变。
func TestLocateRejectsBadCoordinates(t *testing.T) {
	app := tableApp(t, []uint64{10, 20}, &fakeReader{})
	l := app.Layout()

	err := app.Locate(l, []uint64{10, 0})
	if err == nil || !strings.Contains(err.Error(), "第 1 维") {
		t.Fatalf("越界应指出维度，实际 %v", err)
	}
	if app.Table.RowPage != 0 || app.Table.ColPage != 0 {
		t.Fatalf("越界不应改变页号，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
	if err := app.Locate(l, []uint64{1}); err == nil {
		t.Fatal("坐标数量不足应报错")
	}
}

// TestParseCoords 覆盖任务 1.4：逗号、空格与空段的解析。
func TestParseCoords(t *testing.T) {
	cases := []struct {
		in      string
		rank    int
		want    string
		wantErr bool
	}{
		{"1, 2, 3", 3, "[1 2 3]", false},
		{"1 2 3", 3, "[1 2 3]", false},
		{"  1,, 2  ", 2, "[1 2]", false},
		{"1,2，3", 3, "[1 2 3]", false},
		{"", 2, "", true},
		{"1, a", 2, "", true},
		{"-1, 2", 2, "", true},
		{"1,2", 3, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseCoords(tc.in, tc.rank)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("应报错，实际得到 %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCoords: %v", err)
			}
			if fmt.Sprint(got) != tc.want {
				t.Fatalf("解析结果应为 %s，实际 %v", tc.want, got)
			}
		})
	}
}

// TestPagerSidesSplit 覆盖任务 2.1：行分页在左、列分页在右。
func TestPagerSidesSplit(t *testing.T) {
	app := tableApp(t, []uint64{1000, 120}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	rowRange, ok := tt.Find("行 0–99 / 1000")
	if !ok {
		t.Fatalf("缺少行范围，texts %q", tt.Texts())
	}
	colRange, ok := tt.Find("列 0–49 / 120")
	if !ok {
		t.Fatalf("缺少列范围，texts %q", tt.Texts())
	}
	if rowRange.X >= colRange.X {
		t.Fatalf("行分页应在左侧，行范围 x=%v 列范围 x=%v", rowRange.X, colRange.X)
	}
	rowButton, ok := tt.Find("上一屏")
	if !ok {
		t.Fatalf("缺少上一屏按钮，texts %q", tt.Texts())
	}
	colButton, ok := tt.Find("右侧列")
	if !ok {
		t.Fatalf("缺少右侧列按钮，texts %q", tt.Texts())
	}
	if rowButton.X >= colButton.X {
		t.Fatalf("行按钮应在左侧，上一屏 x=%v 右侧列 x=%v", rowButton.X, colButton.X)
	}
}

// TestPagerDirectionIsolation 覆盖任务 2.2：翻页只影响对应方向。
func TestPagerDirectionIsolation(t *testing.T) {
	app := tableApp(t, []uint64{1000, 120}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	if err := tt.Click("下一屏"); err != nil {
		t.Fatalf("Click 下一屏: %v", err)
	}
	tt.Frame()
	if app.Table.RowPage != 1 || app.Table.ColPage != 0 {
		t.Fatalf("行翻页后应为 (1,0)，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
	if err := tt.Click("右侧列"); err != nil {
		t.Fatalf("Click 右侧列: %v", err)
	}
	tt.Frame()
	if app.Table.RowPage != 1 || app.Table.ColPage != 1 {
		t.Fatalf("列翻页后应为 (1,1)，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
	if err := tt.Click("上一屏"); err != nil {
		t.Fatalf("Click 上一屏: %v", err)
	}
	tt.Frame()
	if app.Table.RowPage != 0 || app.Table.ColPage != 1 {
		t.Fatalf("行翻回后应为 (0,1)，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
}

// TestPagerHidesColumnForSingleColumn 覆盖任务 2.3：单列数据集不显示列分页。
func TestPagerHidesColumnForSingleColumn(t *testing.T) {
	app := tableApp(t, []uint64{250}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	if tt.HasText("左侧列") || tt.HasText("右侧列") {
		t.Fatalf("单列数据集不应有列分页，texts %q", tt.Texts())
	}
	if tt.HasText("列 0–49") {
		t.Fatalf("单列数据集不应有列范围，texts %q", tt.Texts())
	}
	if !tt.HasText("行 0–99 / 250") || !tt.HasText("上一屏") {
		t.Fatalf("行分页应仍可用，texts %q", tt.Texts())
	}
}

// TestLocateInputJumpsToPage 覆盖任务 3.1：输入二维坐标后跳到对应页。
func TestLocateInputJumpsToPage(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{10000, 500}, rd)
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	enter := func(label, value string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("Click %s: %v", label, err)
		}
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type(value)
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
	}
	enter("D0 输入", "250")
	enter("D1 输入", "137")

	if app.Table.RowPage != 2 || app.Table.ColPage != 2 {
		t.Fatalf("应跳到 (2,2)，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
	last := rd.slices[len(rd.slices)-1]
	if fmt.Sprint(last.Start) != "[200 100]" || fmt.Sprint(last.Count) != "[100 50]" {
		t.Fatalf("应读取 start=[200 100] count=[100 50]，实际 %v %v", last.Start, last.Count)
	}
}

// TestLocateHighRankUsesSingleInput 覆盖任务 3.2：高秩只出现一个输入框。
func TestLocateHighRankUsesSingleInput(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{2, 2, 2, 100, 50}, rd)
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	var fields int
	for _, in := range tt.AccessAll() {
		if in.Role == ui.RoleTextField {
			fields++
		}
	}
	if fields != 1 {
		t.Fatalf("高秩应只有一个定位输入框，实际 %d 个", fields)
	}

	if err := tt.Click("坐标输入"); err != nil {
		t.Fatalf("Click 坐标输入: %v", err)
	}
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("1, 1, 0, 99, 49")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if fmt.Sprint(app.Table.Fixed) != "[1 1 0]" {
		t.Fatalf("固定维应为 [1 1 0]，实际 %v", app.Table.Fixed)
	}
	last := rd.slices[len(rd.slices)-1]
	if fmt.Sprint(last.Start) != "[1 1 0 0 0]" || fmt.Sprint(last.Count) != "[1 1 1 100 50]" {
		t.Fatalf("读取范围不正确，实际 %v %v", last.Start, last.Count)
	}
}

// TestLocateErrorKeepsPage 覆盖任务 3.3：定位失败显示提示且不改页号。
func TestLocateErrorKeepsPage(t *testing.T) {
	app := tableApp(t, []uint64{1000, 120}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 1200, 700)
	tt.Frame()

	if err := tt.Click("D0 输入"); err != nil {
		t.Fatalf("Click D0 输入: %v", err)
	}
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("5000")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if app.Table.RowPage != 0 || app.Table.ColPage != 0 {
		t.Fatalf("越界不应改变页号，实际 (%d,%d)", app.Table.RowPage, app.Table.ColPage)
	}
	if !tt.HasText("定位失败") || !tt.HasText("第 1 维下标 5000 越界") {
		t.Fatalf("应显示越界提示，texts %q", tt.Texts())
	}
	if !tt.HasText("下一屏") {
		t.Fatalf("定位失败不应阻塞表格，texts %q", tt.Texts())
	}

	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("abc")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("D0 必须是整数下标") {
		t.Fatalf("应提示坐标格式错误，texts %q", tt.Texts())
	}
}

// TestPagerFitsNarrowWindow 断言秩 4 时底栏在窄窗口内不溢出。
func TestPagerFitsNarrowWindow(t *testing.T) {
	app := tableApp(t, []uint64{2, 2, 100, 120}, &fakeReader{})
	tt := ui.NewTester(tableView(app), 864, 700)
	tt.Frame()

	colButton, ok := tt.Find("右侧列")
	if !ok {
		t.Fatalf("窄窗口缺少列分页，texts %q", tt.Texts())
	}
	if colButton.X+colButton.W > 864 {
		t.Fatalf("列分页超出窗口右边界：%+v", colButton)
	}
	if _, ok := tt.Find("D3 输入"); !ok {
		t.Fatalf("秩 4 应有四个定位输入框，texts %q", tt.Texts())
	}
	if _, ok := tt.Find("跳至"); !ok {
		t.Fatalf("窄窗口缺少跳至按钮，texts %q", tt.Texts())
	}
}

// TestTableRealFixtureEnum 用真实夹具验证枚举显示成员名。
func TestTableRealFixtureEnum(t *testing.T) {
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", "types.h5"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	if err := app.SelectPath("/enum_days"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()

	// 夹具里的枚举成员名是中文，首个值 0 对应周一。
	if !tt.HasText("周一") {
		t.Fatalf("枚举应显示成员名，实际 %q", tt.Texts())
	}
}
