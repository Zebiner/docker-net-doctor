// Package diagnostics provides thread-safe timing data storage
package diagnostics

import (
	"fmt"
	"sync"
	"time"
)

// TimingStorage provides thread-safe storage for timing measurements
type TimingStorage struct {
	mu           sync.RWMutex
	measurements []*TimingData
	maxSize      int
	currentIndex int
	wrapped      bool // Indicates if we've wrapped around (circular buffer)
	
	// Indexes for fast lookup
	byCategory   map[string][]*TimingData
	byCheck      map[string][]*TimingData
	byWorker     map[int][]*TimingData
}

// NewTimingStorage creates a new timing storage with bounded size
func NewTimingStorage(maxSize int) *TimingStorage {
	if maxSize <= 0 {
		maxSize = MaxProfileDataPoints
	}

	return &TimingStorage{
		measurements: make([]*TimingData, 0, maxSize),
		maxSize:      maxSize,
		byCategory:   make(map[string][]*TimingData),
		byCheck:      make(map[string][]*TimingData),
		byWorker:     make(map[int][]*TimingData),
	}
}

// Store adds a new timing measurement to storage
func (ts *TimingStorage) Store(timing *TimingData) {
	if timing == nil {
		return
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Implement circular buffer to limit memory usage
	if len(ts.measurements) < ts.maxSize {
		ts.measurements = append(ts.measurements, timing)
		ts.currentIndex = len(ts.measurements) - 1
	} else {
		// Overwrite oldest entry
		ts.currentIndex = (ts.currentIndex + 1) % ts.maxSize
		
		// Remove old entry from indexes
		oldTiming := ts.measurements[ts.currentIndex]
		if oldTiming != nil {
			ts.removeFromIndexes(oldTiming)
		}
		
		ts.measurements[ts.currentIndex] = timing
		ts.wrapped = true
	}

	// Update indexes
	ts.addToIndexes(timing)
}

// GetLatest returns the most recent timing measurement
func (ts *TimingStorage) GetLatest() *TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if len(ts.measurements) == 0 {
		return nil
	}

	return ts.measurements[ts.currentIndex]
}

// GetAll returns all stored timing measurements
func (ts *TimingStorage) GetAll() []*TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make([]*TimingData, 0, len(ts.measurements))
	
	if ts.wrapped {
		// If wrapped, return in chronological order
		// Start from the oldest (next position after current)
		start := (ts.currentIndex + 1) % ts.maxSize
		for i := 0; i < ts.maxSize; i++ {
			idx := (start + i) % ts.maxSize
			if ts.measurements[idx] != nil {
				result = append(result, ts.measurements[idx])
			}
		}
	} else {
		// Not wrapped, return as is
		for _, timing := range ts.measurements {
			if timing != nil {
				result = append(result, timing)
			}
		}
	}

	return result
}

// GetByCategory returns all timing measurements for a specific category
func (ts *TimingStorage) GetByCategory(category string) []*TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	timings, exists := ts.byCategory[category]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]*TimingData, len(timings))
	copy(result, timings)
	return result
}

// GetByCheck returns all timing measurements for a specific check
func (ts *TimingStorage) GetByCheck(checkName string) []*TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	timings, exists := ts.byCheck[checkName]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]*TimingData, len(timings))
	copy(result, timings)
	return result
}

// GetByWorker returns all timing measurements for a specific worker
func (ts *TimingStorage) GetByWorker(workerID int) []*TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	timings, exists := ts.byWorker[workerID]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]*TimingData, len(timings))
	copy(result, timings)
	return result
}

// GetAllDurations returns all stored durations for percentile calculations
func (ts *TimingStorage) GetAllDurations() []time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	durations := make([]time.Duration, 0, len(ts.measurements))
	for _, timing := range ts.measurements {
		if timing != nil {
			durations = append(durations, timing.Duration)
		}
	}
	return durations
}

// GetCheckDurations returns durations for a specific check
func (ts *TimingStorage) GetCheckDurations(checkName string) []time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	timings, exists := ts.byCheck[checkName]
	if !exists {
		return nil
	}

	durations := make([]time.Duration, 0, len(timings))
	for _, timing := range timings {
		if timing != nil {
			durations = append(durations, timing.Duration)
		}
	}
	return durations
}

// GetCategoryDurations returns durations for a specific category
func (ts *TimingStorage) GetCategoryDurations(category string) []time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	timings, exists := ts.byCategory[category]
	if !exists {
		return nil
	}

	durations := make([]time.Duration, 0, len(timings))
	for _, timing := range timings {
		if timing != nil {
			durations = append(durations, timing.Duration)
		}
	}
	return durations
}

// GetTimeRange returns measurements within a specific time range
func (ts *TimingStorage) GetTimeRange(start, end time.Time) []*TimingData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]*TimingData, 0)
	for _, timing := range ts.measurements {
		if timing != nil && 
		   !timing.StartTime.Before(start) && 
		   !timing.EndTime.After(end) {
			result = append(result, timing)
		}
	}
	return result
}

// GetStatistics returns statistical summary of stored timings
func (ts *TimingStorage) GetStatistics() *TimingStatistics {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	stats := &TimingStatistics{
		Count:          len(ts.measurements),
		CategoryCounts: make(map[string]int),
		CheckCounts:    make(map[string]int),
		WorkerCounts:   make(map[int]int),
	}

	if stats.Count == 0 {
		return stats
	}

	var totalDuration time.Duration
	stats.MinDuration = time.Duration(1<<63 - 1) // Max int64

	for _, timing := range ts.measurements {
		if timing == nil {
			continue
		}

		// Update duration stats
		totalDuration += timing.Duration
		if timing.Duration < stats.MinDuration {
			stats.MinDuration = timing.Duration
		}
		if timing.Duration > stats.MaxDuration {
			stats.MaxDuration = timing.Duration
		}

		// Update counts
		stats.CategoryCounts[timing.Category]++
		if timing.CheckName != "" {
			stats.CheckCounts[timing.CheckName]++
		}
		if timing.WorkerID >= 0 {
			stats.WorkerCounts[timing.WorkerID]++
		}

		// Track success rate
		if timing.Success {
			stats.SuccessCount++
		}
	}

	if stats.Count > 0 {
		stats.AverageDuration = totalDuration / time.Duration(stats.Count)
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.Count)
	}

	// Calculate time span
	if len(ts.measurements) > 0 {
		firstTiming := ts.getFirstNonNil()
		lastTiming := ts.getLastNonNil()
		if firstTiming != nil && lastTiming != nil {
			stats.TimeSpan = lastTiming.EndTime.Sub(firstTiming.StartTime)
		}
	}

	return stats
}

// Clear removes all stored timing measurements
func (ts *TimingStorage) Clear() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.measurements = make([]*TimingData, 0, ts.maxSize)
	ts.currentIndex = 0
	ts.wrapped = false
	
	// Clear indexes
	ts.byCategory = make(map[string][]*TimingData)
	ts.byCheck = make(map[string][]*TimingData)
	ts.byWorker = make(map[int][]*TimingData)
}

// Size returns the current number of stored measurements
func (ts *TimingStorage) Size() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.measurements)
}

// Capacity returns the maximum storage capacity
func (ts *TimingStorage) Capacity() int {
	return ts.maxSize
}

// IsFull returns whether storage is at capacity
func (ts *TimingStorage) IsFull() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.measurements) >= ts.maxSize
}

// addToIndexes adds timing to internal indexes
func (ts *TimingStorage) addToIndexes(timing *TimingData) {
	// Add to category index
	if timing.Category != "" {
		ts.byCategory[timing.Category] = append(ts.byCategory[timing.Category], timing)
	}

	// Add to check index
	if timing.CheckName != "" {
		ts.byCheck[timing.CheckName] = append(ts.byCheck[timing.CheckName], timing)
	}

	// Add to worker index
	if timing.WorkerID >= 0 {
		ts.byWorker[timing.WorkerID] = append(ts.byWorker[timing.WorkerID], timing)
	}
}

// removeFromIndexes removes timing from internal indexes
func (ts *TimingStorage) removeFromIndexes(timing *TimingData) {
	// Remove from category index
	if timing.Category != "" {
		ts.removeTimingFromSlice(ts.byCategory[timing.Category], timing, timing.Category, "category")
	}

	// Remove from check index
	if timing.CheckName != "" {
		ts.removeTimingFromSlice(ts.byCheck[timing.CheckName], timing, timing.CheckName, "check")
	}

	// Remove from worker index
	if timing.WorkerID >= 0 {
		ts.removeTimingFromSlice(ts.byWorker[timing.WorkerID], timing, timing.WorkerID, "worker")
	}
}

// removeTimingFromSlice removes a timing from a slice and updates the index
func (ts *TimingStorage) removeTimingFromSlice(slice []*TimingData, timing *TimingData, key interface{}, indexType string) {
	for i, t := range slice {
		if t == timing {
			// Remove element by replacing with last and truncating
			slice[i] = slice[len(slice)-1]
			slice = slice[:len(slice)-1]
			
			// Update the index map
			switch indexType {
			case "category":
				if keyStr, ok := key.(string); ok {
					ts.byCategory[keyStr] = slice
				}
			case "check":
				if keyStr, ok := key.(string); ok {
					ts.byCheck[keyStr] = slice
				}
			case "worker":
				if keyInt, ok := key.(int); ok {
					ts.byWorker[keyInt] = slice
				}
			}
			break
		}
	}
}

// getFirstNonNil returns the first non-nil timing
func (ts *TimingStorage) getFirstNonNil() *TimingData {
	for _, timing := range ts.measurements {
		if timing != nil {
			return timing
		}
	}
	return nil
}

// getLastNonNil returns the last non-nil timing
func (ts *TimingStorage) getLastNonNil() *TimingData {
	for i := len(ts.measurements) - 1; i >= 0; i-- {
		if ts.measurements[i] != nil {
			return ts.measurements[i]
		}
	}
	return nil
}

// TimingStatistics provides statistical summary of timing data
type TimingStatistics struct {
	Count           int
	SuccessCount    int
	SuccessRate     float64
	MinDuration     time.Duration
	MaxDuration     time.Duration
	AverageDuration time.Duration
	TimeSpan        time.Duration
	CategoryCounts  map[string]int
	CheckCounts     map[string]int
	WorkerCounts    map[int]int
}

// ExportData exports timing data in a format suitable for external analysis
func (ts *TimingStorage) ExportData(format string) ([]byte, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	switch format {
	case "json":
		return ts.exportJSON()
	case "csv":
		return ts.exportCSV()
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportJSON exports timing data as JSON
func (ts *TimingStorage) exportJSON() ([]byte, error) {
	// Simplified implementation - in production you'd use encoding/json
	data := ts.GetAll()
	// Convert to JSON (simplified for brevity)
	return []byte(fmt.Sprintf("{ \"measurements\": %d }", len(data))), nil
}

// exportCSV exports timing data as CSV
func (ts *TimingStorage) exportCSV() ([]byte, error) {
	// Simplified implementation - in production you'd use encoding/csv
	var csv string
	csv = "Name,Category,Duration,Success\n"
	
	for _, timing := range ts.GetAll() {
		if timing != nil {
			csv += fmt.Sprintf("%s,%s,%v,%v\n", 
				timing.Name, 
				timing.Category, 
				timing.Duration, 
				timing.Success)
		}
	}
	
	return []byte(csv), nil
}