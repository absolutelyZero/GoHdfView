package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app 是窗口要展示的状态。视图在主线程上按帧从它构建界面：
// 输入之后、Window.Update 之后，以及动画进行时。
//
// 跨帧存活的字段只放在这里；控件本身是 build-pass 的临时值，
// 不能存进结构体（见 design.md 决策 1 与 9）。
type app struct{}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Padding(24).Children(func() {
		ui.Text(c, "GoHdfView").FontSize(22).Bold()
		ui.Text(c, "打开 HDF5 文件或把文件拖入窗口").TextColor(c.Theme().TextMuted)
	})
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "GoHdfView",
			Width:     1200,
			Height:    800,
			MinWidth:  800,
			MinHeight: 520,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
