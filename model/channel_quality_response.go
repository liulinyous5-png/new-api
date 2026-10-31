package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const MaxChannelQualityResponseBytes = 1 << 20

// ChannelQualityResponse is separate from run summaries so list queries and
// system task results never read or serialize the upstream payload.
type ChannelQualityResponse struct {
	RunId      int    `json:"run_id" gorm:"primaryKey;autoIncrement:false"`
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body_base64" gorm:"size:1048576"`
	Truncated  bool   `json:"truncated"`
	Incomplete bool   `json:"incomplete"`
}

// SaveChannelQualityResult commits the verdict and its evidence together.
func SaveChannelQualityResult(run *ChannelQualityRun, response *ChannelQualityResponse) error {
	if response == nil {
		return SaveChannelQualityRun(run)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := saveChannelQualityRun(tx, run); err != nil {
			return err
		}
		response.RunId = run.Id
		// Do not put a potentially large/private response into SQL error logs.
		return tx.Session(&gorm.Session{Logger: logger.Discard}).Clauses(clause.OnConflict{UpdateAll: true}).Create(response).Error
	})
}

func GetChannelQualityResponse(channelID, runID int) (*ChannelQualityResponse, error) {
	// Scope the evidence by its parent channel before reading any payload.
	var run ChannelQualityRun
	if err := DB.Select("id").Where("id = ? AND channel_id = ?", runID, channelID).First(&run).Error; err != nil {
		return nil, err
	}
	var response ChannelQualityResponse
	err := DB.First(&response, "run_id = ?", runID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &response, nil
}
