// Package modelmerge 承载"基础模型 + 资源类型注册块 → 完整授权模型 DSL"的
// 重建式合并（Phase C 改造②从 RegisterResourceType logic 抽公共）：
// 注册表即源头配置（permission-design §6.3"从注册配置任意重生成皆安全"），
// 注册路径（logic）与启动重放路径（svc）共用同一合并函数，保证两条路径
// 产出的模型 DSL 完全一致、可相互归一。
package modelmerge

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadBase 读取合并基准模型 DSL：Engine.ModelFile 指向的基础模型文件
// （空缺省 model.fga，与 bootstrap 同约定——工作目录为 services/authz）。
// 读取失败返回错误，由调用方按 503/启动失败语义处置（无基准模型绝不写新版本）。
func ReadBase(modelFile string) (string, error) {
	path := strings.TrimSpace(modelFile)
	if path == "" {
		path = "model.fga"
	}
	dsl, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取基础模型(%s): %w", path, err)
	}
	return string(dsl), nil
}

// MergeBlocks 重建式合并：基准模型 + 全部已注册类型块，块按类型名升序拼接
//（块首行固定为 "type <名>"），保证同输入同输出（确定性）。
//
// FGA DSL 语法约束：condition 声明只能位于全部 type 声明之后，而基准模型
// v3 起文件末尾带 condition 块——注册块必须插在 condition 之前，否则模型
// 非法（"mismatched input 'type' expecting <EOF>"）。这里在基准模型首个
// 顶层 condition 行前插入注册块；无 condition 时按原语义追加到文件尾。
func MergeBlocks(base string, blocks []string) string {
	sorted := make([]string, len(blocks))
	copy(sorted, blocks)
	SortBlocks(sorted)
	joined := strings.Join(sorted, "\n\n")
	if idx := firstConditionIndex(base); idx >= 0 {
		return strings.TrimSpace(base[:idx]) + "\n\n" + joined + "\n\n" + strings.TrimSpace(base[idx:]) + "\n"
	}
	return strings.TrimSpace(base) + "\n\n" + joined + "\n"
}

// firstConditionIndex 返回基准模型中首个顶层 condition 声明的行首下标
//（列 0 起始的 "condition " 行）；不存在返回 -1。
func firstConditionIndex(dsl string) int {
	offset := 0
	for _, line := range strings.Split(dsl, "\n") {
		if strings.HasPrefix(line, "condition ") {
			return offset
		}
		offset += len(line) + 1
	}
	return -1
}

// SortBlocks 就地按类型名升序排序 DSL 块（合并输出确定性的统一兜底）。
func SortBlocks(blocks []string) {
	sort.Slice(blocks, func(i, j int) bool {
		return BlockTypeName(blocks[i]) < BlockTypeName(blocks[j])
	})
}

// BlockTypeName 从 DSL 块首行 "type <名>" 提取类型名（合并排序键）。
func BlockTypeName(block string) string {
	line := strings.TrimSpace(strings.SplitN(block, "\n", 2)[0])
	return strings.TrimSpace(strings.TrimPrefix(line, "type"))
}
