package server

import (
	"net/http"
	"os"
	"strings"

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
		AllowOrigins:     resolveCorsOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		AllowCredentials: true, // Enable cookies/auth
	}))

	r.GET("/", s.HelloWorldHandler)

	r.GET("/health", s.healthHandler)
	r.POST("/api/activity", activityAPIKeyMiddleware(), s.createActivityHandler)
	r.GET("/api/activity", s.listActivitiesHandler)
	r.GET("/api/activity/summary", s.activitySummaryHandler)
	r.GET("/api/projects", s.listProjectsHandler)
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

func resolveCorsOrigins() []string {
	defaultOrigins := []string{"http://localhost:5173"}
	env := strings.TrimSpace(os.Getenv("CLAWTIVITY_CORS_ORIGINS"))
	if env == "" {
		return defaultOrigins
	}

	parts := strings.Split(env, ",")
	var trimmed []string
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate != "" {
			trimmed = append(trimmed, candidate)
		}
	}
	if len(trimmed) == 0 {
		return defaultOrigins
	}
	return trimmed
}

func activityAPIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expectedKey := strings.TrimSpace(os.Getenv("CLAWTIVITY_API_KEY"))
		if expectedKey == "" {
			c.Next()
			return
		}

		providedKey := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if providedKey == "" || providedKey != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Next()
	}
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
