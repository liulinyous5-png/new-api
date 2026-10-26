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
package aws

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
)

const maxQualityResponseBytes = 1<<20 + 1

const qualityProbeHeadersKey = "quality_probe_headers"

// QualityUnsupportedError means this AWS channel cannot run an Anthropic
// Messages probe. Converse API-key channels and Nova models use other protocols.
type QualityUnsupportedError struct {
	Reason string
}

func (e *QualityUnsupportedError) Error() string {
	if e == nil || e.Reason == "" {
		return "this AWS channel mode cannot run a Messages probe"
	}
	return e.Reason
}

// MessagesProbeSupported reports whether an AWS channel can send a Messages probe.
// Mantle and AKSK InvokeModel can. Converse API-key mode cannot.
func MessagesProbeSupported(apiType string, keyType string) error {
	switch dto.AwsApiType(apiType) {
	case dto.AwsApiTypeMantle:
		return nil
	case "", dto.AwsApiTypeInvokeModel:
		if dto.AwsKeyType(keyType) == dto.AwsKeyTypeApiKey {
			return &QualityUnsupportedError{Reason: "this AWS channel mode cannot run a Messages probe"}
		}
		return nil
	default:
		return &QualityUnsupportedError{Reason: "this AWS channel mode cannot run a Messages probe"}
	}
}

func qualityProbeHeaders(c *gin.Context) map[string]string {
	if c == nil {
		return nil
	}
	value, ok := c.Get(qualityProbeHeadersKey)
	if !ok {
		return nil
	}
	headers, _ := value.(map[string]string)
	return headers
}

func applyQualityProbeHeaders(header http.Header, headers map[string]string) {
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		header.Set(key, value)
	}
}

// ExecuteQuality sends one Messages probe. Mantle returns the HTTP status and
// body. AKSK InvokeModel returns 200 and the Claude JSON body, or the SDK
// status when Bedrock rejects the call.
func (a *Adaptor) ExecuteQuality(c *gin.Context, info *relaycommon.RelayInfo, body []byte, probeHeaders map[string]string) (int, []byte, error) {
	if info == nil {
		return 0, nil, &QualityUnsupportedError{Reason: "this AWS channel mode cannot run a Messages probe"}
	}
	if isNovaModel(getAwsModelID(info.UpstreamModelName)) {
		return 0, nil, &QualityUnsupportedError{Reason: "Amazon Nova cannot run a Messages probe"}
	}
	c.Set(qualityProbeHeadersKey, probeHeaders)
	switch info.ChannelOtherSettings.AwsApiType {
	case dto.AwsApiTypeMantle:
		req, err := a.newMantleRequest(c, info, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		resp, err := channel.DoRequest(c, req, info)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxQualityResponseBytes))
		if err != nil {
			return 0, nil, err
		}
		return resp.StatusCode, respBody, nil
	case "", dto.AwsApiTypeInvokeModel:
		if _, err := a.GetRequestURL(info); err != nil {
			return 0, nil, err
		}
		if a.ClientMode == ClientModeApiKey {
			return 0, nil, &QualityUnsupportedError{Reason: "this AWS channel mode cannot run a Messages probe"}
		}
		if _, err := doAwsClientRequest(c, info, a, bytes.NewReader(body)); err != nil {
			return 0, nil, err
		}
		ctx, cancel := newAwsInvokeContext(c.Request.Context())
		defer cancel()
		input, ok := a.AwsReq.(*bedrockruntime.InvokeModelInput)
		if !ok || a.AwsClient == nil {
			return 0, nil, errors.New("aws invoke request was not prepared")
		}
		awsResp, err := a.AwsClient.InvokeModel(ctx, input)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return 0, nil, c.Request.Context().Err()
			}
			return getAwsErrorStatusCode(err), []byte(err.Error()), nil
		}
		if len(awsResp.Body) > maxQualityResponseBytes-1 {
			return 0, nil, errors.New("upstream response is too large")
		}
		return http.StatusOK, awsResp.Body, nil
	default:
		return 0, nil, &QualityUnsupportedError{Reason: "this AWS channel mode cannot run a Messages probe"}
	}
}
