#!/usr/bin/env python3
"""契约类型一致性校验：对比两个 types.go 的类型集合与字段序列。

用途（Makefile contract-check）：auth/authz 的 types.go 允许保留手写注释
（goctl 生成不复制字段注释），字段对齐也可能不同（手写未对齐 vs 生成已对齐）。
本脚本忽略注释与空白，只对比：类型名集合、每个类型的字段名 + json tag + 顺序。

用法: check-types.py <real_types.go> <gen_types.go>
退出码: 0 一致；1 不一致（输出差异清单）。
"""

import re
import sys


def parse(path):
    """解析 types.go，返回 {类型名: [(字段名, tag), ...]}。"""
    src = open(path, encoding="utf-8").read()
    types = {}
    for m in re.finditer(r"^type (\w+) struct \{(.*?)^}|^type (\w+) struct\{\}", src, re.M | re.S):
        if m.group(1):
            name = m.group(1)
            fields = []
            body = m.group(2) or ""
        else:
            name = m.group(3)
            fields = []
            body = ""
        for line in body.splitlines():
            line = line.strip()
            if not line or line.startswith("//"):
                continue
            fm = re.match(r"^(\w+)\s+\S+(.*)$", line)
            if not fm:
                continue
            tag = ""
            tm = re.search(r"`([^`]+)`", fm.group(2))
            if tm:
                tag = tm.group(1)
            fields.append((fm.group(1), tag))
        types[name] = fields
    return types


def main():
    if len(sys.argv) != 3:
        print("用法: check-types.py <real> <gen>", file=sys.stderr)
        return 2
    real = parse(sys.argv[1])
    gen = parse(sys.argv[2])
    diffs = []
    if any(k is None for k in set(real) | set(gen)):
        print("real None:", [k for k in real if k is None], file=sys.stderr)
        print("gen None:", [k for k in gen if k is None], file=sys.stderr)
    for name in sorted(set(real) | set(gen)):
        if name not in real:
            diffs.append(f"- 类型 {name} 仅存在于生成版")
        elif name not in gen:
            diffs.append(f"+ 类型 {name} 仅存在于真实代码")
        elif real[name] != gen[name]:
            diffs.append(f"* 类型 {name} 字段序列不一致:")
            for i, (a, b) in enumerate(zip(real[name], gen[name]), 1):
                if a != b:
                    diffs.append(f"    第{i}个字段: 真实={a} 生成={b}")
            if len(real[name]) != len(gen[name]):
                diffs.append(
                    f"    字段数: 真实={len(real[name])} 生成={len(gen[name])}"
                )
    if diffs:
        print("types 契约不一致:")
        print("\n".join(diffs))
        return 1
    print("types 契约一致")
    return 0


if __name__ == "__main__":
    sys.exit(main())
