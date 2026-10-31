package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetQualityWebhookConfig(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	cfg, err := service.GetQualityWebhookConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"enabled": cfg.Enabled, "url": cfg.URL, "cooldown_seconds": cfg.CooldownSeconds, "secret_configured": cfg.Secret != ""}})
}

func UpdateQualityWebhookConfig(c *gin.Context) {
	var request struct {
		Enabled         bool    `json:"enabled"`
		URL             string  `json:"url"`
		CooldownSeconds int     `json:"cooldown_seconds"`
		Secret          *string `json:"secret"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	cfg, err := service.GetQualityWebhookConfig()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cfg.Enabled, cfg.URL, cfg.CooldownSeconds = request.Enabled, request.URL, request.CooldownSeconds
	if request.Secret != nil {
		cfg.Secret = *request.Secret
	}
	if err := service.SaveQualityWebhookConfig(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
