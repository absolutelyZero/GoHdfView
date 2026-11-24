package view

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gohdfview/internal/hdf5"
)

// TestTreeWithRealFixture 用真实夹具验证任务 3.1 的标签与维度概要。
func TestTreeWithRealFixture(t *testing.T) {
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", "basic.h5"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	// 根组展开后应看到 /matrix 与 /results。
	if !tt.HasText("matrix") {
		t.Fatalf("应显示 matrix，实际 %q", tt.Texts())
	}
	if !tt.HasText("results") {
		t.Fatalf("应显示 results，实际 %q", tt.Texts())
	}
	if !tt.HasText("[100×50]") {
		t.Fatalf("数据集标签后应附维度概要 [100×50]，实际 %q", tt.Texts())
	}

	// 选中 /matrix 后当前对象应更新。
	if err := app.SelectPath("/matrix"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()
	if app.Current == nil || app.Current.Path != "/matrix" {
		t.Fatalf("当前对象应为 /matrix，实际 %+v", app.Current)
	}
}

// TestTreeRealFixtureNested 验证嵌套组按需加载。
func TestTreeRealFixtureNested(t *testing.T) {
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", "basic.h5"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	tt := ui.NewTester(testView(app), 900, 600)
	tt.Frame()

	if err := app.SelectPath("/results/run-1/temperature"); err != nil {
		t.Fatalf("SelectPath: %v", err)
	}
	tt.Frame()

	for _, seg := range []string{"results", "run-1", "temperature"} {
		if !tt.HasText(seg) {
			t.Fatalf("面包屑与树应包含 %q，实际 %q", seg, tt.Texts())
		}
	}
	if app.Current == nil || app.Current.Path != "/results/run-1/temperature" {
		t.Fatalf("当前对象应为 /results/run-1/temperature，实际 %+v", app.Current)
	}
	// 空组不应有展开箭头：它没有子节点。
	if ids := app.children(app.nodes[app.roots[0]].ID); len(ids) == 0 {
		t.Fatalf("根组应有成员")
	}
}

// TestEmptyGroupHasNoArrow 对应 object-tree 的「空组」scenario：
// 空组的 children 返回 nil，Outline 因此不画展开箭头。
func TestEmptyGroupHasNoArrow(t *testing.T) {
	doc, err := hdf5.Open(filepath.Join("..", "hdf5", "testdata", "basic.h5"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer doc.Close()

	app := NewApp()
	if err := app.SetDocument(doc); err != nil {
		t.Fatalf("SetDocument: %v", err)
	}
	empty, err := app.find("/results/empty")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if empty.Kind != hdf5.KindGroup {
		t.Fatalf("/results/empty 应为组，实际 %v", empty.Kind)
	}
	if ids := app.children(empty.ID); ids != nil {
		t.Fatalf("空组的 children 应返回 nil（不显示箭头），实际 %v", ids)
	}
}
