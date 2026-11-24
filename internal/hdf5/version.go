package hdf5

// LibraryName 与 LibraryVersion 标识本层依赖的 HDF5 解析库。
//
// 解析库没有导出自己的版本常量，所以这里以常量形式记录
// go.mod 中固定的版本；升级依赖时同步修改，供「关于」与
// 验收文档引用（viewer-shell 的「关于」scenario）。
const (
	LibraryName    = "github.com/scigolib/hdf5"
	LibraryVersion = "v0.15.0"
)
