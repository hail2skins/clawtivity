package server

import (
	"errors"
	"net/http"

	"clawtivity/internal/database"
	"github.com/gin-gonic/gin"
)

func (s *Server) createActivityHandler(c *gin.Context) {
	var input database.ActivityFeed
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Always generate a fresh ID server-side.
	input.ID = ""

	if err := s.db.CreateActivity(c.Request.Context(), &input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create activity"})
		return
	}

	c.JSON(http.StatusCreated, input)
}

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
