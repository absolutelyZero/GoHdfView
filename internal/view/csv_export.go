package view

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"gohdfview/internal/hdf5"
)

// CSVExportScope 选择导出的范围。
type CSVExportScope int

const (
	// CSVScopePage 只导出表格当前页。
	CSVScopePage CSVExportScope = iota
	// CSVScopeAll 分块导出整个数据集。
	CSVScopeAll
)

// CSVWriteFunc 把一个快照的 CSV 内容写到 w。
// 由 view 提供、由注入的导出处理器调用，便于 view 与窗口/对话框解耦。
type CSVWriteFunc func(w io.Writer) error

// sliceRequest 按 requestFor 的规则构造任意行/列块的读取请求。
func sliceRequest(l TableLayout, rs, cs, rc, cc int) (start, count []uint64) {
	if l.Scalar {
		return nil, nil
	}
	if l.Rank == 1 {
		return []uint64{uint64(rs)}, []uint64{uint64(rc)}
	}
	start = append([]uint64(nil), l.Leading...)
	count = make([]uint64, len(l.Leading))
	for i := range count {
		count[i] = 1
	}
	start = append(start, uint64(rs), uint64(cs))
	count = append(count, uint64(rc), uint64(cc))
	return
}

// writeCSVRegion 写一个矩形区域的 CSV：可选列号表头，随后每行以行号开头。
func writeCSVRegion(cw *csv.Writer, l TableLayout, node *hdf5.Node, rd Reader, dt hdf5.DatatypeInfo, rs, cs, rc, cc int, header bool) error {
	if header && cc > 0 {
		h := make([]string, 0, cc+1)
		h = append(h, "#")
		for j := 0; j < cc; j++ {
			h = append(h, strconv.Itoa(cs+j))
		}
		if err := cw.Write(h); err != nil {
			return err
		}
	}
	if rc <= 0 || cc <= 0 {
		return nil
	}
	start, count := sliceRequest(l, rs, cs, rc, cc)
	slice, err := rd.ReadSlice(node, start, count)
	if err != nil {
		return err
	}
	if slice == nil {
		return fmt.Errorf("读取结果为空")
	}
	for i := 0; i < rc; i++ {
		row := make([]string, 0, cc+1)
		row = append(row, strconv.Itoa(rs+i))
		for j := 0; j < cc; j++ {
			idx := i*cc + j
			if idx < 0 || idx >= len(slice.Values) {
				row = append(row, "—")
				continue
			}
			row = append(row, formatNumber(dt, slice.Values[idx]))
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return nil
}

// writeCSVAll 分块读取整个数据集并逐行写出，内存仅与一个行块成正比。
func writeCSVAll(cw *csv.Writer, l TableLayout, node *hdf5.Node, rd Reader, dt hdf5.DatatypeInfo) error {
	h := make([]string, 0, l.Cols+1)
	h = append(h, "#")
	for j := 0; j < l.Cols; j++ {
		h = append(h, strconv.Itoa(j))
	}
	if err := cw.Write(h); err != nil {
		return err
	}
	for rs := 0; rs < l.Rows; rs += defaultPageRows {
		rc := min(defaultPageRows, l.Rows-rs)
		rows := make([][]string, rc)
		for i := range rows {
			rows[i] = []string{strconv.Itoa(rs + i)}
		}
		for cs := 0; cs < l.Cols; cs += defaultPageCols {
			cc := min(defaultPageCols, l.Cols-cs)
			start, count := sliceRequest(l, rs, cs, rc, cc)
			slice, err := rd.ReadSlice(node, start, count)
			if err != nil {
				return err
			}
			if slice == nil {
				return fmt.Errorf("读取结果为空")
			}
			for i := 0; i < rc; i++ {
				for j := 0; j < cc; j++ {
					idx := i*cc + j
					v := "—"
					if idx >= 0 && idx < len(slice.Values) {
						v = formatNumber(dt, slice.Values[idx])
					}
					rows[i] = append(rows[i], v)
				}
			}
		}
		for _, row := range rows {
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	return nil
}

// csvWrite 在用户点击导出时对当前状态拍快照，返回可稍后执行的写入函数。
func (a *App) csvWrite(l TableLayout, scope CSVExportScope) CSVWriteFunc {
	sl := l
	sl.Leading = append([]uint64(nil), l.Leading...)
	node := a.Current
	rd := a.reader()
	dt := a.datatype()
	rs, rc, cs, cc := a.pageWindow(l)
	return func(w io.Writer) error {
		cw := csv.NewWriter(w)
		var err error
		if scope == CSVScopeAll {
			err = writeCSVAll(cw, sl, node, rd, dt)
		} else {
			err = writeCSVRegion(cw, sl, node, rd, dt, rs, cs, rc, cc, true)
		}
		if err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	}
}
