package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
	"gohdfview/internal/view"
)

// fixture 返回测试夹具的绝对路径。
func fixture(name string) string {
	return filepath.Join("internal", "hdf5", "testdata", name)
}

// newTester 创建一个以空状态启动的测试器。
func newTester(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := newApp()
	tt := ui.NewTester(a.view, 1200, 800)
	t.Cleanup(func() {
		if doc := a.state.Document(); doc != nil {
			doc.Close()
		}
	})
	return a, tt
}

// openFixture 打开夹具并刷新一帧。
func openFixture(t *testing.T, a *app, tt *ui.Tester, name string) {
	t.Helper()
	a.openFile(fixture(name))
	if a.errMsg != "" {
		t.Fatalf("打开 %s 失败：%s", name, a.errMsg)
	}
	tt.Frame()
}

// 空状态：没有文件时显示提示与打开按钮（viewer-shell）。
func TestEmptyState(t *testing.T) {
	_, tt := newTester(t)
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Errorf("启动后缺少空状态提示，texts %q", tt.Texts())
	}
	if !tt.HasText("GoHdfView") {
		t.Errorf("启动后缺少标题，texts %q", tt.Texts())
	}
}

// 打开文件后出现分栏：左树、右工作区与状态栏（任务 7.1）。
func TestOpenShowsSplitAndStatus(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if !a.state.HasDocument() {
		t.Fatal("打开后 HasDocument 应为 true")
	}
	for _, want := range []string{"对象", "matrix", "results", "basic.h5"} {
		if !tt.HasText(want) {
			t.Errorf("缺少 %q，texts %q", want, tt.Texts())
		}
	}

	// 选中数据集后状态栏给出元素总数。
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatalf("选中 /matrix 失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("共 5000 个元素") {
		t.Errorf("状态栏缺少元素总数，texts %q", tt.Texts())
	}
}

// TestTopBarAvoidsWindowButtons 断言隐藏标题栏时顶栏为窗口按钮留出空间。
func TestTopBarAvoidsWindowButtons(t *testing.T) {
	a, tt := newTester(t)
	tt.SetTitleBar(ui.TitleBar{Height: 46, Right: 138})
	openFixture(t, a, tt, "basic.h5")

	open, ok := tt.Find("打开")
	if !ok {
		t.Fatalf("顶栏缺少打开按钮，texts %q", tt.Texts())
	}
	// 1200 宽窗口右侧 138 属于系统窗口按钮，打开按钮必须在其左侧。
	if open.X+open.W > 1200-138 {
		t.Fatalf("打开按钮侵入了窗口按钮区域：%+v", open)
	}
	mark, ok := tt.Find("GoHdfView")
	if !ok {
		t.Fatalf("顶栏缺少标题，texts %q", tt.Texts())
	}
	if mark.Y+mark.H > 46 {
		t.Fatalf("顶栏标题应位于 46px 内：%+v", mark)
	}
}

// 选中组时「数据」标签置灰并自动落到「元数据」（viewer-shell 视图切换）。
func TestGroupDisablesDataTab(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if err := a.state.SelectPath("/results"); err != nil {
		t.Fatalf("选中 /results 失败：%v", err)
	}
	a.tab = tabData
	tt.Frame()

	if a.tab != tabMetadata {
		t.Errorf("选中组后应落到元数据标签，实际 %d", a.tab)
	}
	if a.state.DataAvailable() {
		t.Error("组不应有数据视图")
	}
	if !tt.HasText("元数据") || !tt.HasText("组信息") {
		t.Errorf("元数据视图内容缺失，texts %q", tt.Texts())
	}
}

// 数据集三个标签都能切换（任务 7.1）。
func TestTabsSwitch(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatalf("选中 /matrix 失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("数据") || !tt.HasText("元数据") || !tt.HasText("属性") {
		t.Fatalf("三个标签应同时出现，texts %q", tt.Texts())
	}

	// 切到元数据后仍应看到数据类型区块。
	if err := tt.Click("元数据"); err != nil {
		t.Fatalf("点击元数据失败：%v", err)
	}
	tt.Frame()
	if a.tab != tabMetadata {
		t.Errorf("点击后 tab 应为元数据，实际 %d", a.tab)
	}
	if !tt.HasText("数据类型") {
		t.Errorf("元数据视图缺少数据类型，texts %q", tt.Texts())
	}

	// 切到属性后应看到 units。
	if err := tt.Click("属性"); err != nil {
		t.Fatalf("点击属性失败：%v", err)
	}
	tt.Frame()
	if a.tab != tabAttributes {
		t.Errorf("点击后 tab 应为属性，实际 %d", a.tab)
	}
	if !tt.HasText("units") {
		t.Errorf("属性视图缺少 units，texts %q", tt.Texts())
	}

	// 回到数据。
	if err := tt.Click("数据"); err != nil {
		t.Fatalf("点击数据失败：%v", err)
	}
	tt.Frame()
	if a.tab != tabData {
		t.Errorf("点击后 tab 应为数据，实际 %d", a.tab)
	}
}

// 切换对象后保持当前视图（viewer-shell 视图切换）。
func TestTabSurvivesSelection(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if err := tt.Click("元数据"); err != nil {
		t.Fatalf("点击元数据失败：%v", err)
	}
	tt.Frame()

	if err := a.state.SelectPath("/results/run-1/temperature"); err != nil {
		t.Fatalf("选中温度数据集失败：%v", err)
	}
	tt.Frame()

	if a.tab != tabMetadata {
		t.Errorf("切换对象后应保持元数据标签，实际 %d", a.tab)
	}
}

// 打开失败保留原有文档并显示可关闭的提示（任务 7.5）。
func TestOpenFailureKeepsDocument(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")
	before := a.state.Document()

	a.openFile(fixture("not_hdf5.txt"))
	tt.Frame()

	if a.state.Document() != before {
		t.Error("打开失败后应保留原文档")
	}
	if !strings.Contains(a.errMsg, "HDF5") {
		t.Errorf("提示应说明不是 HDF5 文件，实际 %q", a.errMsg)
	}
	if !tt.HasText("关闭提示") {
		t.Errorf("提示条应有可关闭按钮，texts %q", tt.Texts())
	}

	// 关闭提示后仍可继续浏览。
	if err := tt.Click("关闭提示"); err != nil {
		t.Fatalf("点击关闭提示失败：%v", err)
	}
	tt.Frame()
	if a.errMsg != "" {
		t.Errorf("关闭后提示应为空，实际 %q", a.errMsg)
	}
	if !tt.HasText("matrix") {
		t.Errorf("关闭提示后树应仍可用，texts %q", tt.Texts())
	}
}

// 错误提示不阻塞浏览：读取失败后仍能选中其他数据集（任务 7.5）。
func TestErrorDoesNotBlockBrowsing(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	a.setError(hdf5.ErrCorrupt)
	tt.Frame()
	if !tt.HasText("关闭提示") {
		t.Fatalf("应有提示条，texts %q", tt.Texts())
	}

	// 提示条存在时仍能切换对象。
	if err := a.state.SelectPath("/results"); err != nil {
		t.Fatalf("有提示时仍应能选中 /results：%v", err)
	}
	tt.Frame()
	if got := a.state.Current; got == nil || got.Path != "/results" {
		t.Errorf("选中应生效，实际 %+v", got)
	}
}

// 关闭文件回到空状态并释放句柄（任务 7.6）。
func TestCloseFileReleasesHandle(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")
	doc := a.state.Document()

	root := doc.Root()
	a.closeFile()
	tt.Frame()

	if a.state.HasDocument() {
		t.Error("关闭后不应有文档")
	}
	if doc.Root() != nil {
		t.Error("关闭后 Root 应为 nil")
	}
	if _, err := root.Children(); err == nil {
		t.Error("关闭后读取应失败")
	}
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Errorf("关闭后应回空状态，texts %q", tt.Texts())
	}

	// 可以重新打开。
	openFixture(t, a, tt, "compressed.h5")
	if !tt.HasText("compressed.h5") {
		t.Errorf("重新打开后应显示文件名，texts %q", tt.Texts())
	}
}

// 关闭时释放句柄：退出后文件不再被占用（任务 7.6）。
func TestCloseDocumentOnExit(t *testing.T) {
	a := newApp()
	a.openFile(fixture("basic.h5"))
	doc := a.state.Document()
	if doc == nil {
		t.Fatal("应已打开文档")
	}
	root := doc.Root()
	doc.Close()
	if _, err := root.Children(); err == nil {
		t.Error("关闭后读取应失败")
	}
}

// 图像视图在数据标签内可切换（dataset-data-view 的图像 scenario）。
func TestDataModeImage(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatalf("选中 /matrix 失败：%v", err)
	}
	// 打开文件时选中的是根组，「数据」标签被置灰并落到元数据；
	// 选中数据集后要显式点回「数据」。
	if err := tt.Click("数据"); err != nil {
		t.Fatalf("点击数据失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("表格") || !tt.HasText("图像") {
		t.Fatalf("数据标签内应有表格/图像切换，texts %q", tt.Texts())
	}

	if err := tt.Click("图像"); err != nil {
		t.Fatalf("点击图像失败：%v", err)
	}
	tt.Frame()
	if a.dataMode != modeImage {
		t.Errorf("应切到图像模式，实际 %d", a.dataMode)
	}
	if !tt.HasText("100 × 50") {
		t.Errorf("图像应显示尺寸，texts %q", tt.Texts())
	}
}

// 图像视图对字符串数据集说明不可用（dataset-data-view）。
func TestImageUnavailableForStrings(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "strings.h5")

	if err := a.state.SelectPath("/names"); err != nil {
		t.Fatalf("选中 /names 失败：%v", err)
	}
	a.tab = tabData
	a.dataMode = modeImage
	tt.Frame()

	if !tt.HasText("图像视图不可用") {
		t.Errorf("字符串数据集应说明图像不可用，texts %q", tt.Texts())
	}
}

// 状态栏显示当前对象路径（任务 7.1）。
func TestStatusBarPath(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	if err := a.state.SelectPath("/results/run-1/temperature"); err != nil {
		t.Fatalf("选中失败：%v", err)
	}
	tt.Frame()

	if !tt.HasText("/results/run-1/temperature") {
		t.Errorf("状态栏应显示完整路径，texts %q", tt.Texts())
	}
	if !tt.HasText("共 10 个元素") {
		t.Errorf("状态栏应显示元素总数，texts %q", tt.Texts())
	}
}

// 菜单构建不 panic 且包含三个顶级菜单（任务 7.2）。
func TestMenuBuilds(t *testing.T) {
	a := newApp()
	m := a.menu()
	if m == nil {
		t.Fatal("菜单不应为空")
	}
	items := m.Items()
	if len(items) != 3 {
		t.Fatalf("应有三个顶级菜单，实际 %d", len(items))
	}
	labels := []string{items[0].Label, items[1].Label, items[2].Label}
	want := []string{"文件", "视图", "帮助"}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("第 %d 个菜单应为 %q，实际 %q", i, want[i], labels[i])
		}
	}
}

// 菜单里的「全部展开」与视图按钮行为一致（任务 7.2）。
func TestMenuExpandAll(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")

	file := a.menu().Items()[1]
	var expand *mygo.MenuItem
	for _, it := range file.Submenu {
		if it.Label == "全部展开" {
			expand = it
		}
	}
	if expand == nil {
		t.Fatal("视图菜单里应有「全部展开」")
	}
	if expand.Accelerator != "CmdOrCtrl+Shift+E" {
		t.Errorf("全部展开的快捷键应为 CmdOrCtrl+Shift+E，实际 %q", expand.Accelerator)
	}
	expand.Click(expand, nil)
	tt.Frame()

	if !tt.HasText("temperature") || !tt.HasText("empty") {
		t.Errorf("全部展开后应看到嵌套成员，texts %q", tt.Texts())
	}
}

// 格式化的文件大小便于阅读（状态栏）。
func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 << 20, "5.0 MB"},
	}
	for _, c := range cases {
		if got := formatSize(c.in); got != c.want {
			t.Errorf("formatSize(%d) = %q，want %q", c.in, got, c.want)
		}
	}
}

// TestWriteCSVFile 覆盖任务 4.1/4.2：成功时原子写入目标文件，失败时清理临时文件。
func TestWriteCSVFile(t *testing.T) {
	t.Run("成功写入", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.csv")
		err := writeCSVFile(path, func(w io.Writer) error {
			_, e := w.Write([]byte("#,0\n0,1\n"))
			return e
		})
		if err != nil {
			t.Fatalf("writeCSVFile: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读回 CSV: %v", err)
		}
		if string(got) != "#,0\n0,1\n" {
			t.Fatalf("CSV 内容不符：%q", got)
		}
	})

	t.Run("写入失败清理临时文件", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.csv")
		err := writeCSVFile(path, func(io.Writer) error { return errors.New("boom") })
		if err == nil {
			t.Fatal("应返回写入错误")
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("失败时不应产生目标文件，stat=%v", statErr)
		}
		if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
			t.Fatalf("失败时应清理临时文件，stat=%v", statErr)
		}
	})
}

// TestExportSuccessBanner 覆盖任务 5.1/5.2：成功导出后显示含路径的非模态提示与快捷打开按钮。
func TestExportSuccessBanner(t *testing.T) {
	a, tt := newTester(t)
	openFixture(t, a, tt, "basic.h5")
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	a.tab = tabData
	tt.Frame()

	// 注入 Shell 处理器并模拟导出成功回写状态（无窗口时同步执行）。
	var opened, revealed string
	a.openExportedFile = func(p string) error { opened = p; return nil }
	a.revealExportedFile = func(p string) { revealed = p }
	a.reportExportDone("data.csv")
	tt.Frame()
	if !tt.HasText("已导出") || !tt.HasText("data.csv") {
		t.Fatalf("应显示导出成功提示与路径，texts %q", tt.Texts())
	}
	if a.recentExport.path != "data.csv" || a.recentExport.at.IsZero() {
		t.Fatalf("最近导出状态不正确：%+v", a.recentExport)
	}

	if err := tt.Click("打开文件"); err != nil {
		t.Fatalf("Click 打开文件: %v", err)
	}
	if err := tt.Click("打开文件夹"); err != nil {
		t.Fatalf("Click 打开文件夹: %v", err)
	}
	if opened != "data.csv" || revealed != "data.csv" {
		t.Fatalf("快捷打开未触发：opened=%q revealed=%q", opened, revealed)
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("Click 关闭: %v", err)
	}
	tt.Frame()
	if tt.HasText("已导出") {
		t.Fatalf("关闭后提示条应消失，texts %q", tt.Texts())
	}
}

// TestOpenMultipleFiles 覆盖任务 2.1/3.1：两个文件作为树根，选中节点切换右侧数据。
func TestOpenMultipleFiles(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	a.openFile(fixture("compressed.h5"))
	tt.Frame()

	if len(a.files) != 2 {
		t.Fatalf("应打开两个文件，实际 %d", len(a.files))
	}
	if a.currentFile() != absFixture(t, "compressed.h5") {
		t.Fatalf("当前文件应为第二个文件，实际 %q", a.currentFile())
	}
	if !tt.HasText("basic.h5") || !tt.HasText("compressed.h5") {
		t.Fatalf("应显示两个文件根节点，texts %q", tt.Texts())
	}

	// 选中第一个文件的根节点：状态栏与标题跟随。
	basicFile := absFixture(t, "basic.h5")
	n := a.files[0].doc.Root()
	a.state.Select(n.ID)
	tt.Frame()
	if a.currentFile() != basicFile {
		t.Fatalf("选中后当前文件应为 basic.h5，实际 %q", a.currentFile())
	}
	if !tt.HasText("basic.h5") {
		t.Fatalf("状态栏应显示 basic.h5，texts %q", tt.Texts())
	}

	// 重复打开同一路径：不新增句柄，定位到已有根节点。
	before := len(a.files)
	a.openFile(fixture("basic.h5"))
	tt.Frame()
	if len(a.files) != before || a.currentFile() != basicFile {
		t.Fatalf("重复打开不应新增文件，files=%d current=%q", len(a.files), a.currentFile())
	}
}

// TestDocumentStateIsolation 覆盖任务 4.1：视图状态按文件隔离，切走再切回原样恢复。
func TestDocumentStateIsolation(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	a.tab = tabData
	a.dataMode = modeImage // 文件 A：图像模式
	tt.Frame()             // 登记文件 A 为当前文件

	a.openFile(fixture("compressed.h5"))
	if err := a.state.SelectPath("/compressed"); err != nil {
		t.Fatal(err)
	}
	a.tab, a.dataMode = tabData, modeTable
	tt.Frame() // view() 保存文件 A 状态并恢复文件 B 默认状态
	if a.dataMode != modeTable {
		t.Fatalf("新文件应为默认模式，实际 mode=%d", a.dataMode)
	}

	// 切回文件 A 的根节点：图像模式与选中对象恢复。
	a.state.Select(a.files[0].doc.Root().ID)
	tt.Frame()
	if a.dataMode != modeImage {
		t.Fatalf("文件 A 应保持图像模式，实际 %d", a.dataMode)
	}
	if a.state.Current == nil || a.state.Current.ID.File != absFixture(t, "basic.h5") {
		t.Fatalf("文件 A 的选中对象应属于 basic.h5，实际 %+v", a.state.Current)
	}
}

// TestCloseDocument 覆盖任务 2.2：关闭选中文件释放句柄并从树移除，最后回到空状态。
func TestCloseDocument(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	a.openFile(fixture("compressed.h5"))
	a.openFile(fixture("types.h5"))
	if len(a.files) != 3 {
		t.Fatalf("应有 3 个文件，实际 %d", len(a.files))
	}

	// 关闭当前选中的第三个文件。
	current := a.currentFile()
	doc := a.state.Document()
	root := doc.Root()
	a.closeFile()
	tt.Frame()
	if len(a.files) != 2 || a.currentFile() == current {
		t.Fatalf("关闭后应剩 2 个文件且切换选中，files=%d current=%q", len(a.files), a.currentFile())
	}
	if _, err := root.Children(); err == nil {
		t.Error("关闭后句柄应释放")
	}

	// 清空其余文件。
	for len(a.files) > 0 {
		a.closeFile()
		tt.Frame()
	}
	if len(a.files) != 0 || a.state.HasDocument() {
		t.Fatalf("关闭全部后应为空状态，files=%d", len(a.files))
	}
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Fatalf("应回到空状态提示")
	}
}

// absFixture 返回夹具绝对路径，供去重断言使用。
func absFixture(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(fixture(name))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestTreeRootSelectionSwitches 覆盖任务 3.1：选中不同文件的树节点即切换数据来源。
func TestTreeRootSelectionSwitches(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	a.openFile(fixture("compressed.h5"))
	tt.Frame()

	if a.currentFile() != absFixture(t, "compressed.h5") {
		t.Fatalf("当前应为 compressed.h5，实际 %q", a.currentFile())
	}
	// 选中 basic.h5 根节点：右侧标题与状态栏跟随该文件。
	a.state.Select(a.files[0].doc.Root().ID)
	tt.Frame()
	if a.currentFile() != absFixture(t, "basic.h5") {
		t.Fatalf("选中 basic 树根应切换文件，current=%q", a.currentFile())
	}
	if !tt.HasText("basic.h5") {
		t.Fatalf("状态栏应显示 basic.h5，texts %q", tt.Texts())
	}
	// 无标签栏残留。
	if tt.HasText("关闭 basic.h5") {
		t.Fatalf("不应再显示文档标签关闭按钮，texts %q", tt.Texts())
	}
}

// failingReader 让某个文件的所有读取都返回错误。
type failingReader struct{ err error }

func (f failingReader) ReadSlice(*hdf5.Node, []uint64, []uint64) (*hdf5.Slice, error) {
	return nil, f.err
}
func (f failingReader) ReadStrings(*hdf5.Node) (*hdf5.Strings, error) {
	return nil, f.err
}
func (f failingReader) ReadCompound(*hdf5.Node) (*hdf5.Compound, error) {
	return nil, f.err
}
func (f failingReader) ReadStrided(*hdf5.Node, uint64, uint64, uint64, uint64, uint64, uint64) (*hdf5.Slice, error) {
	return nil, f.err
}

// TestReadFailureIsolation 覆盖任务 4.2：某文件读取失败显示提示后仍可切换其它文件浏览。
func TestReadFailureIsolation(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	a.openFile(fixture("compressed.h5"))
	tt.Frame()

	// 让 basic.h5 的读取失败。
	basic := absFixture(t, "basic.h5")
	a.state.AddFileReader(basic, failingReader{err: errors.New("读取失败")})
	a.state.Select(a.files[0].doc.Root().ID)
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	a.tab = tabData
	tt.Frame()
	if !tt.HasText("读取失败") {
		t.Fatalf("应显示读取失败提示，texts %q", tt.Texts())
	}

	// 切到 compressed.h5：提示仍可见，但另一个文件能正常读取。
	a.state.Select(a.files[1].doc.Root().ID)
	if err := a.state.SelectPath("/compressed"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("compressed.h5") {
		t.Fatalf("应能切换并浏览第二个文件，texts %q", tt.Texts())
	}
	if a.state.Err != nil && a.state.Err.Error() == "读取失败" {
		t.Fatalf("切到其它文件后不应再显示前一个文件的读取错误")
	}
}

// fakeWindowHandle 记录独立窗口的生命周期事件。
type fakeWindowHandle struct {
	title        string
	shown        bool
	closed       bool
	invalidates  int
	buttonsTheme mygo.ThemeSource
	closeFn      func(*mygo.CloseEvent)
}

func (f *fakeWindowHandle) Show()                                    { f.shown = true }
func (f *fakeWindowHandle) Close()                                   { f.closed = true }
func (f *fakeWindowHandle) Invalidate()                              { f.invalidates++ }
func (f *fakeWindowHandle) SetTitle(t string)                        { f.title = t }
func (f *fakeWindowHandle) SetWindowButtonsTheme(s mygo.ThemeSource) { f.buttonsTheme = s }
func (f *fakeWindowHandle) OnClose(fn func(*mygo.CloseEvent)) func() {
	f.closeFn = fn
	return func() {}
}

// TestDetachTableWindow 覆盖任务 1.2/2.1/2.2/3.2：假工厂验证创建、
// 标题、内容固定与关闭清理。
func TestDetachTableWindow(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	var handles []*fakeWindowHandle
	a.newTableWindow = func(opts mygo.WindowOptions) windowHandle {
		h := &fakeWindowHandle{title: opts.Title}
		handles = append(handles, h)
		return h
	}

	a.openTableInWindow()
	if len(handles) != 1 || len(a.windows) != 1 {
		t.Fatalf("应创建一个独立窗口，handles=%d windows=%d", len(handles), len(a.windows))
	}
	if !handles[0].shown || !strings.Contains(handles[0].title, "matrix") || !strings.Contains(handles[0].title, "basic.h5") {
		t.Fatalf("窗口应显示且标题含数据集与文件：%+v", handles[0])
	}

	// 内容固定：主窗口切换选中后，独立窗口的 App 仍指向打开时的数据集。
	detached := a.windows[0].state
	before := detached.Current
	if before == nil || before.Path != "/matrix" {
		t.Fatalf("独立窗口应定位到 /matrix，实际 %+v", before)
	}
	if err := a.state.SelectPath("/results"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if detached.Current != before || detached.Current.Path != "/matrix" {
		t.Fatalf("独立窗口内容不应随主窗口变化：%+v", detached.Current)
	}

	// 关闭：集合减少、窗口被 Close，但主窗口文档仍打开。
	a.closeTableWindow(a.windows[0])
	if len(a.windows) != 0 || !handles[0].closed {
		t.Fatalf("关闭后 windows=%d closed=%v", len(a.windows), handles[0].closed)
	}
	if !a.state.HasDocument() {
		t.Fatal("关闭独立窗口不应关闭主窗口文档")
	}
}

// TestDetachedWindowUsesDarkTheme 覆盖任务 5.1：独立窗口内容视图应用深色主题与背景。
func TestDetachedWindowUsesDarkTheme(t *testing.T) {
	st := view.NewApp()
	var bg ui.Color
	tt := ui.NewTester(func(c *ui.Context) {
		detachedTableView(st, "test")(c)
		bg = c.Theme().Background
	}, 800, 600)
	tt.Frame()
	if bg.R != view.ColBg.R || bg.G != view.ColBg.G || bg.B != view.ColBg.B || bg.A != view.ColBg.A {
		t.Fatalf("独立窗口应使用深色主题背景 %+v，实际 %+v", view.ColBg, bg)
	}
}

// TestDetachedWindowHiddenTitleBar 覆盖独立窗口标题栏样式：隐藏原生标题栏、深色窗口按钮。
func TestDetachedWindowHiddenTitleBar(t *testing.T) {
	a, _ := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	var got mygo.WindowOptions
	var handle *fakeWindowHandle
	a.newTableWindow = func(opts mygo.WindowOptions) windowHandle {
		got = opts
		handle = &fakeWindowHandle{title: opts.Title}
		return handle
	}
	a.openTableInWindow()
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		if got.TitleBarStyle != mygo.TitleBarHidden || got.TitleBarHeight != topBarH {
			t.Fatalf("独立窗口应隐藏原生标题栏并预留顶栏高度：style=%q height=%d", got.TitleBarStyle, got.TitleBarHeight)
		}
	}
	if handle.buttonsTheme != mygo.ThemeDark {
		t.Fatalf("窗口按钮应按深色背景配色，实际 %q", handle.buttonsTheme)
	}
}

// TestMultipleDetachedWindowTitleBars 覆盖多个独立窗口各自隐藏原生标题栏并使用深色按钮。
func TestMultipleDetachedWindowTitleBars(t *testing.T) {
	a, _ := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	var handles []*fakeWindowHandle
	a.newTableWindow = func(opts mygo.WindowOptions) windowHandle {
		h := &fakeWindowHandle{title: opts.Title}
		handles = append(handles, h)
		return h
	}
	a.openTableInWindow()
	a.openTableInWindow()
	if len(handles) != 2 {
		t.Fatalf("应创建 2 个窗口，实际 %d", len(handles))
	}
	for i, h := range handles {
		if h.buttonsTheme != mygo.ThemeDark {
			t.Errorf("窗口 %d 按钮主题应深色，实际 %q", i, h.buttonsTheme)
		}
	}
}

// TestDetachButton 覆盖任务 3.1：数据视图工具栏显示「独立窗口」按钮且可点击。
func TestDetachButton(t *testing.T) {
	a, tt := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	a.tab = tabData
	var opened int
	// 用真实 openTableInWindow 会触发原生窗口；替换为计数以断言点击。
	orig := a.windows
	a.newTableWindow = func(opts mygo.WindowOptions) windowHandle {
		opened++
		return &fakeWindowHandle{title: opts.Title}
	}
	tt.Frame()
	_ = orig
	if !tt.HasText("独立窗口") {
		t.Fatalf("工具栏应显示「独立窗口」按钮，texts %q", tt.Texts())
	}
	if err := tt.Click("独立窗口"); err != nil {
		t.Fatalf("点击独立窗口: %v", err)
	}
	if opened != 1 {
		t.Fatalf("点击应打开一个独立窗口，实际 %d", opened)
	}
}

// TestCloseAllTableWindows 覆盖任务 3.2：主窗口退出时统一关闭所有独立窗口。
func TestCloseAllTableWindows(t *testing.T) {
	a, _ := newTester(t)
	a.openFile(fixture("basic.h5"))
	if err := a.state.SelectPath("/matrix"); err != nil {
		t.Fatal(err)
	}
	var handles []*fakeWindowHandle
	a.newTableWindow = func(opts mygo.WindowOptions) windowHandle {
		h := &fakeWindowHandle{title: opts.Title}
		handles = append(handles, h)
		return h
	}
	a.openTableInWindow()
	a.openTableInWindow()
	if len(a.windows) != 2 {
		t.Fatalf("应有 2 个独立窗口，实际 %d", len(a.windows))
	}
	a.closeAllTableWindows()
	if len(a.windows) != 0 {
		t.Fatalf("退出后应无独立窗口，实际 %d", len(a.windows))
	}
	for i, h := range handles {
		if !h.closed {
			t.Errorf("窗口 %d 应被关闭", i)
		}
	}
}
