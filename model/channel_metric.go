package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// ChannelMetric is a cumulative snapshot owned by one collector process. Its
// opaque ID includes that process's nonce and all bucket dimensions, so retries
// overwrite the same snapshot and different instances never overwrite each other.
// These operational metrics live in the primary DB, independently of log storage.
type ChannelMetric struct {
	ID           string `gorm:"size:64;primaryKey"`
	ChannelID    int    `gorm:"index:idx_channel_metric_window,priority:1"`
	BucketTs     int64  `gorm:"index:idx_channel_metric_window,priority:2;index:idx_channel_metric_expiry"`
	ModelName    string `gorm:"type:text"`
	Kind         string `gorm:"size:24"`
	FirstAttempt bool
	Result       string `gorm:"size:24"`
	Count        int64
	UpdatedAt    int64 `gorm:"autoUpdateTime:false"`
}

func SaveChannelMetrics(ctx context.Context, rows []ChannelMetric) error {
	if len(rows) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
	}).CreateInBatches(&rows, 100).Error
}

type ChannelMetricTotal struct {
	ChannelID    int
	ModelName    string
	Kind         string
	FirstAttempt bool
	Result       string
	Count        int64
	UpdatedAt    int64
}

func GetChannelMetricTotals(ctx context.Context, ids []int, start, end int64) ([]ChannelMetricTotal, error) {
	rows := []ChannelMetricTotal{}
	if len(ids) == 0 {
		return rows, nil
	}
	// Apply object authorization inside the data query too. A caller-supplied
	// channel ID can never reveal statistics for an inaccessible/deleted channel.
	visible := ChannelScopeFromContext(ctx).Apply(DB.Model(&Channel{})).Select("id").Where("id IN ?", ids)
	modelName := "model_name"
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// Model identifiers are case sensitive even when the database's
		// default collation is case/accent insensitive.
		modelName = "BINARY model_name"
	}
	err := DB.WithContext(ctx).Model(&ChannelMetric{}).
		Select("channel_id, "+modelName+" AS model_name, kind, first_attempt, result, SUM(count) AS count, MAX(updated_at) AS updated_at").
		Where("channel_id IN (?) AND bucket_ts >= ? AND bucket_ts <= ?", visible, start, end).
		Group("channel_id, " + modelName + ", kind, first_attempt, result").Scan(&rows).Error
	return rows, err
}
