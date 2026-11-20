package perfmetrics

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/google/uuid"
)

const channelBucketSeconds int64 = 60

type channelBucketKey struct {
	channelID int
	model     string
	kind      string
	first     bool
	result    string
	ts        int64
}

type channelBucket struct {
	row   model.ChannelMetric
	saved int64
}

// Only the collector writes snapshots. Queries use committed database rows,
// never DB + Redis + local copies of the same counters.
var channelCollector = struct {
	sync.Mutex
	flushMu sync.Mutex
	nonce   string
	buckets map[channelBucketKey]*channelBucket
}{nonce: uuid.NewString(), buckets: make(map[channelBucketKey]*channelBucket)}

func RecordChannelAttempt(ctx context.Context, channelID int, info *relaycommon.RelayInfo, first bool, kind string, apiErr *types.NewAPIError) {
	if info == nil || channelID <= 0 || !perf_metrics_setting.GetSetting().Enabled {
		return
	}
	result := classifyChannelAttempt(ctx, info, apiErr)
	now := time.Now().Unix()
	key := channelBucketKey{channelID, info.OriginModelName, kind, first, result, now - now%channelBucketSeconds}
	channelCollector.Lock()
	defer channelCollector.Unlock()
	bucket := channelCollector.buckets[key]
	if bucket == nil {
		id := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%q:%q:%t:%s:%d", channelCollector.nonce, key.channelID, key.model, key.kind, key.first, key.result, key.ts)))
		bucket = &channelBucket{row: model.ChannelMetric{ID: fmt.Sprintf("%x", id), ChannelID: channelID, BucketTs: key.ts, ModelName: key.model, Kind: kind, FirstAttempt: first, Result: result}}
		channelCollector.buckets[key] = bucket
	}
	bucket.row.Count++
	bucket.row.UpdatedAt = now
}

// RecordMidjourneyAttempt samples submission responses, excluding task polling.
// The caller supplies the existing acceptance decision (including queue codes).
func RecordMidjourneyAttempt(ctx context.Context, channelID int, modelName string, accepted bool, response *dto.MidjourneyResponseWithStatusCode, requestErr error) {
	var apiErr *types.NewAPIError
	switch {
	case requestErr != nil:
		code := types.ErrorCodeBadResponse
		if response != nil && response.Response.Description == string(types.ErrorCodeDoRequestFailed) {
			code = types.ErrorCodeDoRequestFailed
		}
		apiErr = types.NewError(requestErr, code)
	case response == nil:
		apiErr = types.NewError(errors.New("empty Midjourney response"), types.ErrorCodeEmptyResponse)
	case !accepted:
		apiErr = types.NewOpenAIError(errors.New(response.Response.Description), types.ErrorCode(fmt.Sprint(response.Response.Code)), response.StatusCode)
	}
	RecordChannelAttempt(ctx, channelID, &relaycommon.RelayInfo{OriginModelName: modelName}, true, "task_submit", apiErr)
}

// FlushChannelMetrics persists pending counters, including during graceful shutdown.
func FlushChannelMetrics() error {
	channelCollector.flushMu.Lock()
	defer channelCollector.flushMu.Unlock()
	channelCollector.Lock()
	rows := make([]model.ChannelMetric, 0, len(channelCollector.buckets))
	for _, bucket := range channelCollector.buckets {
		if bucket.row.Count != bucket.saved {
			rows = append(rows, bucket.row)
		}
	}
	channelCollector.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := model.SaveChannelMetrics(ctx, rows); err != nil {
		return err
	}
	channelCollector.Lock()
	defer channelCollector.Unlock()
	current := time.Now().Unix()
	current -= current % channelBucketSeconds
	for _, row := range rows {
		key := channelBucketKey{row.ChannelID, row.ModelName, row.Kind, row.FirstAttempt, row.Result, row.BucketTs}
		bucket := channelCollector.buckets[key]
		bucket.saved = row.Count
	}
	for key, bucket := range channelCollector.buckets {
		if key.ts < current && bucket.saved == bucket.row.Count {
			delete(channelCollector.buckets, key)
		}
	}
	return nil
}

func channelMetricsLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastCleanup := int64(0)
	for now := range ticker.C {
		if err := FlushChannelMetrics(); err != nil {
			common.SysError(common.LogText("failed to flush channel metrics: %v", err))
		}
		if now.Unix()-lastCleanup >= 3600 {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := model.DB.WithContext(ctx).Where("bucket_ts < ?", now.Add(-8*24*time.Hour).Unix()).Delete(&model.ChannelMetric{}).Error
			cancel()
			if err != nil {
				common.SysError(common.LogText("failed to expire channel metrics: %v", err))
			} else {
				lastCleanup = now.Unix()
			}
		}
	}
}

type ChannelCounts struct {
	Success          int64            `json:"success"`
	Failure          int64            `json:"failure"`
	Excluded         int64            `json:"excluded"`
	FirstSuccess     int64            `json:"first_success"`
	FirstFailure     int64            `json:"first_failure"`
	SuccessRate      *float64         `json:"success_rate"`
	FirstSuccessRate *float64         `json:"first_success_rate"`
	Results          map[string]int64 `json:"results"`
}

func (counts *ChannelCounts) add(row model.ChannelMetricTotal) {
	if counts.Results == nil {
		counts.Results = make(map[string]int64)
	}
	counts.Results[row.Result] += row.Count
	switch row.Result {
	case "success":
		counts.Success += row.Count
		if row.FirstAttempt {
			counts.FirstSuccess += row.Count
		}
	case "cancelled", "business_rejected", "unknown":
		counts.Excluded += row.Count
	default:
		counts.Failure += row.Count
		if row.FirstAttempt {
			counts.FirstFailure += row.Count
		}
	}
	if total := counts.Success + counts.Failure; total > 0 {
		rate := math.Round(float64(counts.Success)/float64(total)*10000) / 100
		counts.SuccessRate = &rate
	}
	if total := counts.FirstSuccess + counts.FirstFailure; total > 0 {
		rate := math.Round(float64(counts.FirstSuccess)/float64(total)*10000) / 100
		counts.FirstSuccessRate = &rate
	}
}

type ChannelModelMetric struct {
	Model string `json:"model"`
	Kind  string `json:"kind"`
	ChannelCounts
}

type ChannelMetricSummary struct {
	ChannelID int   `json:"channel_id"`
	UpdatedAt int64 `json:"updated_at"`
	ChannelCounts
	Models []ChannelModelMetric `json:"models"`
}

type ChannelMetricsResult struct {
	Enabled     bool                   `json:"enabled"`
	WindowStart int64                  `json:"window_start"`
	WindowEnd   int64                  `json:"window_end"`
	Channels    []ChannelMetricSummary `json:"channels"`
}

func QueryChannelMetrics(ctx context.Context, ids []int, minutes int) (ChannelMetricsResult, error) {
	now := time.Now().Unix()
	// Include the current partial minute and preceding complete minutes. The
	// returned boundaries describe the actual interval represented by the buckets.
	result := ChannelMetricsResult{Enabled: perf_metrics_setting.GetSetting().Enabled, WindowStart: now - now%channelBucketSeconds - int64(minutes)*60 + channelBucketSeconds, WindowEnd: now, Channels: []ChannelMetricSummary{}}
	rows, err := model.GetChannelMetricTotals(ctx, ids, result.WindowStart, now)
	if err != nil {
		return result, err
	}
	byChannel := map[int]*ChannelMetricSummary{}
	for _, row := range rows {
		channel := byChannel[row.ChannelID]
		if channel == nil {
			channel = &ChannelMetricSummary{ChannelID: row.ChannelID, Models: []ChannelModelMetric{}}
			byChannel[row.ChannelID] = channel
		}
		channel.UpdatedAt = max(channel.UpdatedAt, row.UpdatedAt)
		channel.add(row)
		index := slices.IndexFunc(channel.Models, func(m ChannelModelMetric) bool { return m.Model == row.ModelName && m.Kind == row.Kind })
		if index < 0 {
			channel.Models = append(channel.Models, ChannelModelMetric{Model: row.ModelName, Kind: row.Kind})
			index = len(channel.Models) - 1
		}
		channel.Models[index].add(row)
	}
	for _, channel := range byChannel {
		slices.SortFunc(channel.Models, func(a, b ChannelModelMetric) int {
			return strings.Compare(a.Model+"\x00"+a.Kind, b.Model+"\x00"+b.Kind)
		})
		result.Channels = append(result.Channels, *channel)
	}
	slices.SortFunc(result.Channels, func(a, b ChannelMetricSummary) int { return a.ChannelID - b.ChannelID })
	return result, nil
}
