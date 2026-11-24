// Package hdf5 是 GoHdfView 的只读 HDF5 访问层。
//
// 本层把 github.com/scigolib/hdf5 的读接口包装成面向视图的值类型，
// 并把解析库返回的错误归入本包定义的四个哨兵错误，
// 供上层决定提示文案与降级策略。
//
// 只读是本层的硬约束：包内不调用任何写入接口（见 readonly_test.go）。
package hdf5

import (
	"errors"
)

// 本层对外暴露的错误分类。上层用 errors.Is 判断，
// 不依赖解析库错误消息的文本。
var (
	// ErrNotHDF5 表示文件不是 HDF5 格式（签名不匹配、空文件、user block 之外无超级块）。
	ErrNotHDF5 = errors.New("不是 HDF5 文件")

	// ErrCorrupt 表示文件是 HDF5 格式，但结构损坏到无法解析。
	ErrCorrupt = errors.New("HDF5 文件已损坏")

	// ErrUnsupported 表示文件合法，但使用了本工具或解析库尚未实现的特性，
	// 例如 SZIP / N-bit / Scale-offset 过滤器、外部链接、硬链接环。
	ErrUnsupported = errors.New("HDF5 特性不受支持")

	// ErrTooLarge 表示请求读取的数据量超过配置的单次读取上限。
	ErrTooLarge = errors.New("读取量超过上限")

	// ErrOutOfBounds 表示切片请求越出数据集边界。
	ErrOutOfBounds = errors.New("切片请求越界")

	// ErrClosed 表示文档已关闭，后续读取不再可用。
	ErrClosed = errors.New("文档已关闭")

	// ErrNotFound 表示按路径找不到对象。
	ErrNotFound = errors.New("对象不存在")

	// ErrNotDataset 表示对非数据集节点请求数据视图内容。
	ErrNotDataset = errors.New("对象不是数据集")

	// ErrNotGroup 表示对非组节点请求组成员信息。
	ErrNotGroup = errors.New("对象不是组")
)
