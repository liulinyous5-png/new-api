package operation_setting

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled  bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes  float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode         string  `json:"channel_test_mode"`
	ChannelTestConcurrency  int     `json:"channel_test_concurrency"`
	AutoQualityEnabled      bool    `json:"auto_quality_enabled"`
	AutoQualityMinutes      float64 `json:"auto_quality_minutes"`
	QualityDisableOnFailure bool    `json:"quality_disable_on_failure"`
	Quality429Retries       int     `json:"quality_429_retries"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"

	ChannelTestConcurrencyOptionKey  = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency    = 1
	MaxChannelTestConcurrency        = 32
	QualityDisableOnFailureOptionKey = "monitor_setting.quality_disable_on_failure"
	Quality429RetriesOptionKey       = "monitor_setting.quality_429_retries"
	MaxQuality429Retries             = 10
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:  false,
	AutoTestChannelMinutes:  10,
	ChannelTestMode:         ChannelTestModeScheduledAll,
	ChannelTestConcurrency:  DefaultChannelTestConcurrency,
	AutoQualityEnabled:      false,
	AutoQualityMinutes:      60,
	QualityDisableOnFailure: false,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	switch monitorSetting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery:
	default:
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	if monitorSetting.AutoQualityMinutes < MinAutoQualityMinutes || monitorSetting.AutoQualityMinutes > MaxAutoQualityMinutes {
		monitorSetting.AutoQualityMinutes = 60
	}
	return &monitorSetting
}

const (
	MinAutoQualityMinutes = 1
	MaxAutoQualityMinutes = 1440
)

func ValidateAutoQualityMinutes(value string) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < MinAutoQualityMinutes || parsed > MaxAutoQualityMinutes || parsed != math.Trunc(parsed) || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fmt.Errorf("auto quality interval must be between %d and %d minutes", MinAutoQualityMinutes, MaxAutoQualityMinutes)
	}
	return nil
}

func ValidateQuality429Retries(value string) error {
	retries, err := strconv.Atoi(value)
	if err != nil || retries < 0 || retries > MaxQuality429Retries {
		return fmt.Errorf("quality 429 retries must be an integer between 0 and 10")
	}
	return nil
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}
