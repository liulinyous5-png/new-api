package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/pkg/jsplugin"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	doubaoOfficialRouter := router.Group("/api/v3/contents/generations")
	doubaoOfficialRouter.Use(
		middleware.RouteTag("relay"),
		pinTaskPluginAliasRoute("doubao", http.MethodPost, "/doubao/api/v3/contents/generations/tasks"),
		middleware.TokenAuth(),
		middleware.SystemPerformanceCheck(),
		middleware.ModelRequestRateLimit(),
		middleware.PrepareTaskPluginRoute(),
		middleware.Distribute(),
	)
	doubaoOfficialRouter.POST("/tasks", controller.RelayTask)

	doubaoOfficialFetchRouter := router.Group("/api/v3/contents/generations")
	doubaoOfficialFetchRouter.Use(
		middleware.RouteTag("relay"),
		pinTaskPluginAliasRoute("doubao", http.MethodGet, "/doubao/api/v3/contents/generations/tasks/:task_id"),
		middleware.TokenAuth(),
		middleware.SystemPerformanceCheck(),
		middleware.ModelRequestRateLimit(),
		middleware.PrepareTaskPluginRoute(),
	)
	doubaoOfficialFetchRouter.GET("/tasks/:task_id")

	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
}

func pinTaskPluginAliasRoute(pluginKey, method, declaredPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		generation := jsplugin.DefaultRegistry.Generation()
		if generation == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "task plugin is unavailable"}})
			return
		}
		binding, found := generation.LookupDeclaredRoute(method, declaredPath)
		if !found || binding.Plugin == nil || binding.Plugin.Meta.Key != pluginKey {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "task plugin is unavailable"}})
			return
		}
		c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: generation, Plugin: binding.Plugin})
		c.Set(jsplugin.ContextKeyPinnedRoute, jsplugin.PinnedRoute{Generation: generation, Plugin: binding.Plugin, Route: binding.Route})
		c.Next()
	}
}
