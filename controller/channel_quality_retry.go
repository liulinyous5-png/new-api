package controller

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const qualityAuditTimeout = 60 * time.Second

// Stored inside the existing probe snapshot so old audit records need no migration.
type qualityExecution struct {
	Attempts   int    `json:"attempts"`
	RetryLimit int    `json:"retry_limit"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	StopReason string `json:"stop_reason,omitempty"`
}

type qualityProbeResponse struct {
	status     int
	body       []byte
	retryAfter string
	err        error
}

// All attempts share the caller's deadline and already-selected credential.
// send must close its response before returning, including on read errors.
func executeQualityRequests(ctx context.Context, retries int, statuses []int, send func() qualityProbeResponse) (response qualityProbeResponse, execution qualityExecution) {
	execution.RetryLimit = min(max(retries, 0), operation_setting.MaxQuality429Retries)
	started := time.Now()
	defer func() { execution.ElapsedMS = time.Since(started).Milliseconds() }()
	for {
		if err := ctx.Err(); err != nil {
			response.err = err
			return response, execution
		}
		execution.Attempts++
		response = send()
		if response.err != nil || response.status != http.StatusTooManyRequests || slices.Contains(statuses, http.StatusTooManyRequests) {
			return response, execution
		}
		if execution.Attempts > execution.RetryLimit {
			execution.StopReason = "retry_limit"
			return response, execution
		}
		delay := qualityRetryDelay(response.retryAfter, execution.Attempts, time.Now())
		if deadline, ok := ctx.Deadline(); ok && delay >= time.Until(deadline) {
			execution.StopReason = "time_budget"
			return response, execution
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			response.err = ctx.Err()
			return response, execution
		case <-timer.C:
		}
	}
}

func qualityRetryDelay(retryAfter string, attempt int, now time.Time) time.Duration {
	base := time.Second * time.Duration(1<<min(max(attempt, 1), 4))
	delay := base + time.Duration(rand.Float64()*float64(base)/5)
	value := strings.TrimSpace(retryAfter)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		// Anything beyond the per-probe budget cannot be retried. Clamp before
		// converting to Duration to prevent an upstream-controlled overflow.
		return max(delay, time.Duration(min(seconds, uint64(qualityAuditTimeout/time.Second)+1))*time.Second)
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(delay, date.Sub(now))
	}
	return delay
}
