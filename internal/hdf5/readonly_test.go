package hdf5

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 只读是本项目的硬约束：应用代码不得调用任何写入接口。
//
// 这条断言用源码扫描实现，而不是靠「没有 import 写入函数」的约定：
// 解析库把读与写放在同一个包里，任何人加一行 CreateForWrite 都能编译通过，
// 只有静态扫描能在评审前把这类改动拦下来。
//
// 扫描范围是仓库里所有参与常规构建的 Go 文件；夹具生成脚本带
// //go:build ignore，不在扫描范围内——它是唯一被允许写文件的例外，
// 且只写 testdata/ 下的夹具。

// forbiddenCalls 是解析库的写入接口。命中的调用一律视为违反只读约束。
//
// 只列「只可能是写入」的名字：Write、Create、Close 这类通用名在标准库与
// MyGo 里都有合法用法，逐个列入会制造大量误报，因此不在这里。
var forbiddenCalls = []string{
	"CreateForWrite",
	"OpenForWrite",
	"CreateGroupWithLinks",
	"CreateDenseGroup",
	"CreateCompound",
	"CreateSoftLink",
	"CreateHardLink",
	"CreateExternalLink",
	"WriteRaw",
	"WriteAttribute",
	"DeleteAttribute",
}

func TestNoWriteAPIInSource(t *testing.T) {
	root := filepath.Join("..", "..")
	files, err := collectGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("没有扫描到任何 Go 文件，扫描逻辑有问题")
	}

	var violations []string
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		if hasIgnoreBuildTag(string(src)) {
			continue // 夹具生成脚本允许写
		}
		found, err := scanForWrites(path, src)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		violations = append(violations, found...)
	}

	if len(violations) > 0 {
		t.Fatalf("只读约束被违反，发现写入接口调用：\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestReadonlyScanDetectsViolation 是上面那条断言的自检：
// 扫描器必须能在人为构造的源码里抓到写入调用，否则前面的「通过」没有意义。
func TestReadonlyScanDetectsViolation(t *testing.T) {
	src := []byte(`package sample

import libhdf5 "github.com/scigolib/hdf5"

func bad() {
	fw, _ := libhdf5.CreateForWrite("x.h5", libhdf5.CreateTruncate)
	_ = fw
}
`)
	found, err := scanForWrites("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("扫描器未能识别 CreateForWrite，只读断言不可信")
	}
	if !strings.Contains(found[0], "CreateForWrite") {
		t.Fatalf("违规项应指明具体接口，实际: %v", found)
	}
}

// TestReadonlyScanAllowsReads 确认扫描器不会把读接口误判为违规。
func TestReadonlyScanAllowsReads(t *testing.T) {
	src := []byte(`package sample

import libhdf5 "github.com/scigolib/hdf5"

func good() {
	f, _ := libhdf5.Open("x.h5")
	_ = f.Root()
	_, _ = f.Root().Attributes()
}
`)
	found, err := scanForWrites("sample.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("读接口不应被判为违规，实际: %v", found)
	}
}

// collectGoFiles 收集 root 下参与构建的 Go 文件，跳过缓存、构建产物与
// 带 //go:build ignore 的脚本所在目录之外的一切非源码目录。
func collectGoFiles(root string) ([]string, error) {
	var out []string
	skipDirs := map[string]bool{
		".git":     true,
		".probe":   true,
		"build":    true,
		".mygo":    true,
		".dsh":     true,
		"openspec": true,
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		// 跳过测试文件自身：它包含被扫描的接口名。
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// hasIgnoreBuildTag 判断源码是否带 //go:build ignore。
func hasIgnoreBuildTag(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			if trimmed == "//go:build ignore" {
				return true
			}
			continue
		}
		// 构建约束必须出现在第一条非注释、非空行之前。
		break
	}
	return false
}

// scanForWrites 在源码里查找写入接口的调用，返回人类可读的违规列表。
func scanForWrites(path string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}

	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name
		for _, forbidden := range forbiddenCalls {
			if name == forbidden {
				out = append(out, fmtPos(fset, call.Pos(), name))
				return true
			}
		}
		return true
	})
	return out, nil
}

// fmtPos 生成 "文件:行: 接口名" 形式的违规描述。
func fmtPos(fset *token.FileSet, pos token.Pos, name string) string {
	p := fset.Position(pos)
	return p.String() + ": " + name
}
