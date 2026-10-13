package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// mysqlModelVersionRepo 是 ModelVersionRepo 的 MySQL 实现：model_versions 表
// （见 migrations/001_biz.sql）。主键改造：id 为 BIGINT 自增主键（DB 内部序号，
// 不暴露给业务）；engine_model_id 为 NOT NULL UNIQUE KEY，存 OpenFGA 引擎返回的
// 字符串 modelID，应用层经此列与引擎侧状态对应、与引擎 modelID 比对版本。
type mysqlModelVersionRepo struct {
	db *sql.DB
}

// Save 保存模型版本：id 列不传（DB 自增生成），engine_model_id 必填（引擎返回的
// modelID），按 engine_model_id 唯一键判定新建或幂等覆盖（ON DUPLICATE KEY
// UPDATE），填充 CreatedAt 与 create_by/update_by。原 mv-<unixnano> 字符串
// ID 生成已下线。
func (m *mysqlModelVersionRepo) Save(ctx context.Context, mv ModelVersion) error {
	if mv.EnginemodelID == "" {
		return errors.New("Save ModelVersion: EnginemodelID 不能为空")
	}
	if mv.CreatedAt.IsZero() {
		mv.CreatedAt = time.Now()
	}
	now := time.Now()
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO model_versions (engine_model_id, dsl, create_by, update_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE dsl = VALUES(dsl), update_by = VALUES(update_by),
		                          created_at = VALUES(created_at), updated_at = VALUES(updated_at)`,
		mv.EnginemodelID, mv.Dsl, mv.CreateBy, mv.UpdateBy, mv.CreatedAt, now)
	if err != nil {
		return fmt.Errorf("save model version %s: %w", mv.EnginemodelID, err)
	}
	return nil
}

// Latest 取最新模型版本：按（created_at, id）倒序取第一条——DATETIME 秒精度
// 下同秒保存的多条用 id（BIGINT 自增，单调递增）破序；无记录返回 ErrNotFound。
func (m *mysqlModelVersionRepo) Latest(ctx context.Context) (ModelVersion, error) {
	row := m.db.QueryRowContext(ctx,
		"SELECT id, engine_model_id, dsl, created_at FROM model_versions ORDER BY created_at DESC, id DESC LIMIT 1")
	var mv ModelVersion
	if err := row.Scan(&mv.ID, &mv.EnginemodelID, &mv.Dsl, &mv.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ModelVersion{}, ErrNotFound
		}
		return ModelVersion{}, fmt.Errorf("latest model version: %w", err)
	}
	return mv, nil
}
