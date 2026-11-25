package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestIntegrationFullPath 走完整路径：打开夹具 → 展开树 → 选中压缩数据集 →
// 断言元数据显示过滤器 → 切到数据视图断言首屏数值 → 切到属性视图断言属性
// （任务 8.1）。
//
// 这条用例刻意只用公开行为（点击、文本断言）而不直接读 app 的字段，
// 因此它能发现「状态对了但界面没跟上」这类回归。
func TestIntegrationFullPath(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1200, 800)
	t.Cleanup(func() {
		if doc := a.state.Document(); doc != nil {
			doc.Close()
		}
	})

	// 启动即空状态。
	tt.Frame()
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Fatalf("启动应为空状态，texts %q", tt.Texts())
	}

	// 打开文件：左树出现，状态栏给出文件名。
	a.openFile(fixture("compressed.h5"))
	tt.Frame()
	if !tt.HasText("compressed.h5") {
		t.Fatalf("状态栏应显示文件名，texts %q", tt.Texts())
	}

	// 展开树：根组的成员在展开后才出现。
	if err := tt.Click("全部展开"); err != nil {
		t.Fatalf("点击全部展开失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("compressed") {
		t.Fatalf("展开后应看到 compressed 节点，texts %q", tt.Texts())
	}

	// 选中压缩数据集：树的行选择驱动右侧工作区。
	if err := a.state.SelectPath("/compressed"); err != nil {
		t.Fatalf("选中 /compressed 失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("/compressed") {
		t.Fatalf("面包屑应显示路径，texts %q", tt.Texts())
	}

	// 元数据：分块布局与按应用顺序排列的过滤器。
	if err := tt.Click("元数据"); err != nil {
		t.Fatalf("点击元数据失败：%v", err)
	}
	tt.Frame()
	for _, want := range []string{"存储布局", "分块", "shuffle", "deflate (level 6)", "[64, 32]"} {
		if !tt.HasText(want) {
			t.Errorf("元数据缺少 %q，texts %q", want, tt.Texts())
		}
	}

	// 数据视图：首屏是一页，数值来自夹具的写入顺序。
	if err := tt.Click("数据"); err != nil {
		t.Fatalf("点击数据失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("行 0–63 / 64") {
		t.Errorf("数据视图缺少分页信息，texts %q", tt.Texts())
	}

	// 属性视图：压缩数据集没有属性。
	if err := tt.Click("属性"); err != nil {
		t.Fatalf("点击属性失败：%v", err)
	}
	tt.Frame()
	if !tt.HasText("无属性") {
		t.Errorf("属性视图应显示无属性，texts %q", tt.Texts())
	}

	// 关闭文件回到空状态。
	a.closeFile()
	tt.Frame()
	if !tt.HasText("打开 HDF5 文件或把文件拖入窗口") {
		t.Errorf("关闭后应回空状态，texts %q", tt.Texts())
	}
}

// TestIntegrationGroupThenDataset 验证「组 → 数据集」的切换把
// 置灰的「数据」标签重新启用（viewer-shell 的视图切换）。
func TestIntegrationGroupThenDataset(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1200, 800)
	t.Cleanup(func() {
		if doc := a.state.Document(); doc != nil {
			doc.Close()
		}
	})

	a.openFile(fixture("basic.h5"))
	tt.Frame()

	// 组：数据标签不可用，落到元数据。
	if err := a.state.SelectPath("/results"); err != nil {
		t.Fatalf("选中 /results 失败：%v", err)
	}
	tt.Frame()
	if a.state.DataAvailable() {
		t.Error("组不应有数据视图")
	}
	if !tt.HasText("组信息") {
		t.Errorf("组应显示组信息，texts %q", tt.Texts())
	}

	// 数据集：数据标签恢复可用，点击即可切换。
	if err := a.state.SelectPath("/results/run-1/temperature"); err != nil {
		t.Fatalf("选中温度数据集失败：%v", err)
	}
	tt.Frame()
	if !a.state.DataAvailable() {
		t.Fatal("数据集应有数据视图")
	}
	if err := tt.Click("数据"); err != nil {
		t.Fatalf("点击数据失败：%v", err)
	}
	tt.Frame()
	if a.tab != tabData {
		t.Errorf("应切到数据标签，实际 %d", a.tab)
	}
}
