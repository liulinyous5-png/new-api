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
package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
)

func registerEndpointAuditRoutes(apiRouter *gin.RouterGroup) {
	group := apiRouter.Group("/endpoint-audit")
	group.Use(middleware.AdminAuth())
	routes := []permissionRoute{
		{method: http.MethodGet, path: "/tests", permission: authz.EndpointAuditRead, handler: controller.ListEndpointAuditRuns},
		{method: http.MethodPost, path: "/tests", permission: authz.EndpointAuditOperate, handler: controller.StartEndpointAudit},
		{method: http.MethodGet, path: "/tests/:id", permission: authz.EndpointAuditRead, handler: controller.GetEndpointAuditRun},
	}
	for _, route := range routes {
		handlePermissionRoute(group, route.method, route.path, route.permission, route.handler)
	}
}
