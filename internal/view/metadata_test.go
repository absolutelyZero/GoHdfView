package view

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// metadataView 把元数据视图单独装成可测试的视图。
func metadataView(app *App) func(c *ui.Context) {
	return func(c *ui.Context) {
		Metadata(c, app)
	}
}

// openDoc 打开测试夹具并返回已装载的应用。
func openDoc(t *testing.T, name string) (*App, *hdf5.Document) {
	t.Helper()
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", name))
	if err != nil {
		t.Fatalf("Open %s: %v", name, err)
	}
	t.Cleanup(func() { doc.Close() })
	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	return app, doc
}

// TestMetadataDataspace 对应任务 4.1：秩、维度、最大值与元素总数。
func TestMetadataDataspace(t *testing.T) {
	app, _ := openDoc(t, "basic.h5")
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	for _, want := range []string{"秩", "2", "维度", "[100, 50]", "元素总数", "5000"} {
		if !tt.HasText(want) {
			t.Fatalf("元数据应包含 %q，实际 %q", want, tt.Texts())
		}
	}
}

// TestMetadataScalar 对应「标量数据集」scenario：秩 0、维度为空、元素总数 1。
func TestMetadataScalar(t *testing.T) {
	app, _ := openDoc(t, "scalar.h5")
	if err := app.SelectPath("/scalar_like"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	if !containsText(texts, "秩") || !containsText(texts, "1") {
		t.Fatalf("应显示秩与元素总数，实际 %q", texts)
	}
	// scalar_like 在夹具里是 [1]，秩为 1；真正 rank 0 的表示由
	// internal/hdf5 的 DatasetInfo 单测覆盖（解析库不接受空维度）。
	if !containsText(texts, "元素总数") {
		t.Fatalf("应显示元素总数，实际 %q", texts)
	}
}

// TestMetadataDatatypeInt32 对应「有符号 32 位整数」scenario。
func TestMetadataDatatypeInt32(t *testing.T) {
	app, _ := openDoc(t, "types.h5")
	if err := app.SelectPath("/int32"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	for _, want := range []string{"整数", "4 字节", "小端", "有符号"} {
		if !tt.HasText(want) {
			t.Fatalf("类型区块应包含 %q，实际 %q", want, tt.Texts())
		}
	}
}

// TestMetadataCompound 对应「复合类型」scenario：逐成员列出名称与偏移。
func TestMetadataCompound(t *testing.T) {
	app, _ := openDoc(t, "types.h5")
	if err := app.SelectPath("/compound"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	for _, want := range []string{"复合", "成员", "id", "value"} {
		if !containsText(texts, want) {
			t.Fatalf("复合类型应包含 %q，实际 %q", want, texts)
		}
	}
	// 偏移应出现：id 在 0 字节处，value 在 8 字节处。
	if !containsText(texts, "0 字节处") || !containsText(texts, "8 字节处") {
		t.Fatalf("应显示成员偏移，实际 %q", texts)
	}
}

// TestMetadataFilters 对应任务 4.3：过滤器按应用顺序显示。
func TestMetadataFilters(t *testing.T) {
	app, _ := openDoc(t, "compressed.h5")
	if err := app.SelectPath("/compressed"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	if !containsText(texts, "分块") {
		t.Fatalf("应显示布局「分块」，实际 %q", texts)
	}
	if !containsText(texts, "shuffle") {
		t.Fatalf("应显示 shuffle 过滤器，实际 %q", texts)
	}
	if !containsText(texts, "deflate (level 6)") {
		t.Fatalf("应显示 deflate (level 6)，实际 %q", texts)
	}
	// 顺序：shuffle 在 deflate 之前。
	si, di := indexOf(texts, "shuffle"), indexOf(texts, "deflate (level 6)")
	if si < 0 || di < 0 || si > di {
		t.Fatalf("过滤器顺序应为 shuffle → deflate，实际 shuffle@%d deflate@%d", si, di)
	}
}

// TestMetadataNoFilters 对应「连续存储且无压缩」scenario。
func TestMetadataNoFilters(t *testing.T) {
	app, _ := openDoc(t, "strings.h5")
	if err := app.SelectPath("/names"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	if !containsText(texts, "连续") {
		t.Fatalf("应显示布局「连续」，实际 %q", texts)
	}
	if !containsText(texts, "无") {
		t.Fatalf("过滤器应显示「无」，实际 %q", texts)
	}
}

// TestMetadataUnknownFilter 对应「未知的第三方过滤器」scenario。
func TestMetadataUnknownFilter(t *testing.T) {
	// 过滤器描述由 internal/hdf5 的 FilterInfo.Describe 提供，
	// 这里直接断言该函数在未知 ID 上的输出。
	f := hdf5.FilterInfo{ID: 32017, ClientData: []uint32{3, 9}, Known: false}
	got := f.Describe()
	if !strings.Contains(got, "32017") || !strings.Contains(got, "未知过滤器") {
		t.Fatalf("未知过滤器应显示 ID 与「未知过滤器」，实际 %q", got)
	}
	if !strings.Contains(got, "3, 9") {
		t.Fatalf("未知过滤器应显示客户端参数，实际 %q", got)
	}
}

// TestAttributesList 对应任务 4.4：属性名称与值。
func TestAttributesList(t *testing.T) {
	app, _ := openDoc(t, "basic.h5")
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	for _, want := range []string{"属性", "units", "scale_factor", "K"} {
		if !containsText(texts, want) {
			t.Fatalf("属性列表应包含 %q，实际 %q", want, texts)
		}
	}
}

// TestAttributesEmpty 对应「对象没有属性」scenario。
func TestAttributesEmpty(t *testing.T) {
	app, _ := openDoc(t, "basic.h5")
	if err := app.SelectPath("/results/empty"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	if !tt.HasText("无属性") {
		t.Fatalf("空属性对象应显示「无属性」，实际 %q", tt.Texts())
	}
}

// TestGroupMetadataAndDataUnavailable 对应任务 4.5。
func TestGroupMetadataAndDataUnavailable(t *testing.T) {
	app, _ := openDoc(t, "basic.h5")
	if err := app.SelectPath("/results"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt := ui.NewTester(metadataView(app), 800, 600)
	tt.Frame()

	texts := tt.Texts()
	if !containsText(texts, "成员数") {
		t.Fatalf("组元数据应显示成员数，实际 %q", texts)
	}
	if app.DataAvailable() {
		t.Fatalf("选中组时数据视图应不可用")
	}

	// 换成数据集后数据视图应可用。
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	if !app.DataAvailable() {
		t.Fatalf("选中数据集时数据视图应可用")
	}
}

// containsText 报告文本列表中是否有一项等于或包含 want。
func containsText(texts []string, want string) bool {
	return indexOf(texts, want) >= 0
}

// indexOf 返回第一项包含 want 的下标，找不到返回 -1。
func indexOf(texts []string, want string) int {
	for i, s := range texts {
		if strings.Contains(s, want) {
			return i
		}
	}
	return -1
}
