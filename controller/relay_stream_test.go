package controller

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRelayKeepAliveDuringImagePreparation(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	previousCount, previousMedia := constant.CountToken, constant.GetMediaToken
	fetchSettings := system_setting.GetFetchSetting()
	previousFetchSettings := *fetchSettings
	fetchSettings.AllowPrivateIp = true
	fetchSettings.AllowedPorts = []string{"1-65535"}
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	constant.CountToken, constant.GetMediaToken = true, true
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
		constant.CountToken, constant.GetMediaToken = previousCount, previousMedia
		*fetchSettings = previousFetchSettings
	})
	service.InitHttpClient()

	imageRequested := make(chan struct{})
	releaseImage := make(chan struct{})
	var releaseOnce sync.Once
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(imageRequested)
		<-releaseImage
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer imageServer.Close()
	engine := gin.New()
	engine.POST("/v1/messages", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-sonnet-4-20250514")
		Relay(c, types.RelayFormatClaude)
	})
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(releaseImage) })

	body := `{"model":"claude-sonnet-4-20250514","max_tokens":16,"stream":true,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"` + imageServer.URL + `"}}]}]}`
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err, "the gateway must send keepalive while its image download is blocked")
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	reader := bufio.NewReader(response.Body)
	for range 2 {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, ": PING\n", line)
		line, readErr = reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, "\n", line)
	}
	select {
	case <-imageRequested:
	default:
		t.Fatal("the request never reached image preparation")
	}
	releaseOnce.Do(func() { close(releaseImage) })
	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(remaining), "event: error\n")
	require.Contains(t, string(remaining), `"type":"error"`)
	require.Contains(t, string(remaining), "data: ")
}

func TestRelayKeepAliveAcrossUpstreamRetries(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
	})
	service.InitHttpClient()
	var attempts atomic.Int32
	retryStarted, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		close(retryStarted)
		select {
		case <-release:
			_, _ = io.WriteString(w, "data: ok\n\n")
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	engine := gin.New()
	engine.POST("/v1/messages", func(c *gin.Context) {
		stop := helper.StartStreamKeepAlive(c, 10*time.Millisecond)
		defer stop()
		if !assert.NoError(t, http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5*time.Second))) {
			return
		}
		info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}
		for range 2 {
			request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, strings.NewReader("{}"))
			if !assert.NoError(t, err) {
				return
			}
			response, err := channel.DoRequest(c, request, info)
			if !assert.NoError(t, err) {
				return
			}
			if response.StatusCode == http.StatusTooManyRequests {
				response.Body.Close()
				continue
			}
			_, _ = io.Copy(c.Writer, response.Body)
			response.Body.Close()
		}
	})
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(release) })
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer response.Body.Close()
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("the upstream retry did not start")
	}
	reader := bufio.NewReader(response.Body)
	for range 2 {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, ": PING\n", line)
		line, readErr = reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, "\n", line)
	}
	releaseOnce.Do(func() { close(release) })
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(body), "data: ok\n\n")
	require.Equal(t, int32(2), attempts.Load())
}

func TestRelayKeepAlivePreservesValidationErrors(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = true
	t.Cleanup(func() { settings.PingIntervalEnabled = previousEnabled })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-20250514","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	Relay(c, types.RelayFormatClaude)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.NotContains(t, recorder.Body.String(), ": PING")
}

func TestStreamKeepAlivePreservesFastHTTPFailures(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 60
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
	})
	service.InitHttpClient()
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"request rejected"}`)
			}))
			defer upstream.Close()
			engine := gin.New()
			engine.POST("/v1/messages", func(c *gin.Context) {
				stop := helper.StartStreamKeepAlive(c, time.Minute)
				defer stop()
				request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, strings.NewReader("{}"))
				if !assert.NoError(t, err) {
					return
				}
				info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}
				response, err := channel.DoRequest(c, request, info)
				if !assert.NoError(t, err) {
					return
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if assert.NoError(t, err) {
					c.Header("Content-Type", "application/json")
					c.Data(response.StatusCode, "application/json", body)
				}
			})
			gateway := httptest.NewServer(engine)
			defer gateway.Close()
			client := &http.Client{Timeout: 4 * time.Second}
			response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader("{}"))
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, status, response.StatusCode)
			require.Equal(t, "application/json", response.Header.Get("Content-Type"))
			require.JSONEq(t, `{"error":"request rejected"}`, string(body))
		})
	}
}

func TestRelayKeepAliveHonorsStreamSettings(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled := settings.PingIntervalEnabled
	previousCount, previousMedia, previousNonStreamMedia := constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream
	constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = true, true, true
	t.Cleanup(func() {
		settings.PingIntervalEnabled = previousEnabled
		constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = previousCount, previousMedia, previousNonStreamMedia
	})
	for _, test := range []struct {
		name    string
		enabled bool
		stream  bool
	}{
		{name: "ping disabled", enabled: false, stream: true},
		{name: "non-streaming", enabled: true, stream: false},
		{name: "fast streaming failure", enabled: true, stream: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings.PingIntervalEnabled = test.enabled
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := `{"model":"claude-sonnet-4-20250514","max_tokens":16,"stream":` + strconv.FormatBool(test.stream) + `,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"!"}}]}]}`
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-sonnet-4-20250514")
			Relay(c, types.RelayFormatClaude)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			require.NotContains(t, recorder.Body.String(), ": PING")
			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "error", response["type"])
		})
	}
}

func TestRelayStreamErrorAfterKeepAlive(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil)
			helper.SetEventStreamHeaders(c)
			require.NoError(t, helper.PingData(c))
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
			common.SetContextKey(c, constant.ContextKeyUpstreamRequestOrigin, "https://provider.example")
			apiError := types.NewErrorWithStatusCode(errors.New("upstream rejected input at https://provider.example/v1/messages"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
			writeRelayStreamError(c, format, apiError)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.Contains(t, recorder.Body.String(), "upstream rejected input")
			require.NotContains(t, recorder.Body.String(), "provider.example")
			status, ok := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
			require.True(t, ok)
			require.True(t, status.ResponseFailed(), "stream errors must not consume the successful-request rate limit")
			require.Equal(t, http.StatusBadRequest, status.OutcomeSnapshot().ErrorStatus)
			if format == types.RelayFormatClaude {
				require.Contains(t, recorder.Body.String(), "event: error\n")
			} else {
				require.Contains(t, recorder.Body.String(), "data: [DONE]")
			}
		})
	}
}

func TestStreamKeepAliveCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil).WithContext(requestContext)
	stop := helper.StartStreamKeepAlive(c, time.Hour)
	cancel()
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("keepalive did not stop after client cancellation")
	}
	_, err := c.Writer.WriteString("data: done\n\n")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(recorder.Body.String(), "data: done\n\n"))
}

func TestClaudeStreamRetryBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTimeout, previousTTFT := constant.StreamingTimeout, constant.TTFTTimeoutSeconds
	settings := operation_setting.GetGeneralSetting()
	previousPing := settings.PingIntervalEnabled
	previousRetry := operation_setting.AutomaticRetryStatusCodeRanges
	constant.StreamingTimeout, constant.TTFTTimeoutSeconds = 30, 0
	settings.PingIntervalEnabled = false
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 503}}
	t.Cleanup(func() {
		constant.StreamingTimeout, constant.TTFTTimeoutSeconds = previousTimeout, previousTTFT
		settings.PingIntervalEnabled = previousPing
		operation_setting.AutomaticRetryStatusCodeRanges = previousRetry
	})
	const start = "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_A","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
	const ping = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	const partial = "event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\nevent: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}` + "\n\n"
	const failure = "event: error\ndata: " + `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low"},"provider_meta":{"trace_id":"trace_A"}}` + "\n\n"
	const completion = "event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	for _, tc := range []struct {
		name, prefix                                  string
		commentPing, convertedStart, malformed, retry bool
	}{
		{name: "error before message stays internal", retry: true},
		{name: "keepalive does not prevent retry", commentPing: true, retry: true},
		{name: "upstream ping does not prevent retry", prefix: ping, retry: true},
		{name: "native message start prevents retry", prefix: start},
		{name: "converted message start prevents retry", convertedStart: true},
		{name: "partial content prevents retry", prefix: start + partial},
		{name: "malformed chunk after message prevents retry", prefix: start, malformed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-test"}}
			if tc.commentPing {
				helper.SetEventStreamHeaders(c)
				require.NoError(t, helper.PingData(c))
			}
			if tc.convertedStart {
				helper.SetEventStreamHeaders(c)
				require.NoError(t, openai.HandleStreamFormat(c, info, `{"id":"chatcmpl_A","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`, false, false))
			}
			failureData := failure
			if tc.malformed {
				failureData = "data: {broken JSON\n\n"
			}
			upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.prefix + failureData))}
			_, apiErr := claude.ClaudeStreamHandler(c, upstream, info)
			require.NotNil(t, apiErr)
			// Attempt state can be reset; the downstream response remains committed.
			info.StreamStatus = nil
			require.Equal(t, tc.retry, service.ShouldRetryRelayError(c, apiErr, 1))
			if tc.retry {
				require.NotContains(t, recorder.Body.String(), "event: error\n")
				success := &http.Response{StatusCode: http.StatusOK, Header: upstream.Header, Body: io.NopCloser(strings.NewReader(strings.ReplaceAll(start, "msg_A", "msg_B") + completion))}
				_, retryErr := claude.ClaudeStreamHandler(c, success, info)
				require.Nil(t, retryErr)
				assert.NotContains(t, recorder.Body.String(), "event: error\n")
				assert.Contains(t, recorder.Body.String(), "event: message_stop\n")
			} else {
				channelErr := types.NewError(errors.New("no available channel key"), types.ErrorCodeChannelNoAvailableKey)
				assert.False(t, service.ShouldRetryRelayError(c, channelErr, 1), "channel errors must not restart an open message")
				writeRelayStreamError(c, types.RelayFormatClaude, apiErr)
				writeRelayStreamError(c, types.RelayFormatClaude, apiErr)
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
				if !tc.malformed {
					assert.Contains(t, recorder.Body.String(), `"provider_meta":{"trace_id":"trace_A"}`)
				}
				assert.NotContains(t, recorder.Body.String(), "event: message_stop\n")
				assert.NotContains(t, recorder.Body.String(), "msg_B")
			}
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: message_start\n"))
		})
	}
}

func TestClaudeMessageStartBlocksConfiguredGroupFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	model.DB, common.RedisEnabled, common.MemoryCacheEnabled = db, false, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled = previousDB, previousRedis, previousMemory
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.GroupChannelFallback{}))
	fallback := &model.Channel{Id: 2, Name: "fallback", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "test-only"}
	require.NoError(t, db.Create(fallback).Error)
	require.NoError(t, db.Create(&model.GroupChannelFallback{GroupName: "stream-test", ChannelType: fallback.Type, FallbackChannelId: fallback.Id, Enabled: true}).Error)
	selected, err := service.ResolveGroupFallbackChannel("stream-test", fallback.Type)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, fallback.Id, selected.Id, "the fallback is available before the response starts")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	require.NoError(t, helper.ClaudeData(c, dto.ClaudeResponse{Type: "message_start"}))
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, OriginModelName: "claude-test"}
	retry := &service.RetryParam{Ctx: c, TokenGroup: "stream-test", Retry: common.GetPointer(0)}
	last := &model.Channel{Id: 1, Type: fallback.Type}
	handled, apiErr := tryGroupFallbackAfterRetryExhausted(c, info, types.RelayFormatClaude, retry, last)
	assert.False(t, handled, "even an available fallback must not restart a message already sent")
	assert.Nil(t, apiErr)
	assert.Empty(t, c.GetStringSlice("use_channel"))
}

func TestClaudeStreamFinalErrorAfterKeepAlive(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	helper.SetEventStreamHeaders(c)
	require.NoError(t, helper.PingData(c))
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, StreamStatus: relaycommon.NewStreamStatus()}
	state := &claude.ClaudeResponseInfo{Usage: &dto.Usage{}}
	failure := `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low"}}`
	apiErr := claude.HandleStreamResponseData(c, info, state, failure)
	require.NotNil(t, apiErr)
	require.NotContains(t, recorder.Body.String(), "event: error\n")
	require.False(t, service.ShouldRetryRelayError(c, apiErr, 0))
	writeRelayStreamError(c, types.RelayFormatClaude, apiErr)
	writeRelayStreamError(c, types.RelayFormatClaude, apiErr)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error\n"))
	assert.Contains(t, recorder.Body.String(), `"type":"invalid_request_error"`)
	assert.Contains(t, recorder.Body.String(), "Your credit balance is too low")
	assert.NotContains(t, recorder.Body.String(), "event: message_start\n")
	assert.NotContains(t, recorder.Body.String(), "event: message_stop\n")
}

func TestClaudeRetryBoundaryPreservesOtherFormats(t *testing.T) {
	previousRetry := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 503}}
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previousRetry })
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{RelayFormat: format, StreamStatus: relaycommon.NewStreamStatus(), ChannelMeta: &relaycommon.ChannelMeta{}}
			state := &claude.ClaudeResponseInfo{Usage: &dto.Usage{}}
			for _, data := range []string{
				`{"type":"message_start","message":{"id":"msg_A","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
			} {
				require.Nil(t, claude.HandleStreamResponseData(c, info, state, data))
			}
			require.Contains(t, recorder.Body.String(), "hello")
			apiErr := claude.HandleStreamResponseData(c, info, state, `{"type":"error","error":{"type":"api_error","message":"upstream failed"}}`)
			require.NotNil(t, apiErr)
			assert.True(t, service.ShouldRetryRelayError(c, apiErr, 1), "the Claude-specific guard must preserve other response formats' existing retry policy")
			assert.NotContains(t, recorder.Body.String(), "event: message_start\n")
		})
	}
}

type partialClaudeWriteFailure struct {
	gin.ResponseWriter
}

func (w *partialClaudeWriteFailure) Write(data []byte) (int, error) {
	n, _ := w.ResponseWriter.Write(data[:min(1, len(data))])
	return n, io.ErrClosedPipe
}

func (w *partialClaudeWriteFailure) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func TestClaudeStreamInterruptedDownstream(t *testing.T) {
	for _, mode := range []string{"canceled before start", "canceled after start", "partial write failure"} {
		t.Run(mode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			if mode == "canceled before start" {
				cancel()
			} else if mode == "partial write failure" {
				c.Writer = &partialClaudeWriteFailure{ResponseWriter: c.Writer}
			}
			require.NoError(t, helper.ClaudeData(c, dto.ClaudeResponse{Type: "message_start"}))
			if mode == "canceled before start" {
				assert.Empty(t, recorder.Body.String())
				return
			}
			apiErr := types.NewError(errors.New("upstream failed"), types.ErrorCodeChannelNoAvailableKey)
			assert.False(t, service.ShouldRetryRelayError(c, apiErr, 1), "a partial or interrupted write must never restart the downstream message")
			if mode == "canceled after start" {
				cancel()
				before := recorder.Body.String()
				writeRelayStreamError(c, types.RelayFormatClaude, apiErr)
				assert.Equal(t, before, recorder.Body.String(), "do not write after cancellation")
			} else {
				assert.NotEmpty(t, recorder.Body.String(), "the write delivered part of an event before failing")
				assert.NotContains(t, recorder.Body.String(), "message_start")
			}
		})
	}
}
