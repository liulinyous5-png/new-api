package perfmetrics

import (
	"context"
	"errors"
	"net"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// classifyChannelAttempt preserves confirmed upstream errors even if the
// downstream request is cancelled afterwards. No response content is stored.
func classifyChannelAttempt(ctx context.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) string {
	stream := info.StreamStatus.OutcomeSnapshot()
	if stream.Response == relaycommon.ResponseOutcomeFailed {
		return channelErrorCategory(false, stream.ErrorCode, stream.ErrorType, stream.ErrorStatus)
	}
	if apiErr != nil {
		root := rootAPIError(apiErr)
		code := string(root.GetErrorCode())
		if errors.Is(apiErr, context.Canceled) {
			return "cancelled"
		}
		var timeout net.Error
		if errors.Is(apiErr, context.DeadlineExceeded) || (errors.As(apiErr, &timeout) && timeout.Timeout()) {
			return "timeout"
		}
		// A read interrupted by downstream cancellation is not an upstream outage.
		if ctx != nil && ctx.Err() == context.Canceled && (code == string(types.ErrorCodeDoRequestFailed) || code == string(types.ErrorCodeReadResponseBodyFailed)) {
			return "cancelled"
		}
		status := root.StatusCode
		if root.OriginalStatusCode != 0 {
			status = root.OriginalStatusCode
		}
		return channelErrorCategory(root.GetErrorType() == types.ErrorTypeNewAPIError, code, root.ToOpenAIError().Type, status)
	}
	if info.PerformanceBusinessRejection {
		return "business_rejected"
	}
	if stream.Response == relaycommon.ResponseOutcomeIncomplete {
		switch stream.IncompleteReason {
		case "content_filter", "safety", "content_policy_violation":
			return "business_rejected"
		}
	}
	if stream.EndReason == relaycommon.StreamEndReasonTimeout || (info.StreamStatus != nil && errors.Is(info.StreamStatus.EndError, context.DeadlineExceeded)) {
		return "timeout"
	}
	if stream.EndReason == relaycommon.StreamEndReasonPanic {
		return "gateway"
	}
	outcome := ClassifyRelayOutcome(ctx, info, nil)
	if outcome == OutcomeIgnored {
		return "cancelled"
	}
	if outcome == OutcomeFailure {
		return "protocol"
	}
	if info.IsStream && info.StreamStatus == nil {
		return "unknown"
	}
	return "success"
}

func channelErrorCategory(local bool, code, errorType string, status int) string {
	code, errorType = strings.ToLower(code), strings.ToLower(errorType)
	if !local && (status == 401 || status == 403 || status == 408 || status == 429 || status >= 500) {
		// Some gateways use this generic label for every error. A concrete
		// HTTP outage must not be excluded as a client business rejection.
		// Specific codes such as context_length_exceeded still take precedence.
		if code == "invalid_request_error" {
			code = ""
		}
		if errorType == "invalid_request_error" {
			errorType = ""
		}
	}
	if classifyFailure(local, code, errorType, status) == OutcomeIgnored {
		return "business_rejected"
	}
	for _, value := range []string{code, errorType} {
		switch value {
		case "invalid_api_key", "api_key_invalid", "api_key_expired", "api_key_service_blocked", "invalid_authentication", "authentication_error", "unauthenticated", "permission_denied", "permission_error", "access_denied":
			return "upstream_auth"
		case "insufficient_quota", "quota_exceeded", "insufficient_user_quota", "pre_consume_token_quota_failed":
			return "upstream_quota"
		case "rate_limit_exceeded", "rate_limit_error", "resource_exhausted":
			return "rate_limit"
		case "model_not_found":
			return "model_unavailable"
		case "overloaded_error", "server_error", "internal_error", "service_unavailable":
			if local {
				return "gateway"
			}
			if status == 408 || status == 504 {
				return "timeout"
			}
			return "upstream_server"
		case string(types.ErrorCodeDoRequestFailed):
			return "network"
		case string(types.ErrorCodeReadResponseBodyFailed), string(types.ErrorCodeBadResponse), string(types.ErrorCodeBadResponseBody), string(types.ErrorCodeEmptyResponse):
			return "protocol"
		}
	}
	if local {
		return "gateway"
	}
	switch status {
	case 401, 403:
		return "upstream_auth"
	case 408, 504:
		return "timeout"
	case 429:
		return "rate_limit"
	}
	if status >= 500 {
		return "upstream_server"
	}
	return "upstream_other"
}
