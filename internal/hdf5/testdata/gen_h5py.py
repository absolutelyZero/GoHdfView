"""生成由 C 库（h5py / HDF5 官方库）写出的夹具，用于跨实现验证。

Go 侧的 internal/hdf5/testdata/gen.go 用纯 Go 的写入实现产出夹具，
只能证明「本库能读自己写的东西」。这里用 h5py 再写一份，覆盖
官方库产出的文件结构（不同的超级块版本、对象头版本、B-tree 布局）。

用法（需要 numpy 与 h5py）：

    python internal/hdf5/testdata/gen_h5py.py

产物写在 internal/hdf5/testdata/ 下，前缀 h5py_。
"""

import os
import sys

import h5py
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))


def out(name):
    return os.path.join(HERE, name)


def make_basic():
    """二维浮点矩阵、属性、嵌套组、无限维数据集。"""
    path = out("h5py_basic.h5")
    with h5py.File(path, "w") as f:
        # 100×50 的行优先矩阵，值 = row + col/100。
        rows, cols = 100, 50
        data = np.add.outer(np.arange(rows, dtype=np.float64),
                            np.arange(cols, dtype=np.float64) / 100.0)
        ds = f.create_dataset("matrix", data=data)
        ds.attrs["units"] = "K"
        ds.attrs["scale_factor"] = np.array([1.5, 2.5], dtype=np.float64)

        # 分块 + shuffle + deflate 的数据集。
        comp = f.create_dataset("compressed", shape=(64, 32), dtype=np.float64,
                                chunks=(16, 16), shuffle=True,
                                compression="gzip", compression_opts=6)
        comp[...] = np.arange(64 * 32, dtype=np.float64).reshape(64, 32) / 7.0

        g = f.create_group("results")
        g.create_group("empty")
        run = g.create_group("run-1")
        run.create_dataset("temperature", data=np.arange(10, dtype=np.float64))

        # 可无限扩展的一维数据集：最大值为 unlimited。
        f.create_dataset("growable", shape=(5,), maxshape=(None,),
                         dtype=np.float64, data=np.arange(5, dtype=np.float64))
    return path


def make_types():
    """整数、无符号大整数、枚举、复合、变长字符串。"""
    path = out("h5py_types.h5")
    with h5py.File(path, "w") as f:
        f.create_dataset("int32", data=np.array([1, -2, 3, -4, 5], dtype=np.int32))
        f.create_dataset("int64_big", data=np.array(
            [1 << 62, -(1 << 62), 9007199254740993], dtype=np.int64))
        f.create_dataset("uint64_big", data=np.array(
            [1 << 63, 2 ** 64 - 1], dtype=np.uint64))
        f.create_dataset("float32", data=np.array([1.5, -2.25, 0.0], dtype=np.float32))

        # 枚举：七个中文成员名。先按枚举类型建数据集再赋值，
        # 因为 h5py 不接受「int8 数据 + 枚举类型」的隐式转换。
        enum_dt = h5py.enum_dtype({"周一": 0, "周二": 1, "周三": 2, "周四": 3,
                                   "周五": 4, "周六": 5, "周日": 6},
                                  basetype=np.int8)
        days = f.create_dataset("enum_days", shape=(4,), dtype=enum_dt)
        days[...] = np.array([0, 3, 6, 1], dtype=np.int8)

        # 复合类型：{id: int32, value: float64}。
        compound = np.dtype([("id", "<i4"), ("value", "<f8")])
        rows = np.array([(1, 1.5), (2, 2.5), (3, 3.5)], dtype=compound)
        f.create_dataset("compound", data=rows)

        # 变长字符串：Go 侧走 ReadVLenBytes。
        dt = h5py.string_dtype(encoding="utf-8")
        f.create_dataset("names", data=np.array(["alpha", "beta", "gamma", "delta"],
                                                dtype=object), dtype=dt)

        # 定长字符串用 numpy 的定长 bytes dtype 写：h5py 的
        # string_dtype(length=) 与 numpy 2.x 的 object 数组不兼容。
        fixed = np.dtype("S8")
        f.create_dataset("codes",
                         data=np.array([b"AA", b"BB"], dtype=fixed))

        # 标量（rank 0）。
        f.create_dataset("scalar", data=np.float64(42.5))
    return path


def make_unsupported():
    """使用无解码器的过滤器，用于验证「不支持特性降级」。"""
    path = out("h5py_unsupported.h5")
    with h5py.File(path, "w") as f:
        data = np.arange(64, dtype=np.int32).reshape(8, 8)
        # SZIP：本库没有解码器，读取应报「不支持」。
        try:
            f.create_dataset("szip", data=data, chunks=(8, 8), compression="szip")
        except Exception as exc:  # 环境里的 libhdf5 可能没编译 SZIP
            print("跳过 szip：%s" % exc, file=sys.stderr)
        # scale-offset：本库没有解码器。
        f.create_dataset("scaleoffset", data=data, chunks=(8, 8),
                         scaleoffset=2)
    return path


def make_matlab():
    """模拟 MATLAB v7.3 的布局：512 字节 user block + MATLAB_class 属性。

    真正的 MATLAB 文件由 MATLAB 写出，这里只复刻它对本阅读器有影响的
    两个特征：非零 user block 偏移（超级块不在文件开头）与
    MATLAB_class 属性。这样能验证 user block 扫描与属性显示。
    """
    path = out("h5py_matlab.h5")
    with h5py.File(path, "w", userblock_size=512) as f:
        f.attrs["MATLAB_class"] = np.bytes_("double")
        data = np.arange(12, dtype=np.float64).reshape(3, 4)
        ds = f.create_dataset("values", data=data)
        ds.attrs["MATLAB_class"] = np.bytes_("double")
        f.create_group("#refs#")
    return path


def main():
    for maker in (make_basic, make_types, make_unsupported, make_matlab):
        path = maker()
        print("%-40s %8d bytes" % (os.path.basename(path), os.path.getsize(path)))


if __name__ == "__main__":
    main()
