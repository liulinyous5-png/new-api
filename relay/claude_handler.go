package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return newConvertRequestFailedError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	statusCodeMappingStr := c.GetString("status_code_mapping")

	buildRequestBody := func(forceMarshalFromRequest bool) (io.Reader, func(), *types.NewAPIError) {
		if (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) && !forceMarshalFromRequest {
			storage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				return nil, nil, types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			return common.NewReplayableBodyReader(storage), nil, nil
		}

		var outboundRequest any
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
			outboundRequest = request
		} else {
			convertedRequest, convErr := adaptor.ConvertClaudeRequest(c, info, request)
			if convErr != nil {
				return nil, nil, types.NewError(convErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
			outboundRequest = convertedRequest
		}

		jsonData, marshalErr := common.Marshal(outboundRequest)
		if marshalErr != nil {
			return nil, nil, types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
			jsonData, marshalErr = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
			if marshalErr != nil {
				return nil, nil, types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			if len(info.ParamOverride) > 0 {
				jsonData, marshalErr = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if marshalErr != nil {
					return nil, nil, newAPIErrorFromParamOverride(marshalErr)
				}
			}
		}

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, closer, bodyErr := relaycommon.NewOutboundJSONBody(jsonData)
		if bodyErr != nil {
			return nil, nil, types.NewError(bodyErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		return body, func() { _ = closer.Close() }, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		requestBody, release, bodyErr := buildRequestBody(attempt > 0)
		if bodyErr != nil {
			return bodyErr
		}

		resp, doReqErr := adaptor.DoRequest(c, info, requestBody)
		if release != nil {
			release()
		}
		if doReqErr != nil {
			return types.NewOpenAIError(doReqErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}

		if resp == nil {
			break
		}
		httpResp := resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			if attempt == 0 {
				changed := service.FixClaudeRequestOnFirstRetry(request, httpResp.StatusCode, newAPIError.Error())
				if changed > 0 {
					logger.LogInfo(c, fmt.Sprintf("retry claude request after self-heal, changed_count=%d", changed))
					continue
				}
			}
			return newAPIError
		}

		usage, handleRespErr := adaptor.DoResponse(c, httpResp, info)
		if handleRespErr != nil {
			service.ResetStatusCode(handleRespErr, statusCodeMappingStr)
			return handleRespErr
		}

		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
		return nil
	}

	return types.NewErrorWithStatusCode(fmt.Errorf("empty upstream response"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
}
