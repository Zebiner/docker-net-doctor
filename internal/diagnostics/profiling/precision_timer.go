// Package profiling provides high-precision timing utilities
package profiling

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
	_ "unsafe" // For go:linkname
)

// PrecisionTimer provides high-precision timing with 1ms accuracy target
type PrecisionTimer struct {
	startTime     time.Time
	startNano     int64
	stopNano      int64
	overhead      int64
	calibrated    bool
	calibrationNs int64
}

// NewPrecisionTimer creates a new high-precision timer
func NewPrecisionTimer() *PrecisionTimer {
	timer := &PrecisionTimer{}
	timer.calibrate()
	return timer
}

// calibrate calibrates the timer to measure its own overhead
func (t *PrecisionTimer) calibrate() {
	if t.calibrated {
		return
	}

	// Warm up
	for i := 0; i < 100; i++ {
		_ = nanotime()
	}

	// Measure overhead
	const samples = 1000
	var totalOverhead int64

	for i := 0; i < samples; i++ {
		start := nanotime()
		// Minimal operation
		_ = nanotime()
		end := nanotime()
		totalOverhead += end - start
	}

	t.calibrationNs = totalOverhead / samples
	t.calibrated = true
}

// Start begins timing
func (t *PrecisionTimer) Start() {
	// Use both for maximum precision
	t.startTime = time.Now()
	t.startNano = nanotime()
}

// Stop ends timing and returns the duration with overhead compensation
func (t *PrecisionTimer) Stop() time.Duration {
	// Capture stop time immediately
	t.stopNano = nanotime()
	
	// Calculate raw duration
	rawNanos := t.stopNano - t.startNano
	
	// Subtract calibrated overhead
	adjustedNanos := rawNanos - t.calibrationNs
	if adjustedNanos < 0 {
		adjustedNanos = 0
	}
	
	// Store overhead for reporting
	t.overhead = t.calibrationNs
	
	return time.Duration(adjustedNanos)
}

// GetOverhead returns the measured timer overhead
func (t *PrecisionTimer) GetOverhead() int64 {
	return t.overhead
}

// GetStartTime returns the wall clock start time
func (t *PrecisionTimer) GetStartTime() time.Time {
	return t.startTime
}

// Elapsed returns the elapsed time since Start was called
func (t *PrecisionTimer) Elapsed() time.Duration {
	if t.startNano == 0 {
		return 0
	}
	currentNano := nanotime()
	return time.Duration(currentNano - t.startNano)
}

// Reset resets the timer for reuse
func (t *PrecisionTimer) Reset() {
	t.startTime = time.Time{}
	t.startNano = 0
	t.stopNano = 0
	t.overhead = 0
}

// HighResolutionTimer provides an alternative high-resolution timer
type HighResolutionTimer struct {
	start     int64
	stop      int64
	overhead  int64
	precision time.Duration
}

// NewHighResolutionTimer creates a new high-resolution timer
func NewHighResolutionTimer() *HighResolutionTimer {
	return &HighResolutionTimer{
		precision: time.Microsecond, // Target microsecond precision
	}
}

// Start begins high-resolution timing
func (hrt *HighResolutionTimer) Start() {
	// Force garbage collection to minimize interference
	runtime.GC()
	// Use monotonic clock
	hrt.start = time.Now().UnixNano()
}

// Stop ends timing with high resolution
func (hrt *HighResolutionTimer) Stop() time.Duration {
	hrt.stop = time.Now().UnixNano()
	duration := hrt.stop - hrt.start
	
	// Round to target precision
	if hrt.precision > 0 {
		precisionNanos := hrt.precision.Nanoseconds()
		duration = (duration / precisionNanos) * precisionNanos
	}
	
	return time.Duration(duration)
}

// GetPrecision returns the timer's precision
func (hrt *HighResolutionTimer) GetPrecision() time.Duration {
	return hrt.precision
}

// SetPrecision sets the timer's precision
func (hrt *HighResolutionTimer) SetPrecision(precision time.Duration) {
	hrt.precision = precision
}

// TimerPool provides a pool of reusable precision timers
type TimerPool struct {
	timers chan *PrecisionTimer
}

// NewTimerPool creates a new timer pool
func NewTimerPool(size int) *TimerPool {
	if size <= 0 {
		size = 100
	}
	
	pool := &TimerPool{
		timers: make(chan *PrecisionTimer, size),
	}
	
	// Pre-populate pool
	for i := 0; i < size; i++ {
		pool.timers <- NewPrecisionTimer()
	}
	
	return pool
}

// Get retrieves a timer from the pool
func (tp *TimerPool) Get() *PrecisionTimer {
	select {
	case timer := <-tp.timers:
		timer.Reset()
		return timer
	default:
		// Pool empty, create new timer
		return NewPrecisionTimer()
	}
}

// Put returns a timer to the pool
func (tp *TimerPool) Put(timer *PrecisionTimer) {
	timer.Reset()
	select {
	case tp.timers <- timer:
		// Timer returned to pool
	default:
		// Pool full, let timer be garbage collected
	}
}

// BatchTimer times multiple operations and calculates statistics
type BatchTimer struct {
	measurements []time.Duration
	overhead     int64
	capacity     int
}

// NewBatchTimer creates a new batch timer
func NewBatchTimer(capacity int) *BatchTimer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &BatchTimer{
		measurements: make([]time.Duration, 0, capacity),
		capacity:     capacity,
	}
}

// Time measures the duration of an operation
func (bt *BatchTimer) Time(operation func()) time.Duration {
	timer := NewPrecisionTimer()
	timer.Start()
	operation()
	duration := timer.Stop()
	
	atomic.AddInt64(&bt.overhead, timer.GetOverhead())
	
	if len(bt.measurements) < bt.capacity {
		bt.measurements = append(bt.measurements, duration)
	}
	
	return duration
}

// GetMeasurements returns all measurements
func (bt *BatchTimer) GetMeasurements() []time.Duration {
	result := make([]time.Duration, len(bt.measurements))
	copy(result, bt.measurements)
	return result
}

// GetStatistics calculates statistics for all measurements
func (bt *BatchTimer) GetStatistics() TimerStatistics {
	if len(bt.measurements) == 0 {
		return TimerStatistics{}
	}
	
	var total time.Duration
	min := bt.measurements[0]
	max := bt.measurements[0]
	
	for _, d := range bt.measurements {
		total += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	
	return TimerStatistics{
		Count:    len(bt.measurements),
		Total:    total,
		Average:  total / time.Duration(len(bt.measurements)),
		Min:      min,
		Max:      max,
		Overhead: time.Duration(atomic.LoadInt64(&bt.overhead)),
	}
}

// Reset clears all measurements
func (bt *BatchTimer) Reset() {
	bt.measurements = bt.measurements[:0]
	atomic.StoreInt64(&bt.overhead, 0)
}

// TimerStatistics contains timing statistics
type TimerStatistics struct {
	Count    int
	Total    time.Duration
	Average  time.Duration
	Min      time.Duration
	Max      time.Duration
	Overhead time.Duration
}

// nanotime returns the current time in nanoseconds from the monotonic clock
// This uses the runtime's nanotime function for maximum precision
//
//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// MonotonicClock provides access to the monotonic clock
type MonotonicClock struct {
	baseTime int64
}

// NewMonotonicClock creates a new monotonic clock
func NewMonotonicClock() *MonotonicClock {
	return &MonotonicClock{
		baseTime: nanotime(),
	}
}

// Now returns the current monotonic time
func (mc *MonotonicClock) Now() time.Duration {
	return time.Duration(nanotime() - mc.baseTime)
}

// Since returns the duration since the given monotonic time
func (mc *MonotonicClock) Since(t time.Duration) time.Duration {
	return mc.Now() - t
}

// Sleep sleeps for the specified duration with high precision
func (mc *MonotonicClock) Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	
	start := nanotime()
	target := start + int64(d)
	
	// Spin-wait for very short durations (< 1ms)
	if d < time.Millisecond {
		for nanotime() < target {
			runtime.Gosched()
		}
	} else {
		// Use regular sleep for longer durations
		time.Sleep(d)
	}
}

// Benchmark runs a benchmark of an operation
func Benchmark(name string, iterations int, operation func()) BenchmarkResult {
	if iterations <= 0 {
		iterations = 1000
	}
	
	// Warm up
	for i := 0; i < min(10, iterations/10); i++ {
		operation()
	}
	
	// Force GC before benchmark
	runtime.GC()
	
	timer := NewBatchTimer(iterations)
	
	for i := 0; i < iterations; i++ {
		timer.Time(operation)
	}
	
	stats := timer.GetStatistics()
	
	return BenchmarkResult{
		Name:       name,
		Iterations: iterations,
		Total:      stats.Total,
		Average:    stats.Average,
		Min:        stats.Min,
		Max:        stats.Max,
		Overhead:   stats.Overhead,
	}
}

// BenchmarkResult contains benchmark results
type BenchmarkResult struct {
	Name       string
	Iterations int
	Total      time.Duration
	Average    time.Duration
	Min        time.Duration
	Max        time.Duration
	Overhead   time.Duration
}

// String returns a string representation of the benchmark result
func (br BenchmarkResult) String() string {
	return fmt.Sprintf("%s: %d iterations in %v (avg: %v, min: %v, max: %v)",
		br.Name, br.Iterations, br.Total, br.Average, br.Min, br.Max)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ValidateTimerAccuracy validates that the timer achieves 1ms accuracy
func ValidateTimerAccuracy() error {
	timer := NewPrecisionTimer()
	
	// Test with known duration
	timer.Start()
	time.Sleep(10 * time.Millisecond)
	duration := timer.Stop()
	
	// Check if measured duration is within 1ms of expected
	expected := 10 * time.Millisecond
	diff := duration - expected
	if diff < 0 {
		diff = -diff
	}
	
	if diff > time.Millisecond {
		return fmt.Errorf("timer accuracy validation failed: expected ~%v, got %v (diff: %v)",
			expected, duration, diff)
	}
	
	return nil
}