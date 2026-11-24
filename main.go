package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
	"gohdfview/internal/view"
)

// appVersion 是应用自身的版本，与 mygo.json 的 version 保持一致。
const appVersion = "0.1.0"

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

// app 是窗口要展示的状态。视图在主线程上按帧从它构建界面：
// 输入之后、Window.Update 之后，以及动画进行时。
//
// 跨帧存活的字段只放在这里；控件本身是 build-pass 的临时值，
// 不能存进结构体（见 design.md 决策 1 与 9）。
type app struct {
	// state 是对象树、表格、图像等视图状态，全部在 internal/view 里。
	state *view.App
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
}

// newApp 创建空状态的应用。
//
// 这里不注入异步调度器：测试用同步读取以便逐帧断言，
// 真实窗口在 main 里再装上工作 goroutine。
func newApp() *app {
	return &app{
		state:    view.NewApp(),
		split:    320,
		tab:      tabData,
		dataMode: modeTable,
	}
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

// openFile 以只读方式打开文件；失败时保留当前文档。
func (a *app) openFile(path string) {
	if path == "" {
		return
	}
	doc, err := hdf5.Open(path)
	if err != nil {
		// 打不开：当前文档不动，只在提示条上说明原因。
		a.setError(err)
		return
	}
	// 先释放旧句柄再换新文档，避免两个文件同时被占用（任务 7.6）。
	if old := a.state.Document(); old != nil {
		old.Close()
	}
	if err := a.state.SetDocument(doc); err != nil {
		doc.Close()
		a.setError(err)
		return
	}
	a.errMsg = ""
	a.tab = tabData
	a.dataMode = modeTable
	a.refreshTitle()
}

// closeFile 释放句柄并回到空状态（Ctrl+W / 文件菜单）。
func (a *app) closeFile() {
	if doc := a.state.Document(); doc != nil {
		doc.Close()
	}
	if err := a.state.SetDocument(nil); err != nil {
		a.setError(err)
	}
	a.errMsg = ""
	a.tab = tabData
	a.dataMode = modeTable
	a.refreshTitle()
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
			Parent: a.win,
			Title:  "打开 HDF5 文件",
			Filters: []mygo.FileFilter{
				{Name: "HDF5 文件", Extensions: []string{"h5", "hdf5", "he5", "mat"}},
				{Name: "所有文件", Extensions: []string{"*"}},
			},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		if a.win == nil {
			a.openFile(path)
			return
		}
		a.win.Update(func() { a.openFile(path) })
	}()
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

// view 是窗口内容：空状态，或「左树 / 右工作区 + 状态栏」。
func (a *app) view(c *ui.Context) {
	if !a.state.HasDocument() {
		a.emptyState(c)
		return
	}
	ui.Column(c).Fill().Children(func() {
		ui.Split(c, &a.split,
			func() { a.sidebar(c) },
			func() { a.workspace(c) }).Grow(1)
		if a.errMsg != "" {
			a.errorBanner(c)
		}
		ui.Divider(c)
		a.statusBar(c)
	})
}

// emptyState 是没有打开文件时的界面。
func (a *app) emptyState(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Padding(24).Children(func() {
		ui.Text(c, "GoHdfView").FontSize(22).Bold()
		ui.Text(c, "打开 HDF5 文件或把文件拖入窗口").TextColor(c.Theme().TextMuted)
		ui.Button(c, "打开…").OnClick(func() { a.openDialog() })
		if a.errMsg != "" {
			ui.Column(c).Width(520).Gap(6).Children(func() {
				ui.Text(c, a.errMsg).FontSize(12).TextColor(c.Theme().Warning)
				ui.Button(c, "关闭提示").OnClick(func() { a.errMsg = "" })
			})
		}
	})
}

// sidebar 是左栏：树操作按钮与对象树。
func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Gap(6).Padding(8).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "对象").Bold().FontSize(13).Grow(1)
			ui.Button(c, "全部展开").OnClick(func() {
				a.state.ExpandAll()
			})
			ui.Button(c, "全部折叠").OnClick(func() {
				a.state.CollapseAll()
			})
		})
		view.Tree(c, a.state)
	})
}

// workspace 是右栏：路径、标签页与当前视图。
func (a *app) workspace(c *ui.Context) {
	// 选中的对象没有数据视图时，「数据」标签置灰并自动落到「元数据」。
	if a.tab == tabData && !a.state.DataAvailable() {
		a.tab = tabMetadata
	}
	ui.Column(c).Fill().Gap(8).Padding(12).Children(func() {
		view.Breadcrumb(c, a.state)
		a.tabBar(c)
		ui.Divider(c)
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
}

// tabBar 画三个视图标签。
//
// 用 TabsBase 而不是 Tabs：只有前者能把单个标签设为不可用，
// 而「数据」标签在选中组时必须置灰（viewer-shell 的视图切换）。
func (a *app) tabBar(c *ui.Context) {
	names := []string{"数据", "元数据", "属性"}
	tabs := ui.TabsBase(c, &a.tab, len(names))
	tabs.List.Gap(4).Children(func() {
		for i, name := range names {
			tab := tabs.Tab(i).Padding(6, 12).Radius(6)
			if i == a.tab {
				tab.Background(c.Theme().Surface)
			}
			if i == tabData && !a.state.DataAvailable() {
				tab.Disabled(true)
			}
			tab.Children(func() {
				ui.Text(c, name)
			})
		}
	})
}

// dataView 是「数据」标签的内容：表格与图像两种呈现。
func (a *app) dataView(c *ui.Context) {
	if !a.state.DataAvailable() {
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, "该对象没有数据视图").TextColor(c.Theme().TextMuted)
		})
		return
	}
	ui.Column(c).Fill().Gap(6).Children(func() {
		a.dataModeBar(c)
		ui.Column(c).Fill().Grow(1).Children(func() {
			if a.dataMode == modeImage {
				view.ImageView(c, a.state)
				return
			}
			view.DataTable(c, a.state)
		})
	})
}

// dataModeBar 在表格与图像之间切换。
//
// 图像不可用时不禁用该标签：选中后由图像视图说明为什么不可用，
// 比一个点不动的灰色标签更有用（dataset-data-view 的图像 scenario）。
func (a *app) dataModeBar(c *ui.Context) {
	names := []string{"表格", "图像"}
	tabs := ui.TabsBase(c, &a.dataMode, len(names))
	tabs.List.Gap(4).Children(func() {
		for i, name := range names {
			tab := tabs.Tab(i).Padding(4, 10).Radius(6)
			if i == a.dataMode {
				tab.Background(c.Theme().Surface)
			}
			tab.Children(func() {
				ui.Text(c, name).FontSize(12)
			})
		}
	})
}

// errorBanner 是非模态错误提示条，可关闭且不阻塞浏览（任务 7.5）。
func (a *app) errorBanner(c *ui.Context) {
	ui.Row(c).Gap(8).Padding(6, 12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "⚠").TextColor(c.Theme().Warning)
		ui.Text(c, a.errMsg).FontSize(12).Grow(1).SingleLine()
		ui.Button(c, "关闭提示").OnClick(func() { a.errMsg = "" })
	})
}

// statusBar 显示文件名、当前对象路径与元素总数。
func (a *app) statusBar(c *ui.Context) {
	ui.Row(c).Gap(12).Padding(6, 12).AlignItems(ui.Center).Children(func() {
		doc := a.state.Document()
		name := "（未打开文件）"
		if doc != nil {
			name = filepath.Base(doc.Path())
		}
		ui.Text(c, name).FontSize(12).SingleLine()
		if n := a.state.Current; n != nil {
			ui.Text(c, view.PathText(n)).FontSize(12).
				TextColor(c.Theme().TextMuted).SingleLine()
			ui.Text(c, a.countText(n)).FontSize(12).
				TextColor(c.Theme().TextMuted).SingleLine()
		}
		ui.Spacer(c)
		if doc != nil {
			ui.Text(c, formatSize(doc.Size())).FontSize(12).
				TextColor(c.Theme().TextMuted).SingleLine()
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
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "GoHdfView",
			Width:     1200,
			Height:    800,
			MinWidth:  800,
			MinHeight: 520,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})

		// 大切片在工作 goroutine 上读，结果经 Window.Update 写回，
		// 避免读取期间界面不响应（design.md 决策 4）。
		a.state.SetAsync(func(work func()) { go work() })
		a.state.SetInvalidate(func() { a.win.Invalidate() })

		// 拖放打开（任务 7.4）。
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			if len(e.Paths) > 0 {
				a.openFile(e.Paths[0])
			}
		})
		// 退出前释放句柄，退出后文件不被占用（任务 7.6）。
		a.win.OnClose(func(*mygo.CloseEvent) {
			if doc := a.state.Document(); doc != nil {
				doc.Close()
			}
		})

		mygo.App.SetMenu(a.menu())

		// 命令行参数直接打开（任务 7.4）。
		if len(os.Args) > 1 {
			a.openFile(os.Args[1])
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
