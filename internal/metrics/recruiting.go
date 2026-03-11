package metrics

import "sync/atomic"

// Simple global counters for recruiting features (lightweight, in-memory)
var publishedJobsTotal uint64
var applicationsTotal uint64

// IncrementPublishedJobs increments the counter when a job transitions to published
func IncrementPublishedJobs() {
    atomic.AddUint64(&publishedJobsTotal, 1)
}

// IncrementApplications increments the counter when a new application is received
func IncrementApplications() {
    atomic.AddUint64(&applicationsTotal, 1)
}

// GetRecruitingMetrics returns a snapshot of recruiting counters
func GetRecruitingMetrics() map[string]uint64 {
    return map[string]uint64{
        "published_jobs_total": atomic.LoadUint64(&publishedJobsTotal),
        "applications_total":  atomic.LoadUint64(&applicationsTotal),
    }
}
