package server

import (
	"net/http"

	_ "clawtivity/docs"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"clawtivity/cmd/web"
	"io/fs"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"}, // Add your frontend URL
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true, // Enable cookies/auth
	}))

	r.GET("/", s.HelloWorldHandler)

	r.GET("/health", s.healthHandler)
	r.POST("/api/activity", s.createActivityHandler)
	r.GET("/api/activity", s.listActivitiesHandler)
	r.GET("/api/activity/summary", s.activitySummaryHandler)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	staticFiles, _ := fs.Sub(web.Files, "assets")
	r.StaticFS("/assets", http.FS(staticFiles))

	r.GET("/web", func(c *gin.Context) {
		web.DashboardHandler(c.Writer, c.Request)
	})

	r.POST("/hello", func(c *gin.Context) {
		web.HelloWebHandler(c.Writer, c.Request)
	})

	return r
}

func (s *Server) HelloWorldHandler(c *gin.Context) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	c.JSON(http.StatusOK, resp)
}

// healthHandler godoc
// @Summary Health check
// @Description Returns current service/database health details.
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}
