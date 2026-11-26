package view

import (
	_ "embed"
	"sync"

	"github.com/egoist/mygo/ui"
)

// 顶栏标记是 ui/logo.pen 里「L1 · 透镜树」的位图版本：
// 放大镜的镜片里嵌着一棵对象树，手柄沿 45° 悬在圆环右下，留一道细缝。
//
// 两张图是同一几何的两种配色：light 用于深色顶栏，accent 留给空状态等
// 需要强调的场合。图内已按 5% 留白居中，调用方只需给定边长。
var (
	//go:embed assets/mark-light.png
	markLightPNG []byte
	//go:embed assets/mark-accent.png
	markAccentPNG []byte
	//go:embed assets/app-icon.png
	appIconPNG []byte

	markOnce              sync.Once
	markLight, markAccent *ui.Bitmap
)

// loadMarks 解码两张标记位图；解码失败时返回 nil，界面据此回退到文字标记。
func loadMarks() {
	markOnce.Do(func() {
		if b, err := ui.DecodeBitmap(markLightPNG); err == nil {
			markLight = b
		}
		if b, err := ui.DecodeBitmap(markAccentPNG); err == nil {
			markAccent = b
		}
	})
}

// Mark 返回顶栏用的浅色标记；解码失败时返回 nil。
func Mark() *ui.Bitmap {
	loadMarks()
	return markLight
}

// MarkAccent 返回琥珀色标记；解码失败时返回 nil。
func MarkAccent() *ui.Bitmap {
	loadMarks()
	return markAccent
}

// IconPNG 返回应用图标（窗口标题栏与任务栏）的 PNG 字节。
func IconPNG() []byte {
	return appIconPNG
}
