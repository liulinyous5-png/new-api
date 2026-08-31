package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type UpsertGroupChannelFallbackRequest struct {
	Id                int    `json:"id"`
	GroupName         string `json:"group_name"`
	ChannelType       int    `json:"channel_type"`
	FallbackChannelId int    `json:"fallback_channel_id"`
	Enabled           *bool  `json:"enabled"`
	Remark            string `json:"remark"`
	// Deprecated: 仅为兼容旧前端保留，后端不再信任此字段判断编辑态。
	IsEdit *bool `json:"is_edit"`
}

type DeleteGroupChannelFallbackRequest struct {
	GroupName   string `json:"group_name"`
	ChannelType int    `json:"channel_type"`
}

func GetGroupChannelFallbacks(c *gin.Context) {
	list, err := model.GetAllGroupChannelFallbacks()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    list,
	})
}

func UpsertGroupChannelFallback(c *gin.Context) {
	var req UpsertGroupChannelFallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	req.Remark = strings.TrimSpace(req.Remark)

	targetGroup := req.GroupName
	targetChannelType := req.ChannelType

	if req.Id > 0 {
		existingByID, err := model.GetGroupChannelFallbackByID(req.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if existingByID == nil {
			common.ApiErrorMsg(c, "要编辑的兜底配置不存在")
			return
		}
		if req.GroupName != existingByID.GroupName || req.ChannelType != existingByID.ChannelType {
			common.ApiErrorMsg(c, "编辑不允许修改分组或渠道类型，请删除后重新新增")
			return
		}
		targetGroup = existingByID.GroupName
		targetChannelType = existingByID.ChannelType
	} else {
		existed, err := model.GetGroupChannelFallback(req.GroupName, req.ChannelType)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if existed != nil {
			common.ApiErrorMsg(c, "当前分组+渠道类型已存在兜底配置，请勿重复添加")
			return
		}
	}

	if err := service.ValidateGroupFallbackBinding(targetGroup, targetChannelType, req.FallbackChannelId); err != nil {
		common.ApiError(c, err)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfg := &model.GroupChannelFallback{
		Id:                req.Id,
		GroupName:         targetGroup,
		ChannelType:       targetChannelType,
		FallbackChannelId: req.FallbackChannelId,
		Enabled:           enabled,
		Remark:            req.Remark,
	}
	if err := model.UpsertGroupChannelFallback(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateGroupFallbackCache(targetGroup, targetChannelType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteGroupChannelFallback(c *gin.Context) {
	var req DeleteGroupChannelFallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		req.GroupName = c.Query("group_name")
		if req.GroupName == "" {
			req.GroupName = c.Query("group")
		}
		channelTypeStr := c.Query("channel_type")
		if channelTypeStr != "" {
			if parsed, parseErr := strconv.Atoi(channelTypeStr); parseErr == nil {
				req.ChannelType = parsed
			}
		}
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	if err := model.DeleteGroupChannelFallback(req.GroupName, req.ChannelType); err != nil {
		common.ApiError(c, err)
		return
	}
	service.InvalidateGroupFallbackCache(req.GroupName, req.ChannelType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
