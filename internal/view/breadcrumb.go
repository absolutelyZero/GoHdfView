package view

import (
	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// Breadcrumb 显示当前对象的完整路径，每一级是可点击的分段。
//
// 点击某一段会把该祖先展开并选中（任务 3.4）：
// 分段索引 n 对应路径的前 n+1 段，根路径单独作为第 0 段显示。
func Breadcrumb(c *ui.Context, app *App) {
	if !app.HasDocument() {
		return
	}
	cur := app.Current
	if cur == nil {
		return
	}
	segs := segments(cur.Path)
	items := make([]string, 0, len(segs)+1)
	items = append(items, "/")
	items = append(items, segs...)

	// chosen 是本帧被点击的分段；Breadcrumbs 在构建时写入。
	chosen := len(items) - 1
	ui.Breadcrumbs(c, items, &chosen).Label("对象路径").OnChange(func() {
		if chosen < 0 || chosen >= len(items) {
			return
		}
		target := "/"
		if chosen > 0 {
			target = "/" + joinSegs(segs[:chosen])
		}
		if err := app.SelectPath(target); err != nil {
			app.Err = err
		}
	})
}

// joinSegs 用 "/" 连接路径分段。
func joinSegs(segs []string) string {
	out := ""
	for i, s := range segs {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}

// PathText 返回当前对象的完整路径文本，供状态栏复用。
func PathText(n *hdf5.Node) string {
	if n == nil {
		return ""
	}
	return n.Path
}
