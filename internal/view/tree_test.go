package view

import (
	"fmt"
	"image"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// fakeSource 是内存中的假对象树，用于在没有真实文件时构造任意形状。
//
// 节点只填公开字段：view 层从不直接调用 Node.Children，
// 一律经 Source，因此假节点不必持有解析库的句柄。
type fakeSource struct {
	root  *hdf5.Node
	kids  map[hdf5.NodeID][]*hdf5.Node
	calls int
}

func (f *fakeSource) Root() *hdf5.Node { return f.root }

func (f *fakeSource) Children(n *hdf5.Node) ([]*hdf5.Node, error) {
	f.calls++
	return f.kids[n.ID], nil
}

func newFake() *fakeSource {
	root := &hdf5.Node{
		ID:   hdf5.NodeID{Kind: hdf5.KindGroup, Path: "/"},
		Name: "/",
		Path: "/",
		Kind: hdf5.KindGroup,
	}
	return &fakeSource{root: root, kids: map[hdf5.NodeID][]*hdf5.Node{}}
}

// addGroup 在 parent 下加一个组。
func (f *fakeSource) addGroup(parent *hdf5.Node, name string) *hdf5.Node {
	p := join(parent.Path, name)
	n := &hdf5.Node{
		ID:   hdf5.NodeID{Kind: hdf5.KindGroup, Path: p},
		Name: name,
		Path: p,
		Kind: hdf5.KindGroup,
	}
	f.kids[parent.ID] = append(f.kids[parent.ID], n)
	return n
}

// addDataset 在 parent 下加一个数据集节点。
func (f *fakeSource) addDataset(parent *hdf5.Node, name string) *hdf5.Node {
	p := join(parent.Path, name)
	n := &hdf5.Node{
		ID:   hdf5.NodeID{Address: uint64(len(f.kids)) + 1000, Kind: hdf5.KindDataset, Path: p},
		Name: name,
		Path: p,
		Kind: hdf5.KindDataset,
	}
	f.kids[parent.ID] = append(f.kids[parent.ID], n)
	return n
}

func join(parent, name string) string {
	if parent == "/" || parent == "" {
		return "/" + name
	}
	return parent + "/" + name
}

// testView 把对象树与面包屑装成一个可测试的视图。
func testView(app *App) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
			Breadcrumb(c, app)
			Tree(c, app)
		})
	}
}

// newTester 建一个已装入假树的应用与测试器。
func newTester(t *testing.T, src *fakeSource) (*App, *ui.Tester) {
	t.Helper()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	return app, ui.NewTester(testView(app), 900, 600)
}

// themedTree 在测试里只画对象树并套用应用主题：避免面包屑出现同名文本，
// 让 tt.Find 取到树行本身，同时取到设计稿的底色与强调色。
func themedTree(app *App) func(c *ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(Theme())
		ui.Column(c).Fill().Padding(12).Children(func() {
			Tree(c, app)
		})
	}
}

// pixelAt 返回图像在 DIP 坐标 (x, y) 处的颜色。
func pixelAt(img *image.RGBA, x, y float32) ui.Color {
	p := img.RGBAAt(int(x), int(y))
	return ui.RGB(p.R, p.G, p.B)
}

// sameRGB 报告两色在每个通道的差值都不超过 tol。
func sameRGB(a, b ui.Color, tol int) bool {
	diff := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return diff(a.R, b.R) <= tol && diff(a.G, b.G) <= tol && diff(a.B, b.B) <= tol
}

// hasAccent 报告矩形内是否有接近 ColAccent 的像素：选中行的类型图标为
// 琥珀色，描边抗锯齿后仍保留接近纯色的像素，普通文字的抗锯齿边缘达不到
// 这个饱和度。
func hasAccent(img *image.RGBA, r ui.Rect) bool {
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			c := img.RGBAAt(x, y)
			if sameRGB(ui.RGB(c.R, c.G, c.B), ColAccent, 20) {
				return true
			}
		}
	}
	return false
}

// TestTreeRowHeight 断言树行高 28px，与设计稿一致。
func TestTreeRowHeight(t *testing.T) {
	treeFilter, treeApplied = "", ""
	src := newFake()
	grp := src.addGroup(src.root, "grp")
	src.addDataset(src.root, "top")
	src.addDataset(grp, "deep")
	app, tt := newTester(t, src)
	app.Tree.Open.Add(grp.ID)
	app.Tree.Generation++
	tt.Frame()

	grpRect, ok := tt.Find("grp")
	if !ok {
		t.Fatalf("缺少组节点，texts %q", tt.Texts())
	}
	deep, ok := tt.Find("deep")
	if !ok {
		t.Fatalf("缺少第二层节点，texts %q", tt.Texts())
	}
	top, ok := tt.Find("top")
	if !ok {
		t.Fatalf("缺少第一层节点，texts %q", tt.Texts())
	}
	if d := deep.Y - grpRect.Y; d < 27 || d > 29 {
		t.Fatalf("行高应为 28px，实际 %.1f", d)
	}
	if d := top.Y - deep.Y; d < 27 || d > 29 {
		t.Fatalf("行高应为 28px，实际 %.1f", d)
	}
}

// TestTreeSelectedHighlight 断言选中行按设计稿高亮：类型图标为琥珀色
// ColAccent，行底色为 ColSelRow；切换选中后原行恢复普通样式。
func TestTreeSelectedHighlight(t *testing.T) {
	treeFilter, treeApplied = "", ""
	defer func() { treeFilter, treeApplied = "", "" }()

	src := newFake()
	src.addDataset(src.root, "one")
	src.addDataset(src.root, "two")
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	tt := ui.NewTester(themedTree(app), 900, 600)

	if err := app.SelectPath("/one"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	one, ok := tt.Find("one")
	if !ok {
		t.Fatalf("缺少 one 行，texts %q", tt.Texts())
	}
	// tt.Find 给出整行矩形：扫描整行寻找琥珀色的类型图标。
	band := ui.Rect{X: 0, Y: one.Y, W: 900, H: one.H}
	if !hasAccent(tt.Image(), band) {
		t.Fatalf("选中行 one 的类型图标应为琥珀色 ColAccent")
	}
	// 行右侧的空白处应填满选中行的底色。
	if got := pixelAt(tt.Image(), 820, one.Y+one.H/2); !sameRGB(got, ColSelRow, 1) {
		t.Fatalf("选中行 one 的底色应为 ColSelRow %v，实际 %v", ColSelRow, got)
	}

	if err := app.SelectPath("/two"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	one, ok = tt.Find("one")
	if !ok {
		t.Fatalf("切换选中后缺少 one 行，texts %q", tt.Texts())
	}
	two, ok := tt.Find("two")
	if !ok {
		t.Fatalf("切换选中后缺少 two 行，texts %q", tt.Texts())
	}
	band = ui.Rect{X: 0, Y: one.Y, W: 900, H: one.H}
	if hasAccent(tt.Image(), band) {
		t.Fatalf("切换选中后原行 one 不应再有琥珀色图标")
	}
	if got := pixelAt(tt.Image(), 820, one.Y+one.H/2); sameRGB(got, ColSelRow, 1) {
		t.Fatalf("切换选中后原行 one 的底色应恢复普通样式，实际仍为 %v", got)
	}
	twoBand := ui.Rect{X: 0, Y: two.Y, W: 900, H: two.H}
	if !hasAccent(tt.Image(), twoBand) {
		t.Fatalf("切换选中后 two 行的类型图标应为琥珀色")
	}
}

// TestTreeSearchShortcut 断言 Ctrl/Cmd+K 聚焦筛选框并生效。
func TestTreeSearchShortcut(t *testing.T) {
	treeFilter, treeApplied = "", ""
	defer func() { treeFilter, treeApplied = "", "" }()

	src := newFake()
	src.addDataset(src.root, "matrix")
	src.addDataset(src.root, "spectrum")
	_, tt := newTester(t, src)
	tt.Frame()

	mod := ui.Ctrl
	if runtime.GOOS == "darwin" {
		mod = ui.Cmd
	}
	tt.Key(mod, ui.KeyK)
	tt.Frame()
	tt.Type("matrix")
	tt.Frame()

	if !tt.HasText("matrix") {
		t.Fatalf("筛选后应保留 matrix，实际 %q", tt.Texts())
	}
	if tt.HasText("spectrum") {
		t.Fatalf("筛选后不应显示 spectrum，实际 %q", tt.Texts())
	}
}

// TestTreeShowsRootAndChildren 对应任务 3.1：打开夹具后可见根节点与子节点。
func TestTreeShowsRootAndChildren(t *testing.T) {
	src := newFake()
	ds := src.addDataset(src.root, "matrix")
	src.addGroup(src.root, "results")
	app, tt := newTester(t, src)
	tt.Frame()

	if !tt.HasText("/") {
		t.Fatalf("应显示根节点名，实际文本 %q", tt.Texts())
	}
	if !tt.HasText("matrix") {
		t.Fatalf("应显示数据集名，实际文本 %q", tt.Texts())
	}
	if ds.Kind != hdf5.KindDataset {
		t.Fatalf("节点种类应为数据集")
	}
	if app.Current == nil || app.Current.Path != "/" {
		t.Fatalf("打开后当前对象应为根组，实际 %+v", app.Current)
	}
}

// TestTreeExpandedStaysOpen 对应任务 3.2：展开一个组后选中兄弟节点，组仍展开。
func TestTreeExpandedStaysOpen(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	g := src.addGroup(root, "results")
	src.addDataset(g, "temperature")
	src.addDataset(root, "matrix")
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	// 展开 results。
	app.Tree.Open.Add(g.ID)
	app.Tree.Generation++
	tt.Frame()
	if !tt.HasText("temperature") {
		t.Fatalf("展开后应显示子节点，实际 %q", tt.Texts())
	}

	// 选中同层的 matrix。
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	if !app.Tree.Open.Has(g.ID) {
		t.Fatalf("选中兄弟节点后组不应折叠")
	}
	if !tt.HasText("temperature") {
		t.Fatalf("子节点应仍然可见，实际 %q", tt.Texts())
	}
	if app.Current == nil || app.Current.Name != "matrix" {
		t.Fatalf("当前对象应为 matrix，实际 %+v", app.Current)
	}
}

// TestExpandCollapseAll 对应任务 3.3：全部展开/折叠后根节点仍可见。
func TestExpandCollapseAll(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	a := src.addGroup(root, "a")
	b := src.addGroup(a, "b")
	src.addDataset(b, "leaf")
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	before := len(app.visibleIDs())
	app.ExpandAll()
	tt.Frame()
	after := len(app.visibleIDs())
	if after <= before {
		t.Fatalf("全部展开后可见行应增加：%d → %d", before, after)
	}
	if !tt.HasText("leaf") {
		t.Fatalf("全部展开后应显示最深节点，实际 %q", tt.Texts())
	}

	app.CollapseAll()
	tt.Frame()
	if got := len(app.visibleIDs()); got != 1 {
		t.Fatalf("全部折叠后只应剩根节点，实际 %d 行", got)
	}
	if !tt.HasText("/") {
		t.Fatalf("全部折叠后根节点应仍可见，实际 %q", tt.Texts())
	}
}

// TestBreadcrumbSegments 对应任务 3.4：显示完整路径分段，点击祖先跳转。
func TestBreadcrumbSegments(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	results := src.addGroup(root, "results")
	run := src.addGroup(results, "run-3")
	src.addDataset(run, "temperature")
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	if err := app.SelectPath("/results/run-3/temperature"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	for _, seg := range []string{"results", "run-3", "temperature"} {
		if !tt.HasText(seg) {
			t.Fatalf("面包屑应包含分段 %q，实际 %q", seg, tt.Texts())
		}
	}

	if err := tt.Click("results"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	if app.Current == nil || app.Current.Path != "/results" {
		t.Fatalf("点击分段后应选中 /results，实际 %+v", app.Current)
	}
}

// TestTreeKeyboardMoves 对应任务 3.5：方向键在同层可见节点间移动选中项。
func TestTreeKeyboardMoves(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	src.addDataset(root, "one")
	src.addDataset(root, "two")
	src.addDataset(root, "three")
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	// 点击选中第一行，使树获得键盘焦点；方向键只在焦点内生效。
	if err := tt.Click("one"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	tt.Frame()
	start := app.Current.Name

	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if app.Current == nil || app.Current.Name == start {
		t.Fatalf("按下方向键后选中项应变化，仍为 %q", start)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if app.Current == nil || app.Current.Name != start {
		t.Fatalf("按上方向键后应回到 %q，实际 %+v", start, app.Current)
	}
}

// TestTreeLargeGroup 对应任务 3.6：一万成员的组只按需取子节点。
func TestTreeLargeGroup(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	wide := src.addGroup(root, "wide")
	const n = 10000
	for i := 0; i < n; i++ {
		src.addDataset(wide, fmt.Sprintf("ds%05d", i))
	}
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	app.Tree.Open.Add(wide.ID)
	app.Tree.Generation++
	tt.Frame()

	// 根 + wide 两次取子节点；一万个数据集不产生额外调用。
	if src.calls > 4 {
		t.Fatalf("Children 调用应为常数级，实际 %d", src.calls)
	}
	// 帧间无变化时，Generation 让 Outline 复用行，不再重走。
	calls := src.calls
	tt.Frame()
	tt.Frame()
	if src.calls != calls {
		t.Fatalf("无变化时不应重新取子节点：%d → %d", calls, src.calls)
	}
	if len(app.visibleIDs()) != n+2 {
		t.Fatalf("可见行数应为 %d，实际 %d", n+2, len(app.visibleIDs()))
	}
}

// TestSelectKeepsObjectWhenAncestorCollapsed 验证折叠祖先不会丢失当前对象。
func TestSelectKeepsObjectWhenAncestorCollapsed(t *testing.T) {
	src := newFake()
	app := NewApp()
	if err := app.SetSource(src); err != nil {
		t.Fatalf("SetSource: %v", err)
	}
	root := src.root
	g := src.addGroup(root, "g")
	src.addDataset(g, "d")
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	if err := app.SelectPath("/g/d"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	app.CollapseAll()
	tt.Frame()

	if app.Current == nil || app.Current.Path != "/g/d" {
		t.Fatalf("折叠后当前对象应保留，实际 %+v", app.Current)
	}
}
