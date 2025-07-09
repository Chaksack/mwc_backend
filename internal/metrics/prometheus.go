package metrics

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// PrometheusMetricsHandler returns a handler that exposes metrics in Prometheus format
func PrometheusMetricsHandler(metricsService *MetricsService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get metrics from the metrics service
		metrics := metricsService.GetMetrics()

		// Build Prometheus format response
		response := buildPrometheusResponse(metrics)

		// Set content type for Prometheus
		c.Set("Content-Type", "text/plain")

		return c.SendString(response)
	}
}

// buildPrometheusResponse converts the metrics to Prometheus format
func buildPrometheusResponse(metrics fiber.Map) string {
	var response string

	// Add uptime metric
	uptime := metrics["uptime"].(fiber.Map)
	response += "# HELP app_uptime_seconds The uptime of the application in seconds\n"
	response += "# TYPE app_uptime_seconds gauge\n"
	response += "app_uptime_seconds " + strconv.FormatFloat(uptime["seconds"].(float64), 'f', 2, 64) + "\n\n"

	// Add memory metrics
	memory := metrics["memory"].(fiber.Map)
	response += "# HELP app_memory_alloc_bytes Memory allocated and not yet freed\n"
	response += "# TYPE app_memory_alloc_bytes gauge\n"
	response += "app_memory_alloc_bytes " + strconv.FormatUint(memory["alloc"].(uint64), 10) + "\n\n"

	response += "# HELP app_memory_sys_bytes Memory obtained from the OS\n"
	response += "# TYPE app_memory_sys_bytes gauge\n"
	response += "app_memory_sys_bytes " + strconv.FormatUint(memory["sys"].(uint64), 10) + "\n\n"

	response += "# HELP app_memory_gc_count Number of completed GC cycles\n"
	response += "# TYPE app_memory_gc_count counter\n"
	response += "app_memory_gc_count " + strconv.FormatUint(memory["num_gc"].(uint64), 10) + "\n\n"

	// Add goroutines metric
	response += "# HELP app_goroutines Number of goroutines\n"
	response += "# TYPE app_goroutines gauge\n"
	response += "app_goroutines " + strconv.Itoa(metrics["goroutines"].(int)) + "\n\n"

	// Add HTTP request metrics
	http := metrics["http"].(fiber.Map)
	requestCount := http["request_count"].(map[string]int)
	avgResponseTime := http["avg_response_time"].(map[string]float64)

	response += "# HELP http_requests_total Total number of HTTP requests\n"
	response += "# TYPE http_requests_total counter\n"
	for path, count := range requestCount {
		response += "http_requests_total{path=\"" + path + "\"} " + strconv.Itoa(count) + "\n"
	}
	response += "\n"

	response += "# HELP http_request_duration_milliseconds Average duration of HTTP requests in milliseconds\n"
	response += "# TYPE http_request_duration_milliseconds gauge\n"
	for path, duration := range avgResponseTime {
		response += "http_request_duration_milliseconds{path=\"" + path + "\"} " + strconv.FormatFloat(duration, 'f', 2, 64) + "\n"
	}
	response += "\n"

	// Add database metrics
	database := metrics["database"].(fiber.Map)
	response += "# HELP db_queries_total Total number of database queries\n"
	response += "# TYPE db_queries_total counter\n"
	response += "db_queries_total " + strconv.Itoa(database["query_count"].(int)) + "\n\n"

	response += "# HELP db_query_duration_milliseconds Average duration of database queries in milliseconds\n"
	response += "# TYPE db_query_duration_milliseconds gauge\n"
	response += "db_query_duration_milliseconds " + strconv.FormatFloat(database["avg_query_time"].(float64), 'f', 2, 64) + "\n\n"

	// Add database connection metrics if available
	if connectionStats, ok := database["connection_stats"].(fiber.Map); ok {
		response += "# HELP db_connections_open Number of open database connections\n"
		response += "# TYPE db_connections_open gauge\n"
		response += "db_connections_open " + strconv.Itoa(connectionStats["open_connections"].(int)) + "\n\n"

		if inUse, ok := connectionStats["in_use"].(int); ok {
			response += "# HELP db_connections_in_use Number of database connections in use\n"
			response += "# TYPE db_connections_in_use gauge\n"
			response += "db_connections_in_use " + strconv.Itoa(inUse) + "\n\n"
		}

		if idle, ok := connectionStats["idle"].(int); ok {
			response += "# HELP db_connections_idle Number of idle database connections\n"
			response += "# TYPE db_connections_idle gauge\n"
			response += "db_connections_idle " + strconv.Itoa(idle) + "\n\n"
		}
	}

	return response
}

// SetupPrometheusMetricsRoute adds a route for Prometheus metrics
func SetupPrometheusMetricsRoute(app *fiber.App, metricsService *MetricsService) {
	app.Get("/metrics/prometheus", PrometheusMetricsHandler(metricsService))
}
