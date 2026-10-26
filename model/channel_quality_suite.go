/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type ChannelQualitySuite struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name" gorm:"size:128;uniqueIndex"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

type ChannelQualitySuiteItem struct {
	Id      int    `json:"id" gorm:"primaryKey"`
	SuiteId int    `json:"suite_id" gorm:"index;uniqueIndex:idx_cq_suite_item,priority:1"`
	ProbeId int    `json:"probe_id" gorm:"uniqueIndex:idx_cq_suite_item,priority:2"`
	Model   string `json:"model" gorm:"size:255;uniqueIndex:idx_cq_suite_item,priority:3"`
	Sort    int    `json:"sort"`
}

type ChannelQualitySuiteRun struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	ChannelId  int    `json:"channel_id" gorm:"index:idx_cqsr_channel_created,priority:1"`
	SuiteId    int    `json:"suite_id"`
	SuiteName  string `json:"suite_name" gorm:"size:128"`
	Verdict    string `json:"verdict" gorm:"size:32"`
	Error      string `json:"error" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index:idx_cqsr_channel_created,priority:2"`
	FinishedAt int64  `json:"finished_at" gorm:"bigint"`
}

func ListChannelQualitySuites() ([]*ChannelQualitySuite, error) {
	var suites []*ChannelQualitySuite
	err := DB.Order("id asc").Find(&suites).Error
	return suites, err
}

func GetChannelQualitySuite(id int) (*ChannelQualitySuite, error) {
	suite := &ChannelQualitySuite{}
	err := DB.First(suite, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return suite, nil
}

func ListChannelQualitySuiteItems(suiteID int) ([]*ChannelQualitySuiteItem, error) {
	var items []*ChannelQualitySuiteItem
	err := DB.Where("suite_id = ?", suiteID).Order("sort asc, id asc").Find(&items).Error
	return items, err
}

func SaveChannelQualitySuite(suite *ChannelQualitySuite, items []*ChannelQualitySuiteItem) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		if suite.Id == 0 {
			suite.CreatedAt = now
			suite.UpdatedAt = now
			if err := tx.Create(suite).Error; err != nil {
				return err
			}
		} else {
			suite.UpdatedAt = now
			result := tx.Model(&ChannelQualitySuite{}).Where("id = ?", suite.Id).Updates(map[string]any{
				"name":       suite.Name,
				"updated_at": suite.UpdatedAt,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errors.New("quality suite not found")
			}
			if err := tx.Where("suite_id = ?", suite.Id).Delete(&ChannelQualitySuiteItem{}).Error; err != nil {
				return err
			}
		}
		for index, item := range items {
			item.Id = 0
			item.SuiteId = suite.Id
			item.Sort = index
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

func DeleteChannelQualitySuite(id int) error {
	var references int64
	if err := DB.Model(&Channel{}).Where("quality_suite_id = ?", id).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return errors.New("suite is used by a channel")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("suite_id = ?", id).Delete(&ChannelQualitySuiteItem{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&ChannelQualitySuite{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("quality suite not found")
		}
		return nil
	})
}

func ChannelQualitySuiteNameTaken(name string, excludeID int) (bool, error) {
	var count int64
	query := DB.Model(&ChannelQualitySuite{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func ChannelQualityInProgress(channelID int) (bool, error) {
	var count int64
	err := DB.Model(&ChannelQualityRun{}).
		Where("channel_id = ? AND verdict = ?", channelID, ChannelQualityVerdictRunning).
		Count(&count).Error
	if err != nil || count > 0 {
		return count > 0, err
	}
	err = DB.Model(&ChannelQualitySuiteRun{}).
		Where("channel_id = ? AND verdict = ?", channelID, ChannelQualityVerdictRunning).
		Count(&count).Error
	return count > 0, err
}

func ExpireInterruptedQualitySuites() error {
	var runs []*ChannelQualitySuiteRun
	if err := DB.Where("verdict = ?", ChannelQualityVerdictRunning).Find(&runs).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for _, run := range runs {
		if err := DB.Model(&ChannelQualityRun{}).
			Where("suite_run_id = ? AND verdict = ?", run.Id, ChannelQualityVerdictRunning).
			Updates(map[string]any{
				"verdict": ChannelQualityVerdictError,
				"error":   "quality scan was interrupted",
			}).Error; err != nil {
			return err
		}
		if err := DB.Model(run).Updates(map[string]any{
			"verdict":     ChannelQualityVerdictError,
			"error":       "quality scan was interrupted",
			"finished_at": now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func ListQualityAutoChannels() ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where(
		"quality_auto_enabled = ? AND quality_suite_id > ? AND status = ?",
		true,
		0,
		common.ChannelStatusEnabled,
	).Order("id asc").Find(&channels).Error
	return channels, err
}

func SaveChannelQualitySchedule(id int, enabled bool, suiteID int) error {
	return DB.Model(&Channel{}).Where("id = ?", id).UpdateColumns(map[string]any{
		"quality_auto_enabled": enabled,
		"quality_suite_id":     suiteID,
	}).Error
}

func CreateChannelQualitySuiteRun(run *ChannelQualitySuiteRun) error {
	if run.CreatedAt == 0 {
		run.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(run).Error
}

func SaveChannelQualitySuiteRun(run *ChannelQualitySuiteRun) error {
	return DB.Save(run).Error
}

func GetLatestChannelQualitySuiteRun(channelID int) (*ChannelQualitySuiteRun, []*ChannelQualityRun, error) {
	run := &ChannelQualitySuiteRun{}
	err := DB.Where("channel_id = ?", channelID).Order("id desc").First(run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var items []*ChannelQualityRun
	err = DB.Where("suite_run_id = ?", run.Id).Order("id asc").Find(&items).Error
	if err != nil {
		return nil, nil, err
	}
	return run, items, nil
}
