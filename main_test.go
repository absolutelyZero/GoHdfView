package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
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
