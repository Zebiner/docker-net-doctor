// Package diagnostics provides profiling report generation
package diagnostics

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ProfileReporter generates formatted profiling reports
type ProfileReporter struct {
	profiler *PerformanceProfiler
}

// NewProfileReporter creates a new profile reporter
func NewProfileReporter(profiler *PerformanceProfiler) *ProfileReporter {
	return &ProfileReporter{
		profiler: profiler,
	}
}

// GenerateReport generates a comprehensive profiling report
func (pr *ProfileReporter) GenerateReport() string {
	if pr.profiler == nil {
		return "No profiler configured"
	}

	metrics := pr.profiler.GetMetrics()
	var report strings.Builder

	// Header
	report.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	report.WriteString(fmt.Sprintf("%-40s%40s\n", " PERFORMANCE PROFILING REPORT", fmt.Sprintf("Generated: %s ", time.Now().Format("2006-01-02 15:04:05"))))
	report.WriteString("=" + strings.Repeat("=", 78) + "=\n\n")

	// Executive Summary
	pr.writeExecutiveSummary(&report, metrics)

	// Timing Analysis
	pr.writeTimingAnalysis(&report, metrics)

	// Check Performance
	pr.writeCheckPerformance(&report, metrics)

	// Worker Performance
	pr.writeWorkerPerformance(&report, metrics)

	// Category Breakdown
	pr.writeCategoryBreakdown(&report, metrics)

	// Bottleneck Analysis
	pr.writeBottleneckAnalysis(&report, metrics)

	// Resource Usage
	pr.writeResourceUsage(&report, metrics)

	// Profiling Overhead
	pr.writeProfilingOverhead(&report, metrics)

	// Recommendations
	pr.writeRecommendations(&report, metrics)

	return report.String()
}

// writeExecutiveSummary writes the executive summary section
func (pr *ProfileReporter) writeExecutiveSummary(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("EXECUTIVE SUMMARY\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	report.WriteString(fmt.Sprintf("Total Operations:     %d\n", metrics.TotalOperations))
	report.WriteString(fmt.Sprintf("Total Duration:       %v\n", metrics.TotalDuration))
	report.WriteString(fmt.Sprintf("Average Duration:     %v\n", metrics.AverageDuration))
	report.WriteString(fmt.Sprintf("Accuracy Achieved:    %v (target: 1ms)\n", metrics.AccuracyAchieved))

	// Performance rating
	rating := pr.calculatePerformanceRating(metrics)
	report.WriteString(fmt.Sprintf("Performance Rating:   %s\n", rating))

	report.WriteString("\n")
}

// writeTimingAnalysis writes timing statistics
func (pr *ProfileReporter) writeTimingAnalysis(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("TIMING ANALYSIS\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	report.WriteString(fmt.Sprintf("Minimum Duration:     %v\n", metrics.MinDuration))
	report.WriteString(fmt.Sprintf("Maximum Duration:     %v\n", metrics.MaxDuration))
	report.WriteString(fmt.Sprintf("Average Duration:     %v\n", metrics.AverageDuration))

	if len(metrics.Percentiles) > 0 {
		report.WriteString("\nPercentiles:\n")
		report.WriteString(fmt.Sprintf("  P50 (Median):       %v\n", metrics.Percentiles[50]))
		report.WriteString(fmt.Sprintf("  P95:                %v\n", metrics.Percentiles[95]))
		report.WriteString(fmt.Sprintf("  P99:                %v\n", metrics.Percentiles[99]))
	}

	report.WriteString("\n")
}

// writeCheckPerformance writes individual check performance metrics
func (pr *ProfileReporter) writeCheckPerformance(report *strings.Builder, metrics *ProfileMetrics) {
	if len(metrics.CheckMetrics) == 0 {
		return
	}

	report.WriteString("CHECK PERFORMANCE\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")
	report.WriteString(fmt.Sprintf("%-30s %10s %12s %12s %12s %8s\n",
		"Check Name", "Count", "Avg", "Min", "Max", "Success%"))
	report.WriteString(strings.Repeat("-", 80) + "\n")

	// Sort checks by average duration (slowest first)
	checks := make([]*CheckProfileMetrics, 0, len(metrics.CheckMetrics))
	for _, check := range metrics.CheckMetrics {
		checks = append(checks, check)
	}
	sort.Slice(checks, func(i, j int) bool {
		return checks[i].AverageDuration > checks[j].AverageDuration
	})

	for _, check := range checks {
		report.WriteString(fmt.Sprintf("%-30s %10d %12v %12v %12v %7.1f%%\n",
			truncateString(check.Name, 30),
			check.ExecutionCount,
			check.AverageDuration,
			check.MinDuration,
			check.MaxDuration,
			check.SuccessRate*100,
		))
	}

	report.WriteString("\n")
}

// writeWorkerPerformance writes worker pool performance metrics
func (pr *ProfileReporter) writeWorkerPerformance(report *strings.Builder, metrics *ProfileMetrics) {
	if len(metrics.WorkerMetrics) == 0 {
		return
	}

	report.WriteString("WORKER PERFORMANCE\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")
	report.WriteString(fmt.Sprintf("%-10s %12s %12s %12s %12s %12s\n",
		"Worker ID", "Jobs", "Busy Time", "Idle Time", "Avg Job", "Utilization"))
	report.WriteString(strings.Repeat("-", 80) + "\n")

	// Sort workers by utilization
	workers := make([]*WorkerProfileMetrics, 0, len(metrics.WorkerMetrics))
	for _, worker := range metrics.WorkerMetrics {
		workers = append(workers, worker)
	}
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].Utilization > workers[j].Utilization
	})

	for _, worker := range workers {
		report.WriteString(fmt.Sprintf("%-10d %12d %12v %12v %12v %11.1f%%\n",
			worker.WorkerID,
			worker.JobsProcessed,
			worker.TotalBusyTime,
			worker.TotalIdleTime,
			worker.AverageJobTime,
			worker.Utilization*100,
		))
	}

	// Calculate aggregate worker statistics
	var totalJobs int64
	var totalBusyTime time.Duration
	var totalUtilization float64

	for _, worker := range workers {
		totalJobs += worker.JobsProcessed
		totalBusyTime += worker.TotalBusyTime
		totalUtilization += worker.Utilization
	}

	if len(workers) > 0 {
		report.WriteString(strings.Repeat("-", 80) + "\n")
		report.WriteString(fmt.Sprintf("%-10s %12d %12v %12s %12v %11.1f%%\n",
			"TOTAL",
			totalJobs,
			totalBusyTime,
			"-",
			totalBusyTime/time.Duration(totalJobs),
			(totalUtilization/float64(len(workers)))*100,
		))
	}

	report.WriteString("\n")
}

// writeCategoryBreakdown writes performance breakdown by category
func (pr *ProfileReporter) writeCategoryBreakdown(report *strings.Builder, metrics *ProfileMetrics) {
	if len(metrics.CategoryMetrics) == 0 {
		return
	}

	report.WriteString("CATEGORY BREAKDOWN\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")
	report.WriteString(fmt.Sprintf("%-25s %10s %12s %12s %12s %10s\n",
		"Category", "Count", "Total", "Average", "Min", "Max"))
	report.WriteString(strings.Repeat("-", 80) + "\n")

	// Sort categories by total duration
	categories := make([]*struct {
		Name    string
		Metrics *CategoryMetrics
	}, 0, len(metrics.CategoryMetrics))

	for name, catMetrics := range metrics.CategoryMetrics {
		categories = append(categories, &struct {
			Name    string
			Metrics *CategoryMetrics
		}{Name: name, Metrics: catMetrics})
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Metrics.TotalDuration > categories[j].Metrics.TotalDuration
	})

	for _, cat := range categories {
		percentage := float64(cat.Metrics.TotalDuration) / float64(metrics.TotalDuration) * 100
		report.WriteString(fmt.Sprintf("%-25s %10d %12v %12v %12v %12v\n",
			truncateString(cat.Name, 25),
			cat.Metrics.Count,
			cat.Metrics.TotalDuration,
			cat.Metrics.AverageDuration,
			cat.Metrics.MinDuration,
			cat.Metrics.MaxDuration,
		))
		report.WriteString(fmt.Sprintf("  └─ %.1f%% of total execution time\n", percentage))
	}

	report.WriteString("\n")
}

// writeBottleneckAnalysis identifies and reports performance bottlenecks
func (pr *ProfileReporter) writeBottleneckAnalysis(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("BOTTLENECK ANALYSIS\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	bottlenecks := pr.identifyBottlenecks(metrics)

	if len(bottlenecks) == 0 {
		report.WriteString("No significant bottlenecks detected.\n")
	} else {
		report.WriteString("Identified Bottlenecks:\n\n")
		for i, bottleneck := range bottlenecks {
			report.WriteString(fmt.Sprintf("%d. %s\n", i+1, bottleneck.Description))
			report.WriteString(fmt.Sprintf("   Impact: %s\n", bottleneck.Impact))
			report.WriteString(fmt.Sprintf("   Recommendation: %s\n", bottleneck.Recommendation))
			if i < len(bottlenecks)-1 {
				report.WriteString("\n")
			}
		}
	}

	report.WriteString("\n")
}

// writeResourceUsage writes resource usage statistics
func (pr *ProfileReporter) writeResourceUsage(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("RESOURCE USAGE\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	// Get latest resource snapshot from collector
	if pr.profiler != nil && pr.profiler.collector != nil {
		latest := pr.profiler.collector.GetLatestSample()
		if latest != nil {
			report.WriteString(fmt.Sprintf("CPU Usage:            %.1f%%\n", latest.CPUPercent))
			report.WriteString(fmt.Sprintf("Memory Usage:         %.2f MB\n", latest.MemoryMB))
			report.WriteString(fmt.Sprintf("Goroutines:           %d\n", latest.Goroutines))
			report.WriteString(fmt.Sprintf("Active Jobs:          %d\n", latest.ActiveJobs))
			report.WriteString(fmt.Sprintf("Completed Jobs:       %d\n", latest.CompletedJobs))
			report.WriteString(fmt.Sprintf("Error Rate:           %.1f%%\n", latest.ErrorRate*100))
		}

		// Average metrics over last minute
		avgMetrics := pr.profiler.collector.GetAverageMetrics(1 * time.Minute)
		if avgMetrics != nil {
			report.WriteString("\nAverages (last minute):\n")
			report.WriteString(fmt.Sprintf("  CPU:                %.1f%%\n", avgMetrics.AvgCPU))
			report.WriteString(fmt.Sprintf("  Memory:             %.2f MB\n", avgMetrics.AvgMemoryMB))
			report.WriteString(fmt.Sprintf("  Goroutines:         %d\n", avgMetrics.AvgGoroutines))
		}
	}

	report.WriteString("\n")
}

// writeProfilingOverhead writes profiling overhead analysis
func (pr *ProfileReporter) writeProfilingOverhead(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("PROFILING OVERHEAD\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	overheadPercent := pr.profiler.GetOverheadPercentage()
	overheadDuration := time.Duration(metrics.OverheadNanos)

	report.WriteString(fmt.Sprintf("Total Overhead:       %v\n", overheadDuration))
	report.WriteString(fmt.Sprintf("Overhead Percentage:  %.2f%%\n", overheadPercent))
	report.WriteString(fmt.Sprintf("Target Limit:         %.1f%%\n", pr.profiler.config.MaxOverhead*100))

	if pr.profiler.IsWithinOverheadLimit() {
		report.WriteString("Status:               ✓ Within acceptable limits\n")
	} else {
		report.WriteString("Status:               ✗ Exceeds acceptable limits\n")
		report.WriteString("                      Consider reducing profiling detail level\n")
	}

	report.WriteString("\n")
}

// writeRecommendations writes performance recommendations
func (pr *ProfileReporter) writeRecommendations(report *strings.Builder, metrics *ProfileMetrics) {
	report.WriteString("RECOMMENDATIONS\n")
	report.WriteString("-" + strings.Repeat("-", 78) + "\n")

	recommendations := pr.generateRecommendations(metrics)

	if len(recommendations) == 0 {
		report.WriteString("Performance is optimal. No recommendations at this time.\n")
	} else {
		for i, rec := range recommendations {
			report.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
			if i < len(recommendations)-1 {
				report.WriteString("\n")
			}
		}
	}

	report.WriteString("\n")
}

// GenerateFlameGraph generates a flame graph visualization
func (pr *ProfileReporter) GenerateFlameGraph() ([]byte, error) {
	if pr.profiler == nil || pr.profiler.measurements == nil {
		return nil, fmt.Errorf("no profiling data available")
	}

	// Simplified flame graph generation
	// In production, you'd use a proper flame graph library
	var buf bytes.Buffer

	buf.WriteString("# Flame Graph Data\n")
	buf.WriteString("# Format: stack_trace count\n\n")

	// Aggregate call stacks
	stackCounts := make(map[string]int)
	timings := pr.profiler.measurements.GetAll()

	for _, timing := range timings {
		if timing != nil && len(timing.CallStack) > 0 {
			stack := strings.Join(timing.CallStack, ";")
			stackCounts[stack]++
		}
	}

	// Write stack counts
	for stack, count := range stackCounts {
		buf.WriteString(fmt.Sprintf("%s %d\n", stack, count))
	}

	return buf.Bytes(), nil
}

// GenerateSummary generates a brief summary of profiling results
func (pr *ProfileReporter) GenerateSummary() string {
	if pr.profiler == nil {
		return "No profiling data available"
	}

	metrics := pr.profiler.GetMetrics()
	overhead := pr.profiler.GetOverheadPercentage()

	return fmt.Sprintf(
		"Profiling Summary: %d ops in %v (avg: %v, P95: %v) | Overhead: %.2f%% | Accuracy: %v",
		metrics.TotalOperations,
		metrics.TotalDuration,
		metrics.AverageDuration,
		metrics.Percentiles[95],
		overhead,
		metrics.AccuracyAchieved,
	)
}

// Helper functions

// calculatePerformanceRating calculates an overall performance rating
func (pr *ProfileReporter) calculatePerformanceRating(metrics *ProfileMetrics) string {
	score := 100.0

	// Deduct points for slow operations
	if metrics.AverageDuration > 100*time.Millisecond {
		score -= 20
	} else if metrics.AverageDuration > 50*time.Millisecond {
		score -= 10
	}

	// Deduct points for high P95
	if p95, ok := metrics.Percentiles[95]; ok {
		if p95 > 500*time.Millisecond {
			score -= 20
		} else if p95 > 200*time.Millisecond {
			score -= 10
		}
	}

	// Deduct points for poor accuracy
	if metrics.AccuracyAchieved > 5*time.Millisecond {
		score -= 15
	}

	// Deduct points for high overhead
	overhead := pr.profiler.GetOverheadPercentage()
	if overhead > 10 {
		score -= 20
	} else if overhead > 5 {
		score -= 10
	}

	// Return rating based on score
	switch {
	case score >= 90:
		return "★★★★★ Excellent"
	case score >= 75:
		return "★★★★☆ Good"
	case score >= 60:
		return "★★★☆☆ Fair"
	case score >= 40:
		return "★★☆☆☆ Poor"
	default:
		return "★☆☆☆☆ Critical"
	}
}

// Bottleneck represents a identified performance bottleneck
type Bottleneck struct {
	Description    string
	Impact         string
	Recommendation string
}

// identifyBottlenecks analyzes metrics to identify performance bottlenecks
func (pr *ProfileReporter) identifyBottlenecks(metrics *ProfileMetrics) []Bottleneck {
	var bottlenecks []Bottleneck

	// Check for slow checks
	for name, check := range metrics.CheckMetrics {
		if check.AverageDuration > 200*time.Millisecond {
			bottlenecks = append(bottlenecks, Bottleneck{
				Description:    fmt.Sprintf("Check '%s' is slow (avg: %v)", name, check.AverageDuration),
				Impact:         "High - Significantly impacts overall diagnostic time",
				Recommendation: "Optimize check implementation or run in parallel",
			})
		}
	}

	// Check for high P99 latency
	if p99, ok := metrics.Percentiles[99]; ok && p99 > 1*time.Second {
		bottlenecks = append(bottlenecks, Bottleneck{
			Description:    fmt.Sprintf("High P99 latency detected: %v", p99),
			Impact:         "Medium - Occasional slow operations",
			Recommendation: "Investigate outliers and add timeouts",
		})
	}

	// Check for worker imbalance
	if len(metrics.WorkerMetrics) > 1 {
		var maxUtil, minUtil float64 = 0, 1
		for _, worker := range metrics.WorkerMetrics {
			if worker.Utilization > maxUtil {
				maxUtil = worker.Utilization
			}
			if worker.Utilization < minUtil {
				minUtil = worker.Utilization
			}
		}
		if maxUtil-minUtil > 0.3 {
			bottlenecks = append(bottlenecks, Bottleneck{
				Description:    "Worker load imbalance detected",
				Impact:         "Medium - Inefficient resource utilization",
				Recommendation: "Review work distribution algorithm",
			})
		}
	}

	return bottlenecks
}

// generateRecommendations generates performance improvement recommendations
func (pr *ProfileReporter) generateRecommendations(metrics *ProfileMetrics) []string {
	var recommendations []string

	// Check average duration
	if metrics.AverageDuration > 100*time.Millisecond {
		recommendations = append(recommendations,
			"Average operation duration is high. Consider:\n"+
				"   • Increasing parallelism\n"+
				"   • Caching frequently accessed data\n"+
				"   • Optimizing slow checks")
	}

	// Check accuracy
	if metrics.AccuracyAchieved > 5*time.Millisecond {
		recommendations = append(recommendations,
			"Timing accuracy is below target. Consider:\n"+
				"   • Using higher precision timers\n"+
				"   • Reducing system load during profiling\n"+
				"   • Adjusting sampling rate")
	}

	// Check overhead
	if pr.profiler.GetOverheadPercentage() > 5 {
		recommendations = append(recommendations,
			"Profiling overhead is high. Consider:\n"+
				"   • Reducing profiling detail level\n"+
				"   • Sampling instead of full profiling\n"+
				"   • Disabling call stack capture")
	}

	// Check for failed operations
	failureCount := 0
	for _, check := range metrics.CheckMetrics {
		if check.SuccessRate < 0.9 {
			failureCount++
		}
	}
	if failureCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("%d checks have high failure rates. Consider:\n"+
				"   • Adding retry logic\n"+
				"   • Improving error handling\n"+
				"   • Investigating root causes", failureCount))
	}

	return recommendations
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
