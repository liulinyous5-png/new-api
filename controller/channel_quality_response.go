package controller

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetChannelQualityResponse(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	channelID, channelErr := strconv.Atoi(c.Param("id"))
	runID, runErr := strconv.Atoi(c.Param("runId"))
	if channelErr != nil || runErr != nil || channelID <= 0 || runID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid quality run id"})
		return
	}
	response, err := model.GetChannelQualityResponse(channelID, runID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "quality run not found"})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var data any
	if response != nil {
		data = gin.H{
			"run_id":         response.RunId,
			"status_code":    response.StatusCode,
			"body_base64":    base64.StdEncoding.EncodeToString(response.Body),
			"truncated":      response.Truncated,
			"incomplete":     response.Incomplete,
			"formatted_body": formatQualityResponsePreview(response.Body),
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
}

// Bound formatting work and preserve number/string lexemes instead of parsing
// through JavaScript numbers. Full bytes remain available for copy/download.
func formatQualityResponsePreview(body []byte) *string {
	const previewLimit = 65536
	if len(body) > previewLimit {
		return nil
	}
	depth := 0
	quoted, escaped := false, false
	for _, char := range body {
		if quoted {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				quoted = false
			}
			continue
		}
		switch char {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > 16 {
				return nil
			}
		case '}', ']':
			depth--
		}
	}
	formatted, err := common.IndentJson(body)
	if err != nil || len(formatted) > previewLimit {
		return nil
	}
	text := string(formatted)
	return &text
}

func captureQualityResponse(status int, body []byte, incomplete bool) *model.ChannelQualityResponse {
	return &model.ChannelQualityResponse{
		StatusCode: status,
		Body:       bytes.Clone(body[:min(len(body), maxQualityResponseBytes)]),
		Truncated:  len(body) > maxQualityResponseBytes,
		Incomplete: incomplete,
	}
}
