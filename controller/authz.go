package controller

import (
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
)

// GetPermissionCatalog returns the permission schema used by the client to
// render the permission editor: the registry of resources with their actions
// and display label keys, plus the roles with their baseline grant matrices.
// Defining it in the authz package keeps the schema in a single place.
func GetPermissionCatalog(c *gin.Context) {
	types := make([]int, 0, len(constant.ChannelTypeNames))
	for id := range constant.ChannelTypeNames {
		if id > 0 {
			types = append(types, id)
		}
	}
	slices.Sort(types)
	channelTypes := make([]gin.H, 0, len(types))
	for _, id := range types {
		channelTypes = append(channelTypes, gin.H{"id": id, "name": constant.ChannelTypeNames[id]})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"resources":     authz.Catalog(),
			"roles":         authz.Roles(),
			"channel_types": channelTypes,
		},
	})
}
