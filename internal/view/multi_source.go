package view

import (
	"gohdfview/internal/hdf5"
)

// multiSource 把多个已打开文档组合成一棵多根对象树。
// 每个文件根节点映射到自己的文档；子节点读取与元数据采集按节点所属文档路由。
// 文件集合与路由表由 main 在打开/关闭文件时增量维护。
type multiSource struct {
	roots   []*hdf5.Node
	docs    map[string]*hdf5.Document // 文件标识 → 文档句柄
	readers map[string]Reader         // 文件标识 → 读取覆盖（测试注入）
	metas   map[string]Meta           // 文件标识 → 描述覆盖（测试注入）
}

func newMultiSource() *multiSource {
	return &multiSource{
		docs:    make(map[string]*hdf5.Document),
		readers: make(map[string]Reader),
		metas:   make(map[string]Meta),
	}
}

func (m *multiSource) Roots() []*hdf5.Node { return m.roots }

func (m *multiSource) Children(n *hdf5.Node) ([]*hdf5.Node, error) {
	return n.Children()
}

func (m *multiSource) add(doc *hdf5.Document) {
	if doc == nil || doc.Root() == nil {
		return
	}
	file := doc.Root().ID.File
	m.docs[file] = doc
	m.roots = append(m.roots, doc.Root())
}

func (m *multiSource) remove(file string) {
	delete(m.docs, file)
	delete(m.readers, file)
	delete(m.metas, file)
	roots := m.roots[:0]
	for _, n := range m.roots {
		if n.ID.File != file {
			roots = append(roots, n)
		}
	}
	m.roots = roots
}

func (m *multiSource) setReader(file string, rd Reader) {
	if rd == nil {
		delete(m.readers, file)
	} else {
		m.readers[file] = rd
	}
}

func (m *multiSource) setMeta(file string, meta Meta) {
	if meta == nil {
		delete(m.metas, file)
	} else {
		m.metas[file] = meta
	}
}

// routedReader 按节点所属文件把四个读取接口分派到对应文档或注入的读取器。
type routedReader struct{ src *multiSource }

func (r routedReader) fileOf(n *hdf5.Node) string {
	if n == nil {
		return ""
	}
	return n.ID.File
}

func (r routedReader) ReadSlice(n *hdf5.Node, start, count []uint64) (*hdf5.Slice, error) {
	if rd := r.src.readers[r.fileOf(n)]; rd != nil {
		return rd.ReadSlice(n, start, count)
	}
	if doc := r.src.docs[r.fileOf(n)]; doc != nil {
		return doc.ReadSlice(n, start, count)
	}
	return nil, hdf5.ErrClosed
}

func (r routedReader) ReadStrings(n *hdf5.Node) (*hdf5.Strings, error) {
	if rd := r.src.readers[r.fileOf(n)]; rd != nil {
		return rd.ReadStrings(n)
	}
	if doc := r.src.docs[r.fileOf(n)]; doc != nil {
		return doc.ReadStrings(n)
	}
	return nil, hdf5.ErrClosed
}

func (r routedReader) ReadCompound(n *hdf5.Node) (*hdf5.Compound, error) {
	if rd := r.src.readers[r.fileOf(n)]; rd != nil {
		return rd.ReadCompound(n)
	}
	if doc := r.src.docs[r.fileOf(n)]; doc != nil {
		return doc.ReadCompound(n)
	}
	return nil, hdf5.ErrClosed
}

func (r routedReader) ReadStrided(n *hdf5.Node, rowStart, colStart, rowCount, colCount, rowStride, colStride uint64) (*hdf5.Slice, error) {
	if rd := r.src.readers[r.fileOf(n)]; rd != nil {
		return rd.ReadStrided(n, rowStart, colStart, rowCount, colCount, rowStride, colStride)
	}
	if doc := r.src.docs[r.fileOf(n)]; doc != nil {
		return doc.ReadStrided(n, rowStart, colStart, rowCount, colCount, rowStride, colStride)
	}
	return nil, hdf5.ErrClosed
}

// routedMeta 按节点所属文件分派元数据采集。
type routedMeta struct{ src *multiSource }

func (m routedMeta) Info(n *hdf5.Node) (*hdf5.DatasetInfo, error) {
	file := ""
	if n != nil {
		file = n.ID.File
	}
	if meta := m.src.metas[file]; meta != nil {
		return meta.Info(n)
	}
	return n.Info()
}
