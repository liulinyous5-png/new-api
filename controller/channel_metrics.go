package controller

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
)

func GetChannelMetrics(c *gin.Context) {
	minutes, err := strconv.Atoi(c.DefaultQuery("minutes", "5"))
	if err != nil || !slices.Contains([]int{5, 30, 60}, minutes) || c.Query("hours") != "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
		return
	}
	ids := []int{}
	for raw := range strings.SplitSeq(c.Query("channel_ids"), ",") {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 || len(ids) >= 200 {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid parameters"))
			return
		}
		ids = append(ids, id)
	}
	result, err := perfmetrics.QueryChannelMetrics(c.Request.Context(), ids, minutes)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, result)
}
