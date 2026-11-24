package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 视图在没有窗口的情况下也能运行：NewTester 像用户一样点击和输入。
func TestView(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 800, 520)
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Errorf("启动后缺少空状态提示，texts %q", tt.Texts())
	}
}
