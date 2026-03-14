package server

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const defaultLogLevel = "info"

var logLevelPriority = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
}

var metricsCounters = struct {
	activitiesCreated   atomic.Int64
	queueFlushAttempted atomic.Int64
	queueFlushSucceeded atomic.Int64
	queueFlushFailed    atomic.Int64
}{}

var latestQueueDepth atomic.Int64

func resolveLogLevel() string {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("CLAWTIVITY_LOG_LEVEL"))); value != "" {
		if _, ok := logLevelPriority[value]; ok {
			return value
		}
	}
	return defaultLogLevel
}

func shouldLog(level string) bool {
	value := strings.ToLower(strings.TrimSpace(level))
	priority, ok := logLevelPriority[value]
	if !ok {
		priority = logLevelPriority[defaultLogLevel]
	}
	currentPriority, exists := logLevelPriority[resolveLogLevel()]
	if !exists {
		currentPriority = logLevelPriority[defaultLogLevel]
	}
	return priority >= currentPriority
}

func logEvent(level, event string, details map[string]any, queueDepth int) {
	if !shouldLog(level) {
		return
	}
	if details == nil {
		details = make(map[string]any)
	}
	details["queue_depth"] = queueDepth

	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     strings.ToLower(level),
		"event":     event,
		"metrics": map[string]int64{
			"activities_created":    metricsCounters.activitiesCreated.Load(),
			"queue_flush_attempted": metricsCounters.queueFlushAttempted.Load(),
			"queue_flush_succeeded": metricsCounters.queueFlushSucceeded.Load(),
			"queue_flush_failed":    metricsCounters.queueFlushFailed.Load(),
			"queue_depth":           int64(queueDepth),
		},
		"details": details,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("{\"level\":\"error\",\"event\":\"log_serialization_failed\",\"message\":\"%v\"}", err)
		return
	}
	log.Println(string(data))
}

func incActivitiesCreated() {
	metricsCounters.activitiesCreated.Add(1)
}

func incQueueFlushAttempted() {
	metricsCounters.queueFlushAttempted.Add(1)
}

func incQueueFlushSucceeded() {
	metricsCounters.queueFlushSucceeded.Add(1)
}

func incQueueFlushFailed() {
	metricsCounters.queueFlushFailed.Add(1)
}

func currentQueueDepth() int {
	return int(latestQueueDepth.Load())
}

func storeQueueDepth(depth int) {
	latestQueueDepth.Store(int64(depth))
}
