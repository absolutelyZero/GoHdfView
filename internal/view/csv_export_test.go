package view

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// csvFixedReader 按固定值返回切片，用于精确断言格式化结果。
type csvFixedReader struct {
	value float64
	calls []sliceCall
}

func (r *csvFixedReader) ReadSlice(_ *hdf5.Node, start, count []uint64) (*hdf5.Slice, error) {
	r.calls = append(r.calls, sliceCall{Start: append([]uint64(nil), start...), Count: append([]uint64(nil), count...)})
	total := 1
	for _, c := range count {
		total *= int(c)
	}
	vals := make([]float64, total)
	for i := range vals {
		vals[i] = r.value
	}
	return &hdf5.Slice{Start: start, Count: count, Values: vals}, nil
}

func (r *csvFixedReader) ReadStrings(_ *hdf5.Node) (*hdf5.Strings, error)   { return nil, nil }
func (r *csvFixedReader) ReadCompound(_ *hdf5.Node) (*hdf5.Compound, error) { return nil, nil }
func (r *csvFixedReader) ReadStrided(_ *hdf5.Node, _, _, _, _, _, _ uint64) (*hdf5.Slice, error) {
	return nil, fmt.Errorf("not used")
}

func intInfo(shape []uint64) *hdf5.DatasetInfo {
	total := uint64(1)
	for _, d := range shape {
		total *= d
	}
	return &hdf5.DatasetInfo{Rank: len(shape), Shape: shape, MaxShape: append([]uint64(nil), shape...), ElemCount: total,
		Datatype: hdf5.DatatypeInfo{Class: 0, Size: 8, Signed: true}}
}

// TestCSVWriteCurrentPage 覆盖任务 1.1：当前页 CSV 含列号表头、行号与表格一致的格式化文本。
func TestCSVWriteCurrentPage(t *testing.T) {
	rd := &csvFixedReader{value: 9007199254740994} // 超过 2^53，应带 ≈
	app := tableApp(t, []uint64{2, 3}, &fakeReader{})
	app.SetReader(rd)
	app.SetMeta(&fakeMeta{info: intInfo([]uint64{2, 3})})
	l := app.Layout()
	write := app.csvWrite(l, CSVScopePage)
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatalf("写 CSV: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "#,0,1,2\n") {
		t.Fatalf("列号表头应为 #,0,1,2，实际 %q", got)
	}
	wantCell := formatNumber(hdf5.DatatypeInfo{Class: 0, Size: 8, Signed: true}, 9007199254740994)
	if !strings.Contains(wantCell, "≈") {
		t.Fatalf("测试前提：formatNumber 应给大整数加 ≈，实际 %q", wantCell)
	}
	for _, row := range []string{"0", "1"} {
		if !strings.Contains(got, row+","+wantCell+","+wantCell+","+wantCell) {
			t.Errorf("行 %s 应含行号与一致格式化值，实际 %q", row, got)
		}
	}
}

// TestCSVExporterInjection 覆盖任务 1.2：注入的回调在点击导出入口时被调用一次。
func TestCSVExporterInjection(t *testing.T) {
	app := tableApp(t, []uint64{10, 5}, &fakeReader{})
	calls := 0
	app.SetCSVExporter(func(CSVExportScope, CSVWriteFunc) { calls++ })
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()
	if err := tt.Click("导出 CSV"); err != nil {
		t.Fatalf("Click 导出 CSV: %v", err)
	}
	if err := tt.Click("当前页"); err != nil {
		t.Fatalf("Click 当前页: %v", err)
	}
	if calls != 1 {
		t.Fatalf("导出回调应调用一次，实际 %d", calls)
	}
}

// TestCSVExportEntryVisibility 覆盖任务 2.1：仅数值表显示导出入口。
func TestCSVExportEntryVisibility(t *testing.T) {
	app := tableApp(t, []uint64{10, 5}, &fakeReader{})
	app.SetCSVExporter(func(CSVExportScope, CSVWriteFunc) {})
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()
	if !tt.HasText("导出 CSV") {
		t.Fatalf("数值表应显示导出入口，texts %q", tt.Texts())
	}
	if err := tt.Click("导出 CSV"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	for _, label := range []string{"当前页", "整个数据集"} {
		if !tt.HasText(label) {
			t.Errorf("菜单应含 %q，texts %q", label, tt.Texts())
		}
	}

	// 换成字符串表后入口消失。
	strApp := tableApp(t, []uint64{3}, &fakeReader{strs: []string{"a", "b", "c"}})
	strApp.SetCSVExporter(func(CSVExportScope, CSVWriteFunc) {})
	strApp.SetMeta(&fakeMeta{info: &hdf5.DatasetInfo{Rank: 1, Shape: []uint64{3}, ElemCount: 3,
		Datatype: hdf5.DatatypeInfo{Class: 3, Size: 16}}})
	tt2 := ui.NewTester(tableView(strApp), 900, 600)
	tt2.Frame()
	if tt2.HasText("导出 CSV") {
		t.Fatalf("字符串表不应显示导出入口，texts %q", tt2.Texts())
	}
}

// TestCSVExportCurrentPageClick 覆盖任务 2.2：点击当前页后假回调收到正确范围与内容。
func TestCSVExportCurrentPageClick(t *testing.T) {
	app := tableApp(t, []uint64{3, 4}, &fakeReader{})
	var gotScope CSVExportScope = -1
	var gotCSV string
	app.SetCSVExporter(func(scope CSVExportScope, write CSVWriteFunc) {
		gotScope = scope
		var buf bytes.Buffer
		if err := write(&buf); err == nil {
			gotCSV = buf.String()
		}
	})
	tt := ui.NewTester(tableView(app), 900, 600)
	tt.Frame()
	if err := tt.Click("导出 CSV"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("当前页"); err != nil {
		t.Fatal(err)
	}
	if gotScope != CSVScopePage {
		t.Fatalf("范围应为当前页，实际 %v", gotScope)
	}
	if !strings.HasPrefix(gotCSV, "#,0,1,2,3\n") {
		t.Fatalf("CSV 应含当前页列号 0–3，实际 %q", gotCSV)
	}
	if strings.Contains(gotCSV, "\n4,") {
		t.Fatalf("不应含第 4 行（数据集仅 3 行），实际 %q", gotCSV)
	}
}

// TestCSVWriteAllChunked 覆盖任务 3.1：120×110 数据集分块为 2×3，请求 start/count 正确。
func TestCSVWriteAllChunked(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{120, 110}, rd)
	l := app.Layout()
	write := app.csvWrite(l, CSVScopeAll)
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatalf("写 CSV: %v", err)
	}
	if len(rd.slices) != 6 {
		t.Fatalf("应分 2×3=6 块读取，实际 %d 次：%v", len(rd.slices), rd.slices)
	}
	want := []struct{ start, count string }{
		{"[0 0]", "[100 50]"}, {"[0 50]", "[100 50]"}, {"[0 100]", "[100 10]"},
		{"[100 0]", "[20 50]"}, {"[100 50]", "[20 50]"}, {"[100 100]", "[20 10]"},
	}
	for i, w := range want {
		got := rd.slices[i]
		if fmt.Sprint(got.Start) != w.start || fmt.Sprint(got.Count) != w.count {
			t.Errorf("块 %d 应为 start=%s count=%s，实际 %v %v", i, w.start, w.count, got.Start, got.Count)
		}
	}
	if got := strings.Count(buf.String(), "\n"); got != 121 {
		t.Errorf("应有表头 + 120 行 = 121 行，实际 %d", got)
	}
}

// TestCSVExportHugeDatasetBounded 覆盖任务 3.2：超大形状每次请求不超过一页。
func TestCSVExportHugeDatasetBounded(t *testing.T) {
	rd := &fakeReader{}
	app := tableApp(t, []uint64{300, 120}, rd)
	write := app.csvWrite(app.Layout(), CSVScopeAll)
	if err := write(&bytes.Buffer{}); err != nil {
		t.Fatalf("写 CSV: %v", err)
	}
	for i, c := range rd.slices {
		rows, cols := int(c.Count[0]), int(c.Count[1])
		if rows > defaultPageRows || cols > defaultPageCols {
			t.Errorf("块 %d 请求 %dx%d 超过一页 %dx%d", i, rows, cols, defaultPageRows, defaultPageCols)
		}
	}
	// 3 行块 × 3 列块 = 9 次请求，而非一次性载入 36000 个元素。
	if len(rd.slices) != 9 {
		t.Fatalf("应分 3×3=9 块读取，实际 %d", len(rd.slices))
	}
}

// TestCSVWriteFailure 覆盖任务 4.2：读取失败时写入函数返回错误，不产出完整内容。
func TestCSVWriteFailure(t *testing.T) {
	app := tableApp(t, []uint64{10, 5}, &fakeReader{err: fmt.Errorf("损坏的分块")})
	write := app.csvWrite(app.Layout(), CSVScopePage)
	var buf bytes.Buffer
	if err := write(&buf); err == nil {
		t.Fatal("读取失败时应返回错误")
	}
	if buf.Len() != 0 {
		t.Fatalf("失败时不应写出内容，实际 %q", buf.String())
	}
}
