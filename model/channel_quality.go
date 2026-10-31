package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	ChannelQualityVerdictRunning     = "running"
	ChannelQualityVerdictPassed      = "passed"
	ChannelQualityVerdictFailed      = "failed"
	ChannelQualityVerdictUnknown     = "unknown"
	ChannelQualityVerdictError       = "error"
	ChannelQualityVerdictUnsupported = "unsupported"
	ChannelQualityVerdictSkipped     = "skipped"

	ChannelQualityProbeNameClearToolUses = "clear_tool_uses"
	ChannelQualityModelPlaceholder       = "{{model}}"
	MaxChannelQualityProbeBodyBytes      = 2 * 1024 * 1024
)

type ChannelQualityAssertion struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
}

type ChannelQualityProbe struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	Name       string `json:"name" gorm:"size:128;uniqueIndex"`
	Builtin    bool   `json:"builtin"`
	Headers    string `json:"headers" gorm:"type:text"`
	Body       string `json:"body" gorm:"type:text"`
	Statuses   string `json:"statuses" gorm:"type:text"`
	Assertions string `json:"assertions" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

type ChannelQualityRun struct {
	Id               int    `json:"id" gorm:"primaryKey"`
	ChannelId        int    `json:"channel_id" gorm:"index:idx_cqr_channel_created,priority:1"`
	KeyIndex         int    `json:"key_index"`
	RequestedModel   string `json:"requested_model" gorm:"size:255"`
	UpstreamModel    string `json:"upstream_model" gorm:"size:255"`
	ProbeId          int    `json:"probe_id"`
	ProbeName        string `json:"probe_name" gorm:"size:128"`
	ProbeSnapshot    string `json:"probe_snapshot" gorm:"type:text"`
	Verdict          string `json:"verdict" gorm:"size:32"`
	AssertionResults string `json:"assertion_results" gorm:"type:text"`
	ActualEdits      string `json:"actual_edits" gorm:"type:text"`
	UsageJSON        string `json:"usage_json" gorm:"type:text"`
	Error            string `json:"error" gorm:"type:text"`
	SuiteRunId       int    `json:"suite_run_id" gorm:"index"`
	SuiteItemId      int    `json:"suite_item_id"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_cqr_channel_created,priority:2"`
}

func (probe *ChannelQualityProbe) AssertionList() ([]ChannelQualityAssertion, error) {
	if probe == nil || strings.TrimSpace(probe.Assertions) == "" {
		return nil, nil
	}
	var assertions []ChannelQualityAssertion
	if err := common.Unmarshal([]byte(probe.Assertions), &assertions); err != nil {
		return nil, err
	}
	return assertions, nil
}

func (probe *ChannelQualityProbe) StatusList() ([]int, error) {
	if probe == nil || strings.TrimSpace(probe.Statuses) == "" {
		return []int{200}, nil
	}
	var statuses []int
	if err := common.Unmarshal([]byte(probe.Statuses), &statuses); err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return []int{200}, nil
	}
	return statuses, nil
}

func (probe *ChannelQualityProbe) HeaderMap() (map[string]string, error) {
	headers := map[string]string{}
	if probe == nil || strings.TrimSpace(probe.Headers) == "" {
		return headers, nil
	}
	if err := common.Unmarshal([]byte(probe.Headers), &headers); err != nil {
		return nil, err
	}
	return headers, nil
}

func ChannelQualityProbeNameTaken(name string, excludeID int) (bool, error) {
	var count int64
	query := DB.Model(&ChannelQualityProbe{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func ListChannelQualityProbes() ([]*ChannelQualityProbe, error) {
	var probes []*ChannelQualityProbe
	err := DB.Order("builtin desc, id asc").Find(&probes).Error
	return probes, err
}

func GetChannelQualityProbe(id int) (*ChannelQualityProbe, error) {
	probe := &ChannelQualityProbe{}
	err := DB.First(probe, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return probe, nil
}

func CreateChannelQualityProbe(probe *ChannelQualityProbe) error {
	now := common.GetTimestamp()
	probe.CreatedAt = now
	probe.UpdatedAt = now
	probe.Builtin = false
	return DB.Create(probe).Error
}

func UpdateChannelQualityProbe(probe *ChannelQualityProbe) error {
	probe.UpdatedAt = common.GetTimestamp()
	result := DB.Model(&ChannelQualityProbe{}).Where("id = ? AND builtin = ?", probe.Id, false).Updates(map[string]any{
		"name":       probe.Name,
		"headers":    probe.Headers,
		"body":       probe.Body,
		"statuses":   probe.Statuses,
		"assertions": probe.Assertions,
		"updated_at": probe.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("probe not found")
	}
	return nil
}

func DeleteChannelQualityProbe(id int) error {
	var references int64
	if err := DB.Model(&ChannelQualitySuiteItem{}).Where("probe_id = ?", id).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return errors.New("probe is used by a quality suite")
	}
	result := DB.Where("id = ? AND builtin = ?", id, false).Delete(&ChannelQualityProbe{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("probe not found")
	}
	return nil
}

func CreateChannelQualityRun(run *ChannelQualityRun) error {
	if run.CreatedAt == 0 {
		run.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(run).Error
}

func SaveChannelQualityRun(run *ChannelQualityRun) error {
	return saveChannelQualityRun(DB, run)
}

func saveChannelQualityRun(tx *gorm.DB, run *ChannelQualityRun) error {
	return tx.Model(&ChannelQualityRun{}).Where("id = ?", run.Id).Updates(map[string]any{
		"probe_name":        run.ProbeName,
		"requested_model":   run.RequestedModel,
		"upstream_model":    run.UpstreamModel,
		"probe_snapshot":    run.ProbeSnapshot,
		"verdict":           run.Verdict,
		"assertion_results": run.AssertionResults,
		"actual_edits":      run.ActualEdits,
		"usage_json":        run.UsageJSON,
		"error":             run.Error,
	}).Error
}

func ListChannelQualityRuns(channelID int, offset int, limit int) ([]*ChannelQualityRun, int, error) {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 10
	}
	query := DB.Model(&ChannelQualityRun{}).Where("channel_id = ?", channelID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var runs []*ChannelQualityRun
	err := DB.Where("channel_id = ?", channelID).
		Order("id desc").
		Offset(offset).
		Limit(limit).
		Find(&runs).Error
	return runs, int(total), err
}

func GetLatestChannelQualityRun(channelID int) (*ChannelQualityRun, error) {
	run := &ChannelQualityRun{}
	err := DB.Where("channel_id = ?", channelID).Order("id desc").First(run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

type ChannelQualityLatest struct {
	ChannelId int    `json:"channel_id"`
	Verdict   string `json:"verdict"`
	CreatedAt int64  `json:"created_at"`
}

func ListLatestChannelQualityRuns() ([]ChannelQualityLatest, error) {
	// Read only the summary fields, using two batched queries regardless of the
	// number of channels. A suite's final child is not its aggregate verdict.
	var runs []ChannelQualityRun
	latestRunIDs := DB.Model(&ChannelQualityRun{}).Select("MAX(id)").Group("channel_id")
	if err := DB.Select("channel_id", "verdict", "created_at", "suite_run_id").
		Where("id IN (?)", latestRunIDs).Find(&runs).Error; err != nil {
		return nil, err
	}
	var suites []ChannelQualitySuiteRun
	latestSuiteIDs := DB.Model(&ChannelQualitySuiteRun{}).Select("MAX(id)").Group("channel_id")
	if err := DB.Select("id", "channel_id", "verdict", "created_at").
		Where("id IN (?)", latestSuiteIDs).Find(&suites).Error; err != nil {
		return nil, err
	}
	latest := make([]ChannelQualityLatest, 0, len(runs))
	indices := make(map[int]int, len(runs))
	suiteIDs := make(map[int]int, len(runs))
	for _, run := range runs {
		indices[run.ChannelId] = len(latest)
		suiteIDs[run.ChannelId] = run.SuiteRunId
		latest = append(latest, ChannelQualityLatest{
			ChannelId: run.ChannelId,
			Verdict:   run.Verdict,
			CreatedAt: run.CreatedAt,
		})
	}
	for _, suite := range suites {
		index, exists := indices[suite.ChannelId]
		if exists && suiteIDs[suite.ChannelId] != suite.Id && latest[index].CreatedAt >= suite.CreatedAt {
			continue
		}
		item := ChannelQualityLatest{ChannelId: suite.ChannelId, Verdict: suite.Verdict, CreatedAt: suite.CreatedAt}
		if exists {
			latest[index] = item
		} else {
			latest = append(latest, item)
		}
	}
	return latest, nil
}
