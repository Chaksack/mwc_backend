package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"gorm.io/gorm"
)

// MetricsService handles the collection and exposure of application metrics
type MetricsService struct {
	db                *gorm.DB
	startTime         time.Time
	mutex             sync.RWMutex
	httpRequestCount  map[string]int
	httpResponseTimes map[string][]time.Duration
	dbQueryCount      int
	dbQueryTimes      []time.Duration
	logEntries        []LogEntry
	maxLogEntries     int
}

// LogEntry represents a log entry with timestamp, level, and message
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// NewMetricsService creates a new metrics service
func NewMetricsService(db *gorm.DB) *MetricsService {
	return &MetricsService{
		db:                db,
		startTime:         time.Now(),
		httpRequestCount:  make(map[string]int),
		httpResponseTimes: make(map[string][]time.Duration),
		maxLogEntries:     1000, // Keep last 1000 log entries
	}
}

// RecordHTTPRequest records an HTTP request
func (m *MetricsService) RecordHTTPRequest(method, path string, duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := method + " " + path
	m.httpRequestCount[key]++
	m.httpResponseTimes[key] = append(m.httpResponseTimes[key], duration)

	// Trim response times array if it gets too large
	if len(m.httpResponseTimes[key]) > 1000 {
		m.httpResponseTimes[key] = m.httpResponseTimes[key][len(m.httpResponseTimes[key])-1000:]
	}
}

// RecordDBQuery records a database query
func (m *MetricsService) RecordDBQuery(duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.dbQueryCount++
	m.dbQueryTimes = append(m.dbQueryTimes, duration)

	// Trim query times array if it gets too large
	if len(m.dbQueryTimes) > 1000 {
		m.dbQueryTimes = m.dbQueryTimes[len(m.dbQueryTimes)-1000:]
	}
}

// RecordLog records a log entry
func (m *MetricsService) RecordLog(level, message string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}

	m.logEntries = append(m.logEntries, entry)

	// Trim log entries if they exceed the maximum
	if len(m.logEntries) > m.maxLogEntries {
		m.logEntries = m.logEntries[len(m.logEntries)-m.maxLogEntries:]
	}
}

// GetMetrics returns the current metrics
func (m *MetricsService) GetMetrics() fiber.Map {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Calculate uptime
	uptime := time.Since(m.startTime)

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate average response time for each endpoint
	avgResponseTimes := make(map[string]float64)
	for path, times := range m.httpResponseTimes {
		if len(times) > 0 {
			var total time.Duration
			for _, t := range times {
				total += t
			}
			avgResponseTimes[path] = float64(total) / float64(len(times)) / float64(time.Millisecond)
		}
	}

	// Calculate average DB query time
	var avgDBQueryTime float64
	if len(m.dbQueryTimes) > 0 {
		var total time.Duration
		for _, t := range m.dbQueryTimes {
			total += t
		}
		avgDBQueryTime = float64(total) / float64(len(m.dbQueryTimes)) / float64(time.Millisecond)
	}

	// Get database stats
	var dbStats fiber.Map
	if m.db != nil {
		sqlDB, err := m.db.DB()
		if err == nil {
			dbStats = fiber.Map{
				"open_connections": sqlDB.Stats().OpenConnections,
				"in_use":           sqlDB.Stats().InUse,
				"idle":             sqlDB.Stats().Idle,
				"max_open_conns":   sqlDB.Stats().MaxOpenConnections,
			}
		}
	}

	// Return all metrics
	return fiber.Map{
		"uptime": fiber.Map{
			"seconds": uptime.Seconds(),
			"human":   uptime.String(),
		},
		"memory": fiber.Map{
			"alloc":      memStats.Alloc,
			"total_alloc": memStats.TotalAlloc,
			"sys":        memStats.Sys,
			"num_gc":     memStats.NumGC,
		},
		"goroutines": runtime.NumGoroutine(),
		"http": fiber.Map{
			"request_count":    m.httpRequestCount,
			"avg_response_time": avgResponseTimes,
		},
		"database": fiber.Map{
			"query_count":     m.dbQueryCount,
			"avg_query_time":  avgDBQueryTime,
			"connection_stats": dbStats,
		},
		"logs": m.getLastLogs(100), // Return last 100 logs
	}
}

// getLastLogs returns the last n log entries
func (m *MetricsService) getLastLogs(n int) []fiber.Map {
	if n > len(m.logEntries) {
		n = len(m.logEntries)
	}

	logs := make([]fiber.Map, n)
	for i, entry := range m.logEntries[len(m.logEntries)-n:] {
		logs[i] = fiber.Map{
			"timestamp": entry.Timestamp,
			"level":     entry.Level,
			"message":   entry.Message,
		}
	}

	return logs
}

// SetupMetricsRoutes sets up the metrics routes
func SetupMetricsRoutes(app *fiber.App, metricsService *MetricsService) {
	// Add the /metrics endpoint that serves the metrics dashboard
	app.Get("/metrics", func(c *fiber.Ctx) error {
		// Serve the metrics dashboard HTML
		return c.SendFile("./views/metrics.html")
	})

	// Add the /metrics/api endpoint that returns the metrics as JSON
	app.Get("/metrics/api", func(c *fiber.Ctx) error {
		return c.JSON(metricsService.GetMetrics())
	})

	// Add the /metrics/monitor endpoint that uses the built-in Fiber monitor
	app.Get("/metrics/monitor", monitor.New())

	// Add the /metrics/prometheus endpoint that returns metrics in Prometheus format
	app.Get("/metrics/prometheus", PrometheusMetricsHandler(metricsService))
}

// HTTPMetricsMiddleware creates a middleware that records HTTP request metrics
func HTTPMetricsMiddleware(metricsService *MetricsService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Record start time
		start := time.Now()

		// Process request
		err := c.Next()

		// Record metrics after request is processed
		duration := time.Since(start)
		metricsService.RecordHTTPRequest(c.Method(), c.Path(), duration)

		return err
	}
}
