package types

import (
	"fmt"
	"time"
)

// CostBreakdown represents a detailed cost analysis with validation.
type CostBreakdown struct {
	ComputeCost      float64 `json:"compute_cost" yaml:"compute_cost"`
	NodeCost         float64 `json:"node_cost" yaml:"node_cost"`
	OverheadCost     float64 `json:"overhead_cost" yaml:"overhead_cost"`
	DataTransferCost float64 `json:"data_transfer_cost" yaml:"data_transfer_cost"`
	StorageCost      float64 `json:"storage_cost" yaml:"storage_cost"`
	TotalCost        float64 `json:"total_cost" yaml:"total_cost"`
}

// Validate ensures all cost components are non-negative and total is consistent.
func (c *CostBreakdown) Validate() error {
	costs := []struct {
		name  string
		value float64
	}{
		{"compute_cost", c.ComputeCost},
		{"node_cost", c.NodeCost},
		{"overhead_cost", c.OverheadCost},
		{"data_transfer_cost", c.DataTransferCost},
		{"storage_cost", c.StorageCost},
		{"total_cost", c.TotalCost},
	}

	for _, cost := range costs {
		if cost.value < 0 {
			return fmt.Errorf("%s cannot be negative: %f", cost.name, cost.value)
		}
	}

	expectedTotal := c.ComputeCost + c.NodeCost + c.OverheadCost + c.DataTransferCost + c.StorageCost
	if expectedTotal > 0 && c.TotalCost == 0 {
		c.TotalCost = expectedTotal
	}

	return nil
}

// CostPerHour returns the cost per hour based on job duration.
func (c *CostBreakdown) CostPerHour(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	hours := duration.Hours()
	if hours == 0 {
		return c.TotalCost
	}
	return c.TotalCost / hours
}

// PartitionAnalysis represents the analysis results for a compute partition.
type PartitionAnalysis struct {
	Name              string         `json:"name" yaml:"name"`
	Type              string         `json:"type" yaml:"type"` // "local" or "aws"
	QueueDepth        int            `json:"queue_depth" yaml:"queue_depth"`
	EstimatedWaitTime time.Duration  `json:"estimated_wait_time" yaml:"estimated_wait_time"`
	StartupTime       time.Duration  `json:"startup_time" yaml:"startup_time"`
	AvailableNodes    int            `json:"available_nodes" yaml:"available_nodes"`
	TotalNodes        int            `json:"total_nodes" yaml:"total_nodes"`
	EstimatedCost     *CostBreakdown `json:"estimated_cost" yaml:"estimated_cost"`

	// AWS-specific fields
	InstanceType string  `json:"instance_type,omitempty" yaml:"instance_type,omitempty"`
	CurrentPrice float64 `json:"current_price,omitempty" yaml:"current_price,omitempty"`

	// Local cluster specific fields
	ResourcesPerNode map[string]string `json:"resources_per_node,omitempty" yaml:"resources_per_node,omitempty"`
}

// IsAWS returns true if this is an AWS partition analysis.
func (p *PartitionAnalysis) IsAWS() bool {
	return p.Type == "aws"
}

// IsLocal returns true if this is a local cluster partition analysis.
func (p *PartitionAnalysis) IsLocal() bool {
	return p.Type == "local"
}

// TotalExecutionTime returns the total time to complete the job including wait time.
func (p *PartitionAnalysis) TotalExecutionTime(jobDuration time.Duration) time.Duration {
	return p.EstimatedWaitTime + p.StartupTime + jobDuration
}

// UtilizationRate returns the utilization rate of the partition (0.0 to 1.0).
func (p *PartitionAnalysis) UtilizationRate() float64 {
	if p.TotalNodes == 0 {
		return 0.0
	}
	usedNodes := p.TotalNodes - p.AvailableNodes
	return float64(usedNodes) / float64(p.TotalNodes)
}

// Validate checks if the partition analysis is valid.
func (p *PartitionAnalysis) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("partition name cannot be empty")
	}
	if p.Type != "local" && p.Type != "aws" {
		return fmt.Errorf("partition type must be 'local' or 'aws', got: %s", p.Type)
	}
	if p.QueueDepth < 0 {
		return fmt.Errorf("queue_depth cannot be negative: %d", p.QueueDepth)
	}
	if p.EstimatedWaitTime < 0 {
		return fmt.Errorf("estimated_wait_time cannot be negative: %v", p.EstimatedWaitTime)
	}
	if p.StartupTime < 0 {
		return fmt.Errorf("startup_time cannot be negative: %v", p.StartupTime)
	}
	if p.AvailableNodes < 0 {
		return fmt.Errorf("available_nodes cannot be negative: %d", p.AvailableNodes)
	}
	if p.TotalNodes < 0 {
		return fmt.Errorf("total_nodes cannot be negative: %d", p.TotalNodes)
	}
	if p.AvailableNodes > p.TotalNodes {
		return fmt.Errorf("available_nodes (%d) cannot exceed total_nodes (%d)", p.AvailableNodes, p.TotalNodes)
	}
	if p.EstimatedCost != nil {
		if err := p.EstimatedCost.Validate(); err != nil {
			return fmt.Errorf("invalid cost breakdown: %w", err)
		}
	}
	return nil
}

// DecisionWeights contains the weights for different decision factors.
type DecisionWeights struct {
	CostWeight           float64 `yaml:"cost_weight" json:"cost_weight"`
	TimeWeight           float64 `yaml:"time_weight" json:"time_weight"`
	TimeValuePerHour     float64 `yaml:"time_value_per_hour" json:"time_value_per_hour"`
	ReliabilityWeight    float64 `yaml:"reliability_weight" json:"reliability_weight"`
	SustainabilityWeight float64 `yaml:"sustainability_weight" json:"sustainability_weight"`
	RiskWeight           float64 `yaml:"risk_weight" json:"risk_weight"`
}

// Validate ensures all weights are within valid ranges.
func (w *DecisionWeights) Validate() error {
	weights := []struct {
		name  string
		value float64
		min   float64
		max   float64
	}{
		{"cost_weight", w.CostWeight, 0.0, 1.0},
		{"time_weight", w.TimeWeight, 0.0, 1.0},
		{"reliability_weight", w.ReliabilityWeight, 0.0, 1.0},
		{"sustainability_weight", w.SustainabilityWeight, 0.0, 1.0},
		{"risk_weight", w.RiskWeight, 0.0, 1.0},
		{"time_value_per_hour", w.TimeValuePerHour, 0.0, 10000.0}, // Up to $10k/hour
	}

	for _, weight := range weights {
		if weight.value < weight.min || weight.value > weight.max {
			return fmt.Errorf("%s must be between %f and %f, got: %f",
				weight.name, weight.min, weight.max, weight.value)
		}
	}

	return nil
}

// Normalize ensures weights are properly balanced for decision making.
func (w *DecisionWeights) Normalize() {
	total := w.CostWeight + w.TimeWeight + w.ReliabilityWeight + w.SustainabilityWeight + w.RiskWeight
	if total > 0 {
		w.CostWeight /= total
		w.TimeWeight /= total
		w.ReliabilityWeight /= total
		w.SustainabilityWeight /= total
		w.RiskWeight /= total
	}
}

// RecommendationType represents the type of recommendation.
type RecommendationType string

const (
	RecommendationLocal RecommendationType = "local"
	RecommendationAWS   RecommendationType = "aws"
)

// Recommendation represents the final recommendation with detailed reasoning.
type Recommendation struct {
	Preferred        RecommendationType `json:"preferred" yaml:"preferred"`
	TimeSavings      time.Duration      `json:"time_savings" yaml:"time_savings"`
	CostDifference   float64            `json:"cost_difference" yaml:"cost_difference"`
	BreakevenTime    time.Duration      `json:"breakeven_time" yaml:"breakeven_time"`
	Confidence       float64            `json:"confidence" yaml:"confidence"`
	Reasoning        []string           `json:"reasoning" yaml:"reasoning"`
	Score            float64            `json:"score" yaml:"score"`
	RiskAssessment   string             `json:"risk_assessment" yaml:"risk_assessment"`
	Sustainability   string             `json:"sustainability" yaml:"sustainability"`
}

// IsAWSRecommended returns true if AWS is the recommended option.
func (r *Recommendation) IsAWSRecommended() bool {
	return r.Preferred == RecommendationAWS
}

// IsLocalRecommended returns true if local execution is recommended.
func (r *Recommendation) IsLocalRecommended() bool {
	return r.Preferred == RecommendationLocal
}

// Validate checks if the recommendation is valid.
func (r *Recommendation) Validate() error {
	if r.Preferred != RecommendationLocal && r.Preferred != RecommendationAWS {
		return fmt.Errorf("preferred must be 'local' or 'aws', got: %s", r.Preferred)
	}
	if r.Confidence < 0.0 || r.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got: %f", r.Confidence)
	}
	if len(r.Reasoning) == 0 {
		return fmt.Errorf("at least one reasoning point must be provided")
	}
	return nil
}

// Analysis represents the complete analysis results.
type Analysis struct {
	TargetPartition *PartitionAnalysis `json:"target_partition" yaml:"target_partition"`
	BurstPartition  *PartitionAnalysis `json:"burst_partition" yaml:"burst_partition"`
	Recommendation  *Recommendation    `json:"recommendation" yaml:"recommendation"`
	Timestamp       time.Time          `json:"timestamp" yaml:"timestamp"`
	JobRequest      *JobRequest        `json:"job_request" yaml:"job_request"`
	Metadata        AnalysisMetadata   `json:"metadata" yaml:"metadata"`
}

// AnalysisMetadata contains metadata about the analysis process.
type AnalysisMetadata struct {
	Version       string        `json:"version" yaml:"version"`
	Duration      time.Duration `json:"duration" yaml:"duration"`
	DataSources   []string      `json:"data_sources" yaml:"data_sources"`
	Warnings      []string      `json:"warnings" yaml:"warnings"`
	ConfigUsed    string        `json:"config_used" yaml:"config_used"`
}

// Validate performs comprehensive validation of the analysis results.
func (a *Analysis) Validate() error {
	if a.TargetPartition == nil {
		return fmt.Errorf("target_partition is required")
	}
	if err := a.TargetPartition.Validate(); err != nil {
		return fmt.Errorf("invalid target partition: %w", err)
	}

	if a.BurstPartition == nil {
		return fmt.Errorf("burst_partition is required")
	}
	if err := a.BurstPartition.Validate(); err != nil {
		return fmt.Errorf("invalid burst partition: %w", err)
	}

	if a.Recommendation == nil {
		return fmt.Errorf("recommendation is required")
	}
	if err := a.Recommendation.Validate(); err != nil {
		return fmt.Errorf("invalid recommendation: %w", err)
	}

	if a.JobRequest == nil {
		return fmt.Errorf("job_request is required")
	}
	if err := a.JobRequest.Validate(); err != nil {
		return fmt.Errorf("invalid job request: %w", err)
	}

	if a.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	return nil
}

// IsStale returns true if the analysis is older than the specified duration.
func (a *Analysis) IsStale(maxAge time.Duration) bool {
	return time.Since(a.Timestamp) > maxAge
}