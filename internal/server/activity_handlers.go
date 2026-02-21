package server

import (
	"errors"
	"net/http"

	"clawtivity/internal/classifier"
	"clawtivity/internal/database"
	"github.com/gin-gonic/gin"
)

type APIError struct {
	Error string `json:"error"`
}

// createActivityHandler godoc
// @Summary Create activity
// @Description Create new activity entry from OpenClaw activity payload.
// @Tags activities
// @Accept json
// @Produce json
// @Param activity body database.ActivityFeed true "Activity data"
// @Success 201 {object} database.ActivityFeed
// @Failure 400 {object} APIError
// @Failure 500 {object} APIError
// @Router /api/activity [post]
func (s *Server) createActivityHandler(c *gin.Context) {
	var input activityIngest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Always generate a fresh ID server-side.
	input.ActivityFeed.ID = ""
	normalizeActivity(&input.ActivityFeed)
	applyActivityClassification(&input.ActivityFeed, classifier.Signals{
		PromptText:    input.PromptText,
		AssistantText: input.AssistantText,
		ToolsUsed:     input.ToolsUsed,
	})

	if err := s.db.CreateActivity(c.Request.Context(), &input.ActivityFeed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create activity"})
		return
	}

	c.JSON(http.StatusCreated, input.ActivityFeed)
}

// listActivitiesHandler godoc
// @Summary List activities
// @Description List activity entries with optional filters.
// @Tags activities
// @Produce json
// @Param project query string false "Filter by project_tag"
// @Param model query string false "Filter by model"
// @Param date query string false "Filter by created_at date (YYYY-MM-DD)"
// @Success 200 {array} database.ActivityFeed
// @Failure 400 {object} APIError
// @Failure 500 {object} APIError
// @Router /api/activity [get]
func (s *Server) listActivitiesHandler(c *gin.Context) {
	filters := database.ActivityFilters{
		ProjectTag: c.Query("project"),
		Model:      c.Query("model"),
		Date:       c.Query("date"),
	}

	activities, err := s.db.ListActivities(c.Request.Context(), filters)
	if err != nil {
		if errors.Is(err, database.ErrInvalidDateFilter) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query activities"})
		return
	}

	c.JSON(http.StatusOK, activities)
}

// activitySummaryHandler godoc
// @Summary Get activity summary
// @Description Get aggregated activity stats with optional filters.
// @Tags activities
// @Produce json
// @Param project query string false "Filter by project_tag"
// @Param model query string false "Filter by model"
// @Param date query string false "Filter by created_at date (YYYY-MM-DD)"
// @Success 200 {object} database.ActivitySummary
// @Failure 400 {object} APIError
// @Failure 500 {object} APIError
// @Router /api/activity/summary [get]
func (s *Server) activitySummaryHandler(c *gin.Context) {
	filters := database.ActivityFilters{
		ProjectTag: c.Query("project"),
		Model:      c.Query("model"),
		Date:       c.Query("date"),
	}

	summary, err := s.db.SummarizeActivities(c.Request.Context(), filters)
	if err != nil {
		if errors.Is(err, database.ErrInvalidDateFilter) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize activities"})
		return
	}

	c.JSON(http.StatusOK, summary)
}
