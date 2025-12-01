package http

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

//go:embed static
var EFS embed.FS

// initRouter 初始化所有路由
func (s *Service) initRouter() {
	log.Debug().Msg("initializing router")
	s.initBaseRouter()
	s.initMediaRouter()
	s.initAPIRouter()
	s.initMCPRouter()
}

// initBaseRouter 初始化基础路由（静态文件、首页等）
func (s *Service) initBaseRouter() {
	log.Debug().Msg("initializing base router")
	staticDir, _ := fs.Sub(EFS, "static")
	s.router.StaticFS("/static", http.FS(staticDir))
	s.router.StaticFileFS("/favicon.ico", "./favicon.ico", http.FS(staticDir))
	s.router.StaticFileFS("/", "./index.htm", http.FS(staticDir))
	s.router.GET("/health", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	s.router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		log.Debug().Str("path", path).Msg("no route found")
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/static") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			return
		}
		c.Header("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate, value")
		c.Redirect(http.StatusFound, "/")
	})
}

// initMediaRouter 初始化媒体路由（图片、视频、文件、语音等）
func (s *Service) initMediaRouter() {
	log.Debug().Msg("initializing media router")
	s.router.GET("/image/*key", func(c *gin.Context) { s.handleMedia(c, "image") })
	s.router.GET("/video/*key", func(c *gin.Context) { s.handleMedia(c, "video") })
	s.router.GET("/file/*key", func(c *gin.Context) { s.handleMedia(c, "file") })
	s.router.GET("/voice/*key", func(c *gin.Context) { s.handleMedia(c, "voice") })
	s.router.GET("/data/*path", s.handleMediaData)
	s.router.GET("/avatar/:username", s.handleAvatar)
}

// initAPIRouter 初始化 API 路由
func (s *Service) initAPIRouter() {
	log.Debug().Msg("initializing API router")
	api := s.router.Group("/api/v1")
	{
		api.GET("/setting", s.handleGetSetting)
		api.POST("/setting", s.handleUpdateSetting)

		actions := api.Group("/actions")
		actions.POST("/get-data-key", s.handleActionGetDataKey)
		actions.POST("/decrypt", s.handleActionDecrypt)
		actions.POST("/http/start", s.handleActionStartHTTP)
		actions.POST("/http/stop", s.handleActionStopHTTP)
		actions.POST("/auto-decrypt/start", s.handleActionStartAutoDecrypt)
		actions.POST("/auto-decrypt/stop", s.handleActionStopAutoDecrypt)

		dataAPI := api.Group("", s.checkDBStateMiddleware())
		dataAPI.GET("/chatlog", s.handleChatlog)
		dataAPI.GET("/contact", s.handleContacts)
		dataAPI.GET("/chatroom", s.handleChatRooms)
		dataAPI.GET("/session", s.handleSessions)
		dataAPI.GET("/diary", s.handleDiary)
		dataAPI.GET("/dashboard", s.handleDashboard)
		dataAPI.GET("/search", s.handleSearch)
	}
}

// initMCPRouter 初始化 MCP 路由
func (s *Service) initMCPRouter() {
	log.Debug().Msg("initializing MCP router")
	s.router.Any("/mcp", func(c *gin.Context) { s.mcpStreamableServer.ServeHTTP(c.Writer, c.Request) })
	s.router.Any("/sse", func(c *gin.Context) { s.mcpSSEServer.ServeHTTP(c.Writer, c.Request) })
	s.router.Any("/message", func(c *gin.Context) { s.mcpSSEServer.ServeHTTP(c.Writer, c.Request) })
}