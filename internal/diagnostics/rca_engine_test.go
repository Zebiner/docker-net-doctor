package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RCA Engine core interfaces and types

// RCACategory represents categories of root cause analysis
type RCACategory string

const (
	CategoryConnectivity     RCACategory = "connectivity"
	CategoryServiceDiscovery RCACategory = "service_discovery"
	CategorySecurity         RCACategory = "security"
	CategoryPerformance      RCACategory = "performance"
	CategoryConfiguration    RCACategory = "configuration"
)

// RCASeverity represents the severity of identified patterns
type RCASeverity int

const (
	RCASeverityLow RCASeverity = iota
	RCASeverityMedium
	RCASeverityHigh
	RCASeverityCritical
)

// RCARelationshipType represents how issues are related
type RCARelationshipType string

const (
	RelationshipCausal      RCARelationshipType = "causal"       // A causes B
	RelationshipCorrelated  RCARelationshipType = "correlated"   // A and B occur together
	RelationshipIndependent RCARelationshipType = "independent" // A and B are unrelated
	RelationshipChain       RCARelationshipType = "chain"       // A -> B -> C pattern
)

// RCAImpact represents the impact level of correlated issues
type RCAImpact string

const (
	ImpactLow      RCAImpact = "low"
	ImpactMedium   RCAImpact = "medium"
	ImpactHigh     RCAImpact = "high"
	ImpactCritical RCAImpact = "critical"
)

// RCAEngine interface defines the core root cause analysis functionality
type RCAEngine interface {
	AnalyzeResults(ctx context.Context, results []CheckResult) (*RCAReport, error)
	RegisterPattern(pattern RCAPattern) error
	GetRecommendations(analysis *RCAAnalysis) ([]RCARecommendation, error)
	CorrelateIssues(issues []RCAIssue) (*RCACorrelation, error)
}

// RCAReport represents the complete analysis report
type RCAReport struct {
	Analysis        *RCAAnalysis          `json:"analysis"`
	Correlations    []RCACorrelation      `json:"correlations"`
	Recommendations []RCARecommendation   `json:"recommendations"`
	Confidence      float64               `json:"confidence"`
	ExecutionTime   time.Duration         `json:"execution_time"`
	Patterns        []string              `json:"detected_patterns"`
	IssueCount      int                   `json:"issue_count"`
	CriticalPath    []string              `json:"critical_path"`
}

// RCAAnalysis represents the detailed analysis of check results
type RCAAnalysis struct {
	TotalChecks        int                        `json:"total_checks"`
	FailedChecks       int                        `json:"failed_checks"`
	WarningChecks      int                        `json:"warning_checks"`
	Categories         map[RCACategory]int        `json:"categories"`
	SeverityDistribution map[RCASeverity]int      `json:"severity_distribution"`
	IssuesByCategory   map[RCACategory][]RCAIssue `json:"issues_by_category"`
	Timestamp          time.Time                  `json:"timestamp"`
}

// RCAPattern defines a recognizable pattern in diagnostic results
type RCAPattern struct {
	Name        string             `json:"name"`
	Category    RCACategory        `json:"category"`
	Severity    RCASeverity        `json:"severity"`
	Conditions  []RCACondition     `json:"conditions"`
	Handler     RCAPatternHandler  `json:"-"`
	Description string             `json:"description"`
	Priority    int                `json:"priority"`
}

// RCACondition defines a condition that must be met for pattern recognition
type RCACondition struct {
	CheckName   string      `json:"check_name"`
	Status      Status      `json:"status"`
	Severity    Severity    `json:"severity"`
	DetailsKey  string      `json:"details_key,omitempty"`
	DetailsValue interface{} `json:"details_value,omitempty"`
}

// RCAPatternHandler is a function that handles matched patterns
type RCAPatternHandler func(results []CheckResult) (*RCAPatternMatch, error)

// RCAPatternMatch represents a matched pattern with details
type RCAPatternMatch struct {
	Pattern     RCAPattern    `json:"pattern"`
	MatchedResults []CheckResult `json:"matched_results"`
	Confidence  float64       `json:"confidence"`
	Evidence    []string      `json:"evidence"`
}

// RCAIssue represents an identified issue from analysis
type RCAIssue struct {
	ID           string      `json:"id"`
	CheckName    string      `json:"check_name"`
	Category     RCACategory `json:"category"`
	Severity     RCASeverity `json:"severity"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Impact       RCAImpact   `json:"impact"`
	Evidence     []string    `json:"evidence"`
	AffectedResources []string `json:"affected_resources"`
}

// RCACorrelation represents correlated issues
type RCACorrelation struct {
	ID           string              `json:"id"`
	Issues       []RCAIssue          `json:"issues"`
	Relationship RCARelationshipType `json:"relationship"`
	Confidence   float64             `json:"confidence"`
	Impact       RCAImpact           `json:"impact"`
	Description  string              `json:"description"`
	RootCause    *RCAIssue           `json:"root_cause,omitempty"`
}

// RCARecommendation represents actionable recommendations
type RCARecommendation struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Category    RCACategory `json:"category"`
	Priority    int         `json:"priority"`
	Actions     []RCAAction `json:"actions"`
	Impact      RCAImpact   `json:"impact"`
	Effort      string      `json:"effort"` // "low", "medium", "high"
	RelatedIssues []string  `json:"related_issues"`
}

// RCAAction represents a specific action to take
type RCAAction struct {
	Type        string `json:"type"` // "command", "config", "architectural"
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	ConfigFile  string `json:"config_file,omitempty"`
	ConfigChange string `json:"config_change,omitempty"`
}

// Mock RCA Engine for testing
type mockRCAEngine struct {
	patterns     []RCAPattern
	analysisErr  error
	patternErr   error
	recErr       error
	correlateErr error
	analysisTime time.Duration
	report       *RCAReport
}

func (m *mockRCAEngine) AnalyzeResults(ctx context.Context, results []CheckResult) (*RCAReport, error) {
	if m.analysisErr != nil {
		return nil, m.analysisErr
	}
	
	// Simulate analysis time
	time.Sleep(m.analysisTime)
	
	if m.report != nil {
		return m.report, nil
	}
	
	// Default mock report
	return &RCAReport{
		Analysis: &RCAAnalysis{
			TotalChecks:   len(results),
			FailedChecks:  countFailedChecks(results),
			WarningChecks: countWarningChecks(results),
			Categories:    map[RCACategory]int{},
			Timestamp:     time.Now(),
		},
		Correlations:    []RCACorrelation{},
		Recommendations: []RCARecommendation{},
		Confidence:      0.85,
		ExecutionTime:   m.analysisTime,
		Patterns:        []string{},
		IssueCount:      len(results),
	}, nil
}

func (m *mockRCAEngine) RegisterPattern(pattern RCAPattern) error {
	if m.patternErr != nil {
		return m.patternErr
	}
	m.patterns = append(m.patterns, pattern)
	return nil
}

func (m *mockRCAEngine) GetRecommendations(analysis *RCAAnalysis) ([]RCARecommendation, error) {
	if m.recErr != nil {
		return nil, m.recErr
	}
	return []RCARecommendation{}, nil
}

func (m *mockRCAEngine) CorrelateIssues(issues []RCAIssue) (*RCACorrelation, error) {
	if m.correlateErr != nil {
		return nil, m.correlateErr
	}
	return &RCACorrelation{
		ID:           "test-correlation",
		Issues:       issues,
		Relationship: RelationshipCorrelated,
		Confidence:   0.8,
		Impact:       ImpactMedium,
	}, nil
}

// Helper functions
func countFailedChecks(results []CheckResult) int {
	count := 0
	for _, result := range results {
		if result.Status == StatusFail || result.Status == StatusError {
			count++
		}
	}
	return count
}

func countWarningChecks(results []CheckResult) int {
	count := 0
	for _, result := range results {
		if result.Status == StatusWarning {
			count++
		}
	}
	return count
}

// Test mock check results for various scenarios
var (
	mockDNSFailureResult = CheckResult{
		Name:      "dns_resolution",
		Status:    StatusFail,
		Message:   "DNS resolution failed for external domains",
		Timestamp: time.Now(),
		Duration:  50 * time.Millisecond,
		Details: map[string]interface{}{
			"failed_domains": []string{"google.com", "docker.io"},
			"dns_servers":    []string{"8.8.8.8", "1.1.1.1"},
			"error_type":     "timeout",
		},
		Suggestions: []string{"Check network connectivity", "Verify DNS server configuration"},
	}

	mockConnectivityFailureResult = CheckResult{
		Name:      "container_connectivity",
		Status:    StatusFail,
		Message:   "Containers cannot communicate across networks",
		Timestamp: time.Now(),
		Duration:  75 * time.Millisecond,
		Details: map[string]interface{}{
			"source_container": "web-1",
			"target_container": "db-1",
			"network_bridge":   "custom-net",
			"ping_result":      "timeout",
		},
		Suggestions: []string{"Check iptables rules", "Verify network bridge configuration"},
	}

	mockServiceDiscoveryWarningResult = CheckResult{
		Name:      "service_discovery",
		Status:    StatusWarning,
		Message:   "Service discovery configuration incomplete",
		Timestamp: time.Now(),
		Duration:  30 * time.Millisecond,
		Details: map[string]interface{}{
			"missing_load_balancer": true,
			"health_checks":         false,
			"service_mesh":          []string{},
		},
		Suggestions: []string{"Configure load balancer", "Add health checks"},
	}

	mockSecurityFailureResult = CheckResult{
		Name:      "namespace_check",
		Status:    StatusFail,
		Message:   "Security violations detected in container namespace configuration",
		Timestamp: time.Now(),
		Duration:  40 * time.Millisecond,
		Details: map[string]interface{}{
			"privileged_containers": []string{"app-1", "worker-2"},
			"host_network_mode":     []string{"monitoring"},
			"security_risks":        []string{"host networking bypasses namespace isolation"},
		},
		Suggestions: []string{"Remove privileged mode", "Use custom networks instead of host"},
	}

	mockPerformanceWarningResult = CheckResult{
		Name:      "mtu_consistency",
		Status:    StatusWarning,
		Message:   "MTU size inconsistencies detected",
		Timestamp: time.Now(),
		Duration:  25 * time.Millisecond,
		Details: map[string]interface{}{
			"mtu_sizes":      map[string]int{"eth0": 1500, "docker0": 1450},
			"inconsistency":  true,
			"affected_networks": []string{"bridge", "custom-net"},
		},
		Suggestions: []string{"Standardize MTU sizes across networks"},
	}
)

// Test RCA Engine Interface Implementation
func TestRCAEngine_Interface(t *testing.T) {
	engine := &mockRCAEngine{}
	
	// Verify it implements RCAEngine interface
	var _ RCAEngine = engine
	
	ctx := context.Background()
	
	// Test basic interface methods exist and are callable
	results := []CheckResult{mockDNSFailureResult}
	report, err := engine.AnalyzeResults(ctx, results)
	assert.NoError(t, err)
	assert.NotNil(t, report)
	
	pattern := RCAPattern{Name: "test", Category: CategoryConnectivity}
	err = engine.RegisterPattern(pattern)
	assert.NoError(t, err)
	
	recs, err := engine.GetRecommendations(report.Analysis)
	assert.NoError(t, err)
	assert.NotNil(t, recs)
	
	issues := []RCAIssue{{ID: "test", CheckName: "test"}}
	correlation, err := engine.CorrelateIssues(issues)
	assert.NoError(t, err)
	assert.NotNil(t, correlation)
}

// Test Connectivity Pattern Recognition
func TestRCAEngine_ConnectivityPattern(t *testing.T) {
	engine := &mockRCAEngine{
		report: &RCAReport{
			Analysis: &RCAAnalysis{
				TotalChecks:   2,
				FailedChecks:  2,
				WarningChecks: 0,
				Categories: map[RCACategory]int{
					CategoryConnectivity: 2,
				},
			},
			Correlations: []RCACorrelation{
				{
					ID:           "connectivity-pattern",
					Issues:       []RCAIssue{},
					Relationship: RelationshipCausal,
					Confidence:   0.9,
					Impact:       ImpactHigh,
					Description:  "DNS failure causing container connectivity issues",
				},
			},
			Recommendations: []RCARecommendation{
				{
					ID:          "fix-dns",
					Title:       "Fix DNS Resolution",
					Description: "Resolve DNS configuration to restore connectivity",
					Category:    CategoryConnectivity,
					Priority:    1,
					Impact:      ImpactHigh,
				},
			},
			Confidence:   0.9,
			Patterns:     []string{"connectivity-failure-pattern"},
			IssueCount:   2,
			CriticalPath: []string{"dns_resolution", "container_connectivity"},
		},
	}
	
	ctx := context.Background()
	results := []CheckResult{mockDNSFailureResult, mockConnectivityFailureResult}
	
	report, err := engine.AnalyzeResults(ctx, results)
	require.NoError(t, err)
	require.NotNil(t, report)
	
	// Verify connectivity pattern recognition
	assert.Equal(t, 2, report.Analysis.TotalChecks)
	assert.Equal(t, 2, report.Analysis.FailedChecks)
	assert.Greater(t, report.Confidence, 0.8)
	assert.Contains(t, report.Patterns, "connectivity-failure-pattern")
	assert.Len(t, report.Correlations, 1)
	assert.Equal(t, RelationshipCausal, report.Correlations[0].Relationship)
}

// Test Service Discovery Pattern Recognition
func TestRCAEngine_ServiceDiscoveryPattern(t *testing.T) {
	engine := &mockRCAEngine{
		report: &RCAReport{
			Analysis: &RCAAnalysis{
				TotalChecks:   2,
				FailedChecks:  0,
				WarningChecks: 2,
				Categories: map[RCACategory]int{
					CategoryServiceDiscovery: 2,
				},
			},
			Correlations: []RCACorrelation{
				{
					ID:           "service-discovery-gap",
					Relationship: RelationshipCorrelated,
					Confidence:   0.85,
					Impact:       ImpactMedium,
					Description:  "Missing service discovery infrastructure",
				},
			},
			Recommendations: []RCARecommendation{
				{
					ID:       "add-load-balancer",
					Title:    "Configure Load Balancer",
					Category: CategoryServiceDiscovery,
					Priority: 2,
					Actions: []RCAAction{
						{
							Type:        "command",
							Description: "Deploy Traefik load balancer",
							Command:     "docker run -d traefik:latest",
						},
					},
				},
			},
			Confidence: 0.85,
			Patterns:   []string{"service-discovery-incomplete"},
		},
	}
	
	ctx := context.Background()
	results := []CheckResult{mockServiceDiscoveryWarningResult, mockPerformanceWarningResult}
	
	report, err := engine.AnalyzeResults(ctx, results)
	require.NoError(t, err)
	require.NotNil(t, report)
	
	// Verify service discovery pattern
	assert.Equal(t, 2, report.Analysis.WarningChecks)
	assert.Contains(t, report.Patterns, "service-discovery-incomplete")
	assert.Len(t, report.Recommendations, 1)
	assert.Equal(t, CategoryServiceDiscovery, report.Recommendations[0].Category)
	assert.Len(t, report.Recommendations[0].Actions, 1)
}

// Test Security Pattern Recognition
func TestRCAEngine_SecurityPattern(t *testing.T) {
	engine := &mockRCAEngine{
		report: &RCAReport{
			Analysis: &RCAAnalysis{
				TotalChecks:   1,
				FailedChecks:  1,
				WarningChecks: 0,
				Categories: map[RCACategory]int{
					CategorySecurity: 1,
				},
				SeverityDistribution: map[RCASeverity]int{
					RCASeverityCritical: 1,
				},
			},
			Correlations: []RCACorrelation{
				{
					ID:           "security-violation",
					Relationship: RelationshipIndependent,
					Confidence:   0.95,
					Impact:       ImpactCritical,
					Description:  "Multiple security violations detected",
				},
			},
			Recommendations: []RCARecommendation{
				{
					ID:       "security-hardening",
					Title:    "Implement Security Hardening",
					Category: CategorySecurity,
					Priority: 1, // Highest priority for security
					Impact:   ImpactCritical,
					Actions: []RCAAction{
						{
							Type:        "config",
							Description: "Remove privileged mode from containers",
							ConfigFile:  "docker-compose.yml",
							ConfigChange: "Remove 'privileged: true' from all services",
						},
					},
				},
			},
			Confidence: 0.95,
			Patterns:   []string{"security-violation-pattern"},
		},
	}
	
	ctx := context.Background()
	results := []CheckResult{mockSecurityFailureResult}
	
	report, err := engine.AnalyzeResults(ctx, results)
	require.NoError(t, err)
	
	// Verify security pattern recognition
	assert.Contains(t, report.Patterns, "security-violation-pattern")
	assert.Equal(t, ImpactCritical, report.Correlations[0].Impact)
	assert.Equal(t, 1, report.Recommendations[0].Priority) // Highest priority
	assert.Greater(t, report.Confidence, 0.9) // High confidence for security issues
}

// Test Multi-Pattern Complex Analysis
func TestRCAEngine_ComplexMultiPattern(t *testing.T) {
	engine := &mockRCAEngine{
		report: &RCAReport{
			Analysis: &RCAAnalysis{
				TotalChecks:   4,
				FailedChecks:  3,
				WarningChecks: 1,
				Categories: map[RCACategory]int{
					CategoryConnectivity:     2,
					CategoryServiceDiscovery: 1,
					CategorySecurity:         1,
				},
			},
			Correlations: []RCACorrelation{
				{
					ID:           "complex-system-failure",
					Relationship: RelationshipChain,
					Confidence:   0.87,
					Impact:       ImpactCritical,
					Description:  "Cascading failures across multiple systems",
				},
			},
			Recommendations: []RCARecommendation{
				{
					ID:       "systematic-fix",
					Title:    "Systematic Infrastructure Fix",
					Category: CategoryConfiguration,
					Priority: 1,
					Actions: []RCAAction{
						{Type: "architectural", Description: "Redesign network architecture"},
						{Type: "config", Description: "Update security configurations"},
						{Type: "command", Description: "Restart networking services"},
					},
				},
			},
			Confidence:   0.87,
			Patterns:     []string{"multi-system-failure", "cascading-errors"},
			CriticalPath: []string{"security", "dns", "connectivity", "service_discovery"},
		},
	}
	
	ctx := context.Background()
	results := []CheckResult{
		mockDNSFailureResult,
		mockConnectivityFailureResult,
		mockServiceDiscoveryWarningResult,
		mockSecurityFailureResult,
	}
	
	report, err := engine.AnalyzeResults(ctx, results)
	require.NoError(t, err)
	
	// Verify complex pattern recognition
	assert.Equal(t, 4, report.Analysis.TotalChecks)
	assert.Equal(t, 3, report.Analysis.FailedChecks)
	assert.Contains(t, report.Patterns, "multi-system-failure")
	assert.Contains(t, report.Patterns, "cascading-errors")
	assert.Equal(t, RelationshipChain, report.Correlations[0].Relationship)
	assert.Len(t, report.CriticalPath, 4)
	assert.Len(t, report.Recommendations[0].Actions, 3) // Multiple coordinated actions
}

// Test Performance Requirements
func TestRCAEngine_Performance(t *testing.T) {
	// Test pattern analysis performance <50ms
	engine := &mockRCAEngine{
		analysisTime: 40 * time.Millisecond, // Under 50ms requirement
	}
	
	ctx := context.Background()
	results := make([]CheckResult, 10) // 10 check results
	for i := range results {
		results[i] = mockDNSFailureResult
	}
	
	start := time.Now()
	report, err := engine.AnalyzeResults(ctx, results)
	duration := time.Since(start)
	
	require.NoError(t, err)
	require.NotNil(t, report)
	
	// Verify performance requirements
	assert.Less(t, duration, 50*time.Millisecond, "Pattern analysis must complete within 50ms")
	assert.Less(t, report.ExecutionTime, 50*time.Millisecond, "Reported execution time must be under 50ms")
}

// Test Correlation Performance
func TestRCAEngine_CorrelationPerformance(t *testing.T) {
	engine := &mockRCAEngine{
		analysisTime: 180 * time.Millisecond, // Under 200ms requirement for complex correlation
	}
	
	ctx := context.Background()
	// Complex scenario with multiple patterns
	results := []CheckResult{
		mockDNSFailureResult,
		mockConnectivityFailureResult,
		mockServiceDiscoveryWarningResult,
		mockSecurityFailureResult,
		mockPerformanceWarningResult,
	}
	
	start := time.Now()
	report, err := engine.AnalyzeResults(ctx, results)
	duration := time.Since(start)
	
	require.NoError(t, err)
	require.NotNil(t, report)
	
	// Verify correlation performance requirements
	assert.Less(t, duration, 200*time.Millisecond, "Complex correlation analysis must complete within 200ms")
}

// Test Error Handling
func TestRCAEngine_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		engine      *mockRCAEngine
		results     []CheckResult
		expectError bool
	}{
		{
			name:        "nil results",
			engine:      &mockRCAEngine{},
			results:     nil,
			expectError: true,
		},
		{
			name:        "empty results",
			engine:      &mockRCAEngine{},
			results:     []CheckResult{},
			expectError: true,
		},
		{
			name:        "analysis error",
			engine:      &mockRCAEngine{analysisErr: assert.AnError},
			results:     []CheckResult{mockDNSFailureResult},
			expectError: true,
		},
		{
			name:        "pattern registration error",
			engine:      &mockRCAEngine{patternErr: assert.AnError},
			results:     []CheckResult{mockDNSFailureResult},
			expectError: false, // Analysis should still work
		},
	}
	
	ctx := context.Background()
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := tt.engine.AnalyzeResults(ctx, tt.results)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, report)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, report)
			}
		})
	}
}

// Test Context Cancellation
func TestRCAEngine_ContextCancellation(t *testing.T) {
	engine := &mockRCAEngine{
		analysisTime: 1 * time.Second, // Long running analysis
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	results := []CheckResult{mockDNSFailureResult}
	
	_, err := engine.AnalyzeResults(ctx, results)
	
	// Should timeout due to context cancellation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

// Test Pattern Registration and Management
func TestRCAEngine_PatternRegistration(t *testing.T) {
	engine := &mockRCAEngine{}
	
	// Test successful pattern registration
	pattern := RCAPattern{
		Name:        "test-connectivity-pattern",
		Category:    CategoryConnectivity,
		Severity:    RCASeverityHigh,
		Description: "Test pattern for connectivity issues",
		Priority:    1,
		Conditions: []RCACondition{
			{
				CheckName: "dns_resolution",
				Status:    StatusFail,
				Severity:  SeverityError,
			},
			{
				CheckName: "container_connectivity",
				Status:    StatusFail,
				Severity:  SeverityError,
			},
		},
	}
	
	err := engine.RegisterPattern(pattern)
	assert.NoError(t, err)
	assert.Len(t, engine.patterns, 1)
	assert.Equal(t, "test-connectivity-pattern", engine.patterns[0].Name)
}

// Test Recommendation Generation
func TestRCAEngine_RecommendationGeneration(t *testing.T) {
	engine := &mockRCAEngine{}
	
	analysis := &RCAAnalysis{
		TotalChecks:   3,
		FailedChecks:  2,
		WarningChecks: 1,
		Categories: map[RCACategory]int{
			CategoryConnectivity: 2,
			CategorySecurity:     1,
		},
	}
	
	recommendations, err := engine.GetRecommendations(analysis)
	require.NoError(t, err)
	assert.NotNil(t, recommendations)
	
	// Test with error condition
	engine.recErr = assert.AnError
	recommendations, err = engine.GetRecommendations(analysis)
	assert.Error(t, err)
	assert.Nil(t, recommendations)
}

// Test Issue Correlation
func TestRCAEngine_IssueCorrelation(t *testing.T) {
	engine := &mockRCAEngine{}
	
	issues := []RCAIssue{
		{
			ID:          "dns-issue",
			CheckName:   "dns_resolution",
			Category:    CategoryConnectivity,
			Severity:    RCASeverityHigh,
			Title:       "DNS Resolution Failure",
			Description: "Cannot resolve external domains",
			Impact:      ImpactHigh,
		},
		{
			ID:          "connectivity-issue",
			CheckName:   "container_connectivity",
			Category:    CategoryConnectivity,
			Severity:    RCASeverityHigh,
			Title:       "Container Connectivity Failure",
			Description: "Containers cannot communicate",
			Impact:      ImpactHigh,
		},
	}
	
	correlation, err := engine.CorrelateIssues(issues)
	require.NoError(t, err)
	require.NotNil(t, correlation)
	
	assert.Equal(t, "test-correlation", correlation.ID)
	assert.Len(t, correlation.Issues, 2)
	assert.Equal(t, RelationshipCorrelated, correlation.Relationship)
	assert.Equal(t, 0.8, correlation.Confidence)
}

// Benchmark tests
func BenchmarkRCAEngine_PatternRecognition(b *testing.B) {
	engine := &mockRCAEngine{}
	ctx := context.Background()
	results := []CheckResult{
		mockDNSFailureResult,
		mockConnectivityFailureResult,
		mockServiceDiscoveryWarningResult,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.AnalyzeResults(ctx, results)
	}
}

func BenchmarkRCAEngine_ComplexCorrelation(b *testing.B) {
	engine := &mockRCAEngine{}
	ctx := context.Background()
	results := []CheckResult{
		mockDNSFailureResult,
		mockConnectivityFailureResult,
		mockServiceDiscoveryWarningResult,
		mockSecurityFailureResult,
		mockPerformanceWarningResult,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.AnalyzeResults(ctx, results)
	}
}