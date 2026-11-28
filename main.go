package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
	"gohdfview/internal/view"
)

// appVersion 是应用自身的版本，与 mygo.json 的 version 保持一致。
const appVersion = "0.1.1"

// 右侧工作区的三个标签页（viewer-shell 的「视图切换」）。
const (
	tabData = iota
	tabMetadata
	tabAttributes
)

// 数据视图内部的两种呈现方式（dataset-data-view 的图像视图）。
const (
	modeTable = iota
	modeImage
)

// 界面尺寸取自设计稿（A · 仪器）。
const (
	topBarH   = 46
	sidebarW  = 296
	tabStripH = 40
	statusH   = 30
	// inspectorBreakpoint 是显示右侧检查器所需的最小窗口宽度。
	inspectorBreakpoint = 1180
)

// app 是窗口要展示的状态。视图在主线程上按帧从它构建界面：
// 输入之后、Window.Update 之后，以及动画进行时。
//
// 跨帧存活的字段只放在这里；控件本身是 build-pass 的临时值，
// 不能存进结构体（见 design.md 决策 1 与 9）。
type app struct {
	// state 是唯一的视图状态：一棵统一对象树容纳所有已打开文件。
	state *view.App
	// files 是已打开文件的句柄集合，负责去重与退出时释放。
	files []*openFile
	// viewStates 按文件保存视图标签与数据模式，实现「视图状态按文件隔离」。
	viewStates map[string]*fileViewState
	// lastFile 是上一帧选中的文件标识，用于切换文件时保存/恢复视图状态。
	lastFile string
	// windows 是已打开的数据表独立窗口；newTableWindow 可注入以测试。
	windows        []*tableWindow
	newTableWindow windowFactory
	// win 是主窗口，用于改标题与请求重绘；无窗口时为 nil。
	win *mygo.Window

	// split 是左栏宽度（DIP），由 ui.Split 拖动改写。
	split float32
	// tab 是当前选中的工作区标签页。
	tab int
	// dataMode 是数据视图内当前选中的呈现方式。
	dataMode int

	// errMsg 是当前显示的非模态错误提示；为空表示没有提示。
	//
	// 打开失败时保留原有文档，只在这里追加一条可关闭的提示
	// （viewer-shell 的「错误提示不阻塞浏览」）。
	errMsg string

	// recentExport 是最近一次成功导出的路径与完成时间；path 为空表示无提示。
	recentExport struct {
		path string
		at   time.Time
	}

	// openExportedFile / revealExportedFile 可由测试注入以替代系统 Shell 调用。
	openExportedFile   func(string) error
	revealExportedFile func(string)
}

// openFile 是 main 维护的一个已打开文件句柄。
// 统一树的状态在 view.App 里，这里只负责句柄生命周期与路径去重。
type openFile struct {
	path string // 规范化绝对路径，用于去重
	doc  *hdf5.Document
}

// fileViewState 是单个文件的视图标签与数据模式。
type fileViewState struct {
	tab      int
	dataMode int
}

// windowHandle 是独立窗口需要的原生窗口能力子集，便于测试注入假实现。
type windowHandle interface {
	Show()
	Close()
	Invalidate()
	SetTitle(string)
	SetWindowButtonsTheme(mygo.ThemeSource)
	OnClose(func(*mygo.CloseEvent)) func()
}

// windowFactory 创建原生窗口；默认使用 mygo.NewWindow。
type windowFactory func(mygo.WindowOptions) windowHandle

// tableWindow 是一个独立数据表窗口：持有自己的视图状态与窗口句柄。
// 文档句柄归主窗口所有，这里只读复用。
type tableWindow struct {
	win     windowHandle
	state   *view.App
	title   string
	closing bool
}

// currentFile 返回当前选中节点所属文件的标识；无选中时返回空串。
func (a *app) currentFile() string {
	if a.state == nil || a.state.Current == nil {
		return ""
	}
	return a.state.Current.ID.File
}

// switchToFile 在选中文件变化时保存旧文件视图状态、恢复新文件视图状态。
// 视图标签与数据模式因而按文件隔离（design.md 决策 6）。
// 必须在改变 currentFile 的操作之后立即调用，以便先捕获旧文件状态。
func (a *app) switchToFile() {
	f := a.currentFile()
	if f == a.lastFile {
		return
	}
	if a.lastFile != "" {
		a.viewStates[a.lastFile] = &fileViewState{tab: a.tab, dataMode: a.dataMode}
	}
	a.lastFile = f
	if vs := a.viewStates[f]; vs != nil {
		a.tab, a.dataMode = vs.tab, vs.dataMode
	} else {
		a.tab, a.dataMode = tabData, modeTable
	}
}

// wireState 给唯一的 view.App 注入导出、独立窗口与异步回调。
// 无窗口的测试环境保持同步读取，便于逐帧断言。
func (a *app) wireState() {
	st := a.state
	st.SetCSVExporter(a.saveCSV)
	st.SetTableDetacher(a.openTableInWindow)
	if a.win == nil {
		return
	}
	st.SetAsync(func(work func()) { go work() })
	st.SetInvalidate(func() { a.invalidate() })
}

// newApp 创建空状态的应用。
//
// 这里不注入异步调度器：测试用同步读取以便逐帧断言，
// 真实窗口在 main 里再装上工作 goroutine。
func newApp() *app {
	a := &app{
		state:          view.NewApp(),
		viewStates:     make(map[string]*fileViewState),
		split:          sidebarW,
		tab:            tabData,
		dataMode:       modeTable,
		newTableWindow: func(opts mygo.WindowOptions) windowHandle { return mygo.NewWindow(opts) },
	}
	a.wireState()
	return a
}

// invalidate 请求下一帧重绘；没有窗口时什么也不做。
func (a *app) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

// setError 记录一条非模态错误提示。
func (a *app) setError(err error) {
	if err == nil {
		return
	}
	a.errMsg = err.Error()
	a.invalidate()
}

// openFile 以只读方式打开文件，作为根节点加入统一对象树并选中。
// 打开失败保留当前状态并显示非模态提示；重复打开同一路径只定位到已有根节点。
func (a *app) openFile(path string) {
	if path == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	for _, of := range a.files {
		if of.path == abs || filepath.Clean(of.doc.Path()) == filepath.Clean(abs) {
			if root := of.doc.Root(); root != nil {
				a.state.Select(root.ID)
			}
			a.refreshTitle()
			a.invalidate()
			return
		}
	}
	doc, err := hdf5.Open(abs)
	if err != nil {
		a.setError(err)
		return
	}
	if err := a.state.AddDocument(doc); err != nil {
		doc.Close()
		a.setError(err)
		return
	}
	a.files = append(a.files, &openFile{path: abs, doc: doc})
	a.switchToFile()
	a.refreshTitle()
	a.invalidate()
}

// closeFile 关闭当前选中节点所属的文件（Ctrl+W / 文件菜单）。
func (a *app) closeFile() {
	file := a.currentFile()
	if file == "" {
		return
	}
	for i, of := range a.files {
		if of.doc.Root() != nil && of.doc.Root().ID.File == file {
			a.state.RemoveDocument(file)
			_ = of.doc.Close()
			a.files = append(a.files[:i], a.files[i+1:]...)
			delete(a.viewStates, file)
			if a.lastFile == file {
				a.lastFile = ""
			}
			break
		}
	}
	a.refreshTitle()
	a.invalidate()
}

// detachedTableView 返回独立数据表窗口的内容视图：与主窗口 view() 一致，
// 每帧先设置深色主题，自绘深色标题栏（原生标题栏已隐藏），再以 ColBg 背景包裹 DataTable。
func detachedTableView(st *view.App, title string) func(*ui.Context) {
	return func(c *ui.Context) {
		c.SetTheme(view.Theme())
		ui.Column(c).Fill().Background(view.ColBg).Children(func() {
			// 自绘标题栏：交通灯浮在其上，空白处可拖动窗口。
			tb := c.TitleBar()
			h := float32(topBarH)
			if tb.Height > h {
				h = tb.Height
			}
			bar := ui.Row(c).Shrink(0).Height(h).AlignItems(ui.Center).
				Padding(0, tb.Right+16, 0, tb.Left+16).Background(view.ColPanel)
			if tb.Height > 0 {
				bar.DragWindow()
			}
			bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLine) })
			bar.Children(func() {
				ui.Text(c, title).FontSize(13).FontWeight(600).
					TextColor(view.ColText).SingleLine()
			})
			view.DataTable(c, st)
		})
	}
}

// openTableInWindow 把当前选中的数据集的数据表放到独立原生窗口。
// 每个窗口持有独立 view.App，内容在打开时固定，不随主窗口选中变化。
func (a *app) openTableInWindow() {
	node := a.state.Current
	if node == nil || !a.state.DataAvailable() {
		return
	}
	doc := a.state.Document()
	if doc == nil {
		return
	}

	// 独立状态：复用同一份只读文档句柄，定位到打开时选中的数据集。
	st := view.NewApp()
	if err := st.AddDocument(doc); err != nil {
		a.setError(err)
		return
	}
	if err := st.SelectPath(node.Path); err != nil {
		a.setError(err)
		return
	}
	if a.win != nil {
		st.SetAsync(func(work func()) { go work() })
	}
	st.SetCSVExporter(a.saveCSV)

	title := node.Name + " — " + filepath.Base(doc.Path())
	tw := &tableWindow{state: st, title: title}
	if a.win != nil {
		st.SetInvalidate(func() {
			if tw.win != nil {
				tw.win.Invalidate()
			}
		})
	}
	factory := a.newTableWindow
	if factory == nil {
		factory = func(opts mygo.WindowOptions) windowHandle { return mygo.NewWindow(opts) }
	}
	content := ui.View(detachedTableView(st, title))
	opts := mygo.WindowOptions{
		Title:   title,
		Width:   1000,
		Height:  700,
		Content: content,
	}
	// 与主窗口一致：隐藏原生标题栏（macOS 避免出现浅色系统条），
	// 由自绘深色标题栏充当，窗口按钮按深色背景配色。
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		opts.TitleBarStyle = mygo.TitleBarHidden
		opts.TitleBarHeight = topBarH
	}
	tw.win = factory(opts)
	if tw.win != nil {
		tw.win.SetWindowButtonsTheme(mygo.ThemeDark)
		tw.win.OnClose(func(*mygo.CloseEvent) { a.closeTableWindow(tw) })
		tw.win.Show()
	}
	a.windows = append(a.windows, tw)
	a.invalidate()
}

// closeTableWindow 关闭一个独立数据表窗口；不关闭共享的文档句柄。
func (a *app) closeTableWindow(tw *tableWindow) {
	if tw == nil || tw.closing {
		return
	}
	tw.closing = true
	for i, w := range a.windows {
		if w == tw {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	if tw.win != nil {
		tw.win.Close()
	}
}

// closeAllTableWindows 关闭所有独立窗口（主窗口退出时调用）。
func (a *app) closeAllTableWindows() {
	for _, w := range append([]*tableWindow(nil), a.windows...) {
		a.closeTableWindow(w)
	}
}

// refreshTitle 把文件名写进窗口标题。
func (a *app) refreshTitle() {
	if a.win == nil {
		return
	}
	if doc := a.state.Document(); doc != nil {
		a.win.SetTitle("GoHdfView — " + filepath.Base(doc.Path()))
		return
	}
	a.win.SetTitle("GoHdfView")
}

// openDialog 弹出系统文件选择对话框，选中后打开（任务 7.3）。
//
// 对话框会阻塞到用户关闭它，所以放到别的 goroutine；
// 结果再经 Window.Update 回到主线程改动状态。
func (a *app) openDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "打开 HDF5 文件",
			Multiple: true,
			Filters: []mygo.FileFilter{
				{Name: "HDF5 文件", Extensions: []string{"h5", "hdf5", "he5", "mat"}},
				{Name: "所有文件", Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		apply := func() {
			for _, p := range paths {
				a.openFile(p)
			}
		}
		if a.win == nil {
			apply()
			return
		}
		a.win.Update(apply)
	}()
}

// saveCSV 是注入到 view 的导出处理器：弹出保存对话框后用临时文件原子写入。
// scope 仅用于默认文件名提示；实际 CSV 内容由 write 决定。
func (a *app) saveCSV(scope view.CSVExportScope, write view.CSVWriteFunc) {
	name := "data.csv"
	if scope == view.CSVScopeAll {
		name = "data-all.csv"
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "导出数据表为 CSV",
			DefaultPath: name,
			Filters: []mygo.FileFilter{
				{Name: "CSV 文件", Extensions: []string{"csv"}},
				{Name: "所有文件", Extensions: []string{"*"}},
			},
		})
		if err != nil || path == "" {
			return // 取消：不产生文件
		}
		if err := writeCSVFile(path, write); err != nil {
			a.reportExportError(err)
			return
		}
		a.reportExportDone(path)
	}()
}

// reportExportDone 把成功导出的路径切回主线程，记录为「最近导出」并请求重绘。
func (a *app) reportExportDone(path string) {
	set := func() {
		a.recentExport.path = path
		a.recentExport.at = time.Now()
		a.invalidate()
	}
	if a.win != nil {
		a.win.Update(set)
		return
	}
	set()
}

// openExported 用系统默认应用打开导出的 CSV。
func (a *app) openExported(path string) {
	if a.openExportedFile != nil {
		if err := a.openExportedFile(path); err != nil {
			a.setError(err)
		}
		return
	}
	if err := mygo.Shell.OpenPath(path); err != nil {
		a.setError(err)
	}
}

// revealExported 在文件管理器中定位导出的 CSV。
func (a *app) revealExported(path string) {
	if a.revealExportedFile != nil {
		a.revealExportedFile(path)
		return
	}
	mygo.Shell.ShowItemInFolder(path)
}

// writeCSVFile 用临时文件 + 原子重命名把 CSV 写到 path；失败时清理临时文件。
func writeCSVFile(path string, write view.CSVWriteFunc) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err == nil {
		err = write(f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// reportExportError 把后台导出的错误切回主线程显示。
func (a *app) reportExportError(err error) {
	show := func() { a.setError(err) }
	if a.win != nil {
		a.win.Update(show)
		return
	}
	show()
}

// showAbout 显示版本号与解析库版本（帮助菜单）。
func (a *app) showAbout() {
	if a.win == nil {
		return
	}
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Parent: a.win,
			Type:   mygo.MessageInfo,
			Title:  "关于 GoHdfView",
			Message: fmt.Sprintf("GoHdfView %s\nHDF5 解析库：%s %s",
				appVersion, hdf5.LibraryName, hdf5.LibraryVersion),
			Detail:  "只读 HDF5 阅读器，基于 MyGo 原生界面。",
			Buttons: []string{"确定"},
		})
	}()
}

// menu 构建「文件 / 视图 / 帮助」菜单（任务 7.2）。
//
// 菜单项调用与视图完全相同的操作函数，因此快捷键与按钮行为一致。
func (a *app) menu() *mygo.Menu {
	setTab := func(t int) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if t == tabData && !a.state.DataAvailable() {
				return
			}
			a.tab = t
			a.invalidate()
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.openDialog()
			}},
			mygo.Separator(),
			{Label: "关闭文件", Accelerator: "CmdOrCtrl+W", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.closeFile()
				a.invalidate()
			}},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "数据", Accelerator: "CmdOrCtrl+1", Click: setTab(tabData)},
			{Label: "元数据", Accelerator: "CmdOrCtrl+2", Click: setTab(tabMetadata)},
			{Label: "属性", Accelerator: "CmdOrCtrl+3", Click: setTab(tabAttributes)},
			mygo.Separator(),
			{Label: "在独立窗口打开数据表", Accelerator: "CmdOrCtrl+Shift+D", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.openTableInWindow()
			}},
			mygo.Separator(),
			{Label: "全部展开", Accelerator: "CmdOrCtrl+Shift+E", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.state.ExpandAll()
				a.invalidate()
			}},
			{Label: "全部折叠", Accelerator: "CmdOrCtrl+Shift+W", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.state.CollapseAll()
				a.invalidate()
			}},
		}},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			{Label: "关于", Click: func(*mygo.MenuItem, *mygo.Window) { a.showAbout() }},
		}},
	})
}

// view 是窗口内容：顶栏 + 左树 / 右工作区 + 状态栏。
func (a *app) view(c *ui.Context) {
	a.switchToFile()
	c.SetTheme(view.Theme())
	ui.Column(c).Fill().Background(view.ColBg).Children(func() {
		a.topBar(c)
		if !a.state.HasDocument() {
			a.emptyState(c)
			return
		}
		ui.Split(c, &a.split,
			func() { a.sidebar(c) },
			func() { a.workspace(c) }).Grow(1)
		if a.errMsg != "" {
			a.errorBanner(c)
		}
		a.statusBar(c)
	})
}

// topBar 是 46px 的顶栏：标记、文件名、菜单、只读徽标与打开按钮。
//
// Windows 与 macOS 上窗口使用隐藏标题栏（TitleBarHidden），顶栏同时充当标题栏：
// 为窗口按钮留出 c.TitleBar().Right（macOS 交通灯在左侧，对应 Left），并让空白处可拖动窗口。
func (a *app) topBar(c *ui.Context) {
	tb := c.TitleBar()
	height := float32(topBarH)
	if tb.Height > height {
		height = tb.Height
	}
	bar := ui.Row(c).Shrink(0).Height(height).AlignItems(ui.Center).
		Padding(0, tb.Right+16, 0, tb.Left+16).Gap(14).Background(view.ColPanel)
	if tb.Height > 0 {
		bar.DragWindow()
	}
	bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLine) })
	bar.Children(func() {
		// 标记直接用 logo.pen 导出的位图：镜片里嵌对象树，与窗口图标同源。
		if bm := view.Mark(); bm != nil {
			ui.Image(c, bm).Fit(ui.Contain).Size(24, 24).Shrink(0)
		} else {
			// 位图解码失败时退回文字方块，界面不至于缺角。
			mark := ui.Box(c).Size(20, 20).Shrink(0).Radius(4).
				Background(view.ColAccent).Center()
			mark.Children(func() {
				ui.Text(c, "G").FontSize(11).FontWeight(700).TextColor(view.ColBg)
			})
		}
		ui.Text(c, "GoHdfView").FontSize(13).FontWeight(600).
			TextColor(view.ColText).SingleLine()
		name := ""
		if doc := a.state.Document(); doc != nil {
			name = filepath.Base(doc.Path())
		}
		if name != "" {
			a.topDivider(c)
			ui.Text(c, name).Font(view.FontMono).FontSize(12).TextColor(view.ColDim).SingleLine()
		}
		ui.Spacer(c).Grow(1)
		a.menuButton(c, "文件", a.fileMenu)
		a.menuButton(c, "视图", a.viewMenu)
		a.menuButton(c, "帮助", a.helpMenu)
		a.topDivider(c)
		a.readOnlyBadge(c)
		view.AccentButton(c, "打开", func() { a.openDialog() })
	})
}

func (a *app) topDivider(c *ui.Context) {
	ui.Box(c).Width(1).Height(16).Shrink(0).Background(view.ColLine)
}

// menuButton 是顶栏的纯文字菜单按钮，菜单画在窗口内。
func (a *app) menuButton(c *ui.Context, label string, build func(m *ui.Menu)) {
	b := ui.ButtonBase(c).Padding(4, 6).Radius(4)
	b.Children(func() {
		ui.Text(c, label).FontSize(12.5).TextColor(view.ColDim).SingleLine()
	})
	b.PopoverMenuButton(build, ui.PopoverMenuButtonOptions{
		Panel: func(panel ui.Element) {
			panel.Background(view.ColPanel2).Border(1, view.ColLine).Radius(6).
				NoShadow().Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.45))
		},
		Item: func(item ui.Element) { item.Padding(4, 8).Radius(4) },
	})
}

func (a *app) readOnlyBadge(c *ui.Context) {
	badge := ui.Row(c).Shrink(0).Height(24).AlignItems(ui.Center).Gap(6).
		Padding(0, 9).Radius(4).Background(ui.Hex("#14181C"))
	badge.Children(func() {
		ui.Box(c).Size(5, 5).Shrink(0).Radius(3).Background(view.ColAccent)
		ui.Text(c, "只读").FontSize(11).TextColor(view.ColDim).SingleLine()
	})
}

// fileMenu / viewMenu / helpMenu 是窗口内菜单的内容，与系统菜单同源。
func (a *app) fileMenu(m *ui.Menu) {
	if m.Item("打开…").Shortcut(ui.Ctrl, ui.KeyO).Chosen() {
		a.openDialog()
	}
	m.Separator()
	if m.Item("关闭文件").Shortcut(ui.Ctrl, ui.KeyW).Chosen() {
		a.closeFile()
		a.invalidate()
	}
}

func (a *app) viewMenu(m *ui.Menu) {
	pick := func(t int) {
		if t == tabData && !a.state.DataAvailable() {
			return
		}
		a.tab = t
		a.invalidate()
	}
	if m.Item("数据").Chosen() {
		pick(tabData)
	}
	if m.Item("元数据").Chosen() {
		pick(tabMetadata)
	}
	if m.Item("属性").Chosen() {
		pick(tabAttributes)
	}
	m.Separator()
	if m.Item("在独立窗口打开数据表").Chosen() {
		a.openTableInWindow()
	}
	m.Separator()
	if m.Item("全部展开").Chosen() {
		a.state.ExpandAll()
		a.invalidate()
	}
	if m.Item("全部折叠").Chosen() {
		a.state.CollapseAll()
		a.invalidate()
	}
}

func (a *app) helpMenu(m *ui.Menu) {
	if m.Item("关于").Chosen() {
		a.showAbout()
	}
}

// emptyState 是没有打开文件时的界面。
func (a *app) emptyState(c *ui.Context) {
	ui.Column(c).Fill().Grow(1).Center().Gap(14).Padding(24).Children(func() {
		// 空状态用琥珀色标记：与顶栏同一图形，只在没有内容时点亮。
		if bm := view.MarkAccent(); bm != nil {
			ui.Image(c, bm).Fit(ui.Contain).Size(56, 56)
		}
		ui.Text(c, "打开 HDF5 文件或把文件拖入窗口").FontSize(13).
			TextColor(view.ColDim)
		view.AccentButton(c, "打开…", func() { a.openDialog() })
		if a.errMsg != "" {
			box := ui.Column(c).Width(520).Gap(8).Padding(10, 12).Radius(5).
				Background(view.ColWarnBg).Border(1, view.ColLine)
			box.Children(func() {
				ui.Text(c, a.errMsg).FontSize(12).TextColor(view.ColDim)
				ui.Row(c).Children(func() {
					ui.Spacer(c).Grow(1)
					view.GhostButton(c, "关闭提示", func() { a.errMsg = "" })
				})
			})
		}
	})
}

// sidebar 是左栏：标题栏、树操作与对象树。
func (a *app) sidebar(c *ui.Context) {
	col := ui.Column(c).Fill().Background(view.ColPanel)
	col.Draw(func(p *ui.Painter, r ui.Rect) { view.VLine(p, r, view.ColLine) })
	col.Children(func() {
		head := ui.Row(c).Shrink(0).Height(38).AlignItems(ui.Center).
			Padding(0, 14).Gap(8)
		head.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLineSoft) })
		head.Children(func() {
			ui.Text(c, "对象树").FontSize(11).FontWeight(500).TextColor(view.ColMute).
				LetterSpacing(1.1).SingleLine()
			ui.Spacer(c).Grow(1)
			a.iconAction(c, view.IconExpand, "全部展开", func() { a.state.ExpandAll() })
			a.iconAction(c, view.IconCollapse, "全部折叠", func() { a.state.CollapseAll() })
			a.treeMore(c)
		})
		view.Tree(c, a.state)
	})
}

// iconAction 是侧栏标题栏的 13px 图标按钮。
func (a *app) iconAction(c *ui.Context, icon *ui.SVG, label string, onClick func()) {
	b := ui.ButtonBase(c).Padding(2).Radius(4).Label(label)
	b.Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(view.ColMute)
	})
	b.OnClick(onClick)
}

// treeMore 是侧栏标题栏的更多菜单，与图标按钮承载相同的树操作。
func (a *app) treeMore(c *ui.Context) {
	b := ui.ButtonBase(c).Padding(2).Radius(4).Label("更多")
	b.Children(func() {
		ui.Icon(c, view.IconMore).Size(13, 13).TextColor(view.ColMute)
	})
	b.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item("全部展开").Chosen() {
			a.state.ExpandAll()
		}
		if m.Item("全部折叠").Chosen() {
			a.state.CollapseAll()
		}
	}, ui.PopoverMenuButtonOptions{
		Panel: func(panel ui.Element) {
			panel.Background(view.ColPanel2).Border(1, view.ColLine).Radius(6).
				NoShadow().Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.45))
		},
		Item: func(item ui.Element) { item.Padding(4, 8).Radius(4) },
	})
}

// workspace 是右栏：路径、标题、标签页、当前视图与检查器。
func (a *app) workspace(c *ui.Context) {
	// 选中的对象没有数据视图时，「数据」标签置灰并自动落到「元数据」。
	if a.tab == tabData && !a.state.DataAvailable() {
		a.tab = tabMetadata
	}
	// 窗口窄时收起检查器，优先保证表格可用宽度。
	w, _ := c.Size()
	showInspector := w >= inspectorBreakpoint
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Fill().Grow(1).Children(func() {
			view.Breadcrumb(c, a.state)
			a.titleBlock(c)
			a.tabBar(c)
			ui.Column(c).Fill().Grow(1).Children(func() {
				switch a.tab {
				case tabData:
					a.dataView(c)
				case tabAttributes:
					view.Attributes(c, a.state)
				default:
					view.Metadata(c, a.state)
				}
			})
		})
		if showInspector {
			view.Inspector(c, a.state)
		}
	})
}

// titleBlock 是对象标题：名称、类型标签与维度概要。
func (a *app) titleBlock(c *ui.Context) {
	n := a.state.Current
	if n == nil {
		return
	}
	box := ui.Column(c).Shrink(0).Padding(16, 20).Gap(8)
	box.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, n.Name).FontSize(22).FontWeight(600).
				TextColor(view.ColText).SingleLine()
			view.Chip(c, view.TypeChip(n))
			ui.Spacer(c).Grow(1)
			if s := n.DimensionSummary(); s != "" {
				ui.Text(c, s).Font(view.FontMono).FontSize(11.5).
					TextColor(view.ColMute).SingleLine()
			}
		})
	})
}

// tabBar 画三个视图标签，右侧是表格/图像的分段切换。
//
// 用 TabsBase 而不是 Tabs：只有前者能把单个标签设为不可用，
// 而「数据」标签在选中组时必须置灰（viewer-shell 的视图切换）。
func (a *app) tabBar(c *ui.Context) {
	names := []string{"数据", "元数据", "属性"}
	bar := ui.Row(c).Shrink(0).Height(tabStripH).AlignItems(ui.Stretch).
		Padding(0, 20).Gap(22)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLineSoft) })
	bar.Children(func() {
		tabs := ui.TabsBase(c, &a.tab, len(names))
		tabs.List.Gap(22).AlignItems(ui.Stretch).Children(func() {
			for i, name := range names {
				i, name := i, name
				on := i == a.tab
				col := view.ColMute
				if on {
					col = view.ColText
				}
				tab := tabs.Tab(i).Justify(ui.End).Padding(0, 2)
				if i == tabData && !a.state.DataAvailable() {
					tab.Disabled(true)
				}
				tab.Children(func() {
					ui.Text(c, name).FontSize(13).FontWeight(weight(on, 600)).
						TextColor(col).SingleLine()
				})
				tab.Draw(func(p *ui.Painter, r ui.Rect) {
					if on {
						p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2},
							view.ColAccent, 1)
					}
				})
			}
		})
		ui.Spacer(c).Grow(1)
		if a.state.DataAvailable() {
			a.modeSwitch(c)
		}
	})
}

func weight(on bool, w int) int {
	if on {
		return w
	}
	return 400
}

// modeSwitch 是「表格 / 图像」分段控件（A · 仪器设计稿）。
func (a *app) modeSwitch(c *ui.Context) {
	names := []string{"表格", "图像"}
	icons := []*ui.SVG{view.IconTable, view.IconImage}
	seg := ui.SegmentedBase(c, &a.dataMode, len(names))
	seg.Track.AlignItems(ui.Stretch).Padding(2).Gap(2).Radius(5).
		Background(ui.Hex("#0B0E10")).Border(1, view.ColLine)
	seg.Track.Children(func() {
		for i := range names {
			i := i
			on := i == a.dataMode
			col := view.ColMute
			if on {
				col = view.ColText
			}
			s := seg.Segment(i).Padding(4, 9).Radius(4).Gap(5).
				AlignItems(ui.Center).Justify(ui.Center)
			if on {
				s.Background(view.ColSelRow)
			}
			s.Children(func() {
				ui.Icon(c, icons[i]).Size(11, 11).TextColor(col)
				ui.Text(c, names[i]).FontSize(11.5).TextColor(col).SingleLine()
			})
		}
	})
}

// dataView 是「数据」标签的内容：表格与图像两种呈现。
func (a *app) dataView(c *ui.Context) {
	if !a.state.DataAvailable() {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "该对象没有数据视图").FontSize(12).TextColor(view.ColMute)
		})
		return
	}
	ui.Column(c).Fill().Children(func() {
		if a.recentExport.path != "" {
			a.exportBanner(c)
		}
		if a.dataMode == modeImage {
			view.ImageView(c, a.state)
			return
		}
		view.DataTable(c, a.state)
	})
}

// exportBanner 是导出成功后的非模态提示条：显示保存路径并提供快捷打开操作。
func (a *app) exportBanner(c *ui.Context) {
	path := a.recentExport.path
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 16).Gap(8).
		Background(view.ColPanel2)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLine) })
	bar.Children(func() {
		ui.Text(c, "✓").FontSize(12).TextColor(view.ColAccent).SingleLine()
		ui.Text(c, "已导出").FontSize(11.5).TextColor(view.ColDim).SingleLine()
		ui.Text(c, path).Font(view.FontMono).FontSize(11).TextColor(view.ColMute).
			Grow(1).SingleLine()
		view.GhostButton(c, "打开文件", func() { a.openExported(path) })
		view.GhostButton(c, "打开文件夹", func() { a.revealExported(path) })
		view.GhostButton(c, "关闭", func() { a.recentExport.path = "" })
	})
}

// errorBanner 是非模态错误提示条，可关闭且不阻塞浏览（任务 7.5）。
func (a *app) errorBanner(c *ui.Context) {
	bar := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(6, 16).Gap(8).
		Background(view.ColWarnBg)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLine) })
	bar.Children(func() {
		ui.Text(c, "⚠").FontSize(12).TextColor(view.ColAccent).SingleLine()
		ui.Text(c, a.errMsg).FontSize(11.5).TextColor(view.ColDim).
			Grow(1).SingleLine()
		view.GhostButton(c, "关闭提示", func() { a.errMsg = "" })
	})
}

// statusBar 显示文件名、当前对象路径与元素总数。
func (a *app) statusBar(c *ui.Context) {
	bar := ui.Row(c).Shrink(0).Height(statusH).AlignItems(ui.Center).
		Padding(0, 16).Gap(10).Background(view.ColPanel)
	bar.Draw(func(p *ui.Painter, r ui.Rect) { view.Hairline(p, r, view.ColLine) })
	bar.Children(func() {
		doc := a.state.Document()
		name := "（未打开文件）"
		if doc != nil {
			name = filepath.Base(doc.Path())
		}
		ui.Text(c, name).FontSize(11.5).TextColor(view.ColDim).SingleLine()
		if n := a.state.Current; n != nil {
			ui.Text(c, "·").FontSize(11.5).TextColor(view.ColFaint).SingleLine()
			ui.Text(c, view.PathText(n)).Font(view.FontMono).FontSize(11).
				TextColor(view.ColMute).SingleLine()
			if s := a.countText(n); s != "" {
				ui.Text(c, "·").FontSize(11.5).TextColor(view.ColFaint).SingleLine()
				ui.Text(c, s).FontSize(11.5).TextColor(view.ColMute).SingleLine()
			}
		}
		ui.Spacer(c).Grow(1)
		if doc != nil {
			ui.Text(c, formatSize(doc.Size())).Font(view.FontMono).FontSize(11).
				TextColor(view.ColMute).SingleLine()
		}
	})
}

// countText 描述当前对象的规模。
func (a *app) countText(n *hdf5.Node) string {
	if n.Kind == hdf5.KindDataset {
		info, err := n.Info()
		if err != nil {
			return ""
		}
		return "共 " + strconv.FormatUint(info.ElemCount, 10) + " 个元素"
	}
	if n.Kind == hdf5.KindGroup {
		gi, err := n.GroupInfo()
		if err != nil {
			return ""
		}
		return strconv.Itoa(gi.MemberCount) + " 个成员"
	}
	return ""
}

// formatSize 把字节数写成便于阅读的形式。
func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	v := float64(n)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, u := range units {
		v /= unit
		if v < unit {
			return strconv.FormatFloat(v, 'f', 1, 64) + " " + u
		}
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + " PB"
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title:           "GoHdfView",
			Width:           1440,
			Height:          900,
			MinWidth:        800,
			MinHeight:       520,
			StateKey:        "main",
			AutoHideMenuBar: true,
			Content:         ui.View(a.view),
		}
		// 隐藏系统标题栏，由深色顶栏充当标题栏：
		// Windows 避免出现浅色白条，macOS 避免出现与深色界面不协调的白色原生条。
		// 窗口按钮仍由系统绘制，按深色背景配色（macOS 上为交通灯按钮）。
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			opts.TitleBarStyle = mygo.TitleBarHidden
			opts.TitleBarHeight = topBarH
		}
		a.win = mygo.NewWindow(opts)
		a.win.SetWindowButtonsTheme(mygo.ThemeDark)
		// 窗口与任务栏图标用 logo.pen 导出的应用图标，与顶栏标记同源。
		_ = a.win.SetIcon(view.IconPNG())

		// 唯一的 view.App 注入异步调度、重绘与导出回调（design.md 决策 7）。
		a.wireState()

		// 拖放打开（任务 7.4）。
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			for _, p := range e.Paths {
				a.openFile(p)
			}
		})
		// 退出前释放句柄，退出后文件不被占用（任务 7.6）。
		a.win.OnClose(func(*mygo.CloseEvent) {
			a.closeAllTableWindows()
			for _, of := range a.files {
				_ = of.doc.Close()
			}
		})

		mygo.App.SetMenu(a.menu())

		// 命令行参数直接打开（任务 7.4）。
		for _, p := range os.Args[1:] {
			a.openFile(p)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
