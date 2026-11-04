package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const ChannelStatusReasonQualityFailed = "Quality audit failed; manual recovery required"

// DisableChannelForQualityAudit disables the whole channel, including multi-key
// channels. Persist routing availability and the reason together before updating
// the local cache or allowing the caller to announce a successful disable.
func DisableChannelForQualityAudit(channelID int) (bool, error) {
	channelStatusLock.Lock()
	defer channelStatusLock.Unlock()
	pollingLock := GetChannelPollingLock(channelID)
	pollingLock.Lock()
	defer pollingLock.Unlock()

	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id", "status", "other_info").First(&channel, channelID).Error; err != nil {
			return err
		}
		if channel.Status != common.ChannelStatusEnabled {
			return nil
		}
		info := map[string]any{}
		if channel.OtherInfo != "" {
			if err := common.UnmarshalJsonStr(channel.OtherInfo, &info); err != nil {
				return err
			}
		}
		if info == nil {
			info = make(map[string]any)
		}
		info["status_reason"] = ChannelStatusReasonQualityFailed
		info["status_time"] = common.GetTimestamp()
		encoded, err := common.Marshal(info)
		if err != nil {
			return err
		}
		result := tx.Model(&Channel{}).Where("id = ? AND status = ?", channelID, common.ChannelStatusEnabled).
			Updates(map[string]any{"status": common.ChannelStatusAutoDisabled, "other_info": string(encoded)})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := tx.Model(&Ability{}).Where("channel_id = ?", channelID).Update("enabled", false).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if changed {
		CacheUpdateChannelStatus(channelID, common.ChannelStatusAutoDisabled)
	}
	return changed, nil
}
