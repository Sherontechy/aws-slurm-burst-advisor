package types

import (
	"testing"
	"time"
)

func TestCostBreakdown_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cost      CostBreakdown
		wantError bool
	}{
		{
			name: "valid cost breakdown",
			cost: CostBreakdown{
				ComputeCost: 10.0,
				NodeCost:    5.0,
				TotalCost:   15.0,
			},
			wantError: false,
		},
		{
			name: "negative compute cost",
			cost: CostBreakdown{
				ComputeCost: -10.0,
				NodeCost:    5.0,
				TotalCost:   15.0,
			},
			wantError: true,
		},
		{
			name: "auto-calculate total cost",
			cost: CostBreakdown{
				ComputeCost: 10.0,
				NodeCost:    5.0,
				TotalCost:   0.0, // Should be auto-calculated
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cost.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("CostBreakdown.Validate() error = %v, wantError %v", err, tt.wantError)
			}

			// Check auto-calculation
			if !tt.wantError && tt.name == "auto-calculate total cost" {
				if tt.cost.TotalCost != 15.0 {
					t.Errorf("Expected TotalCost to be auto-calculated to 15.0, got %f", tt.cost.TotalCost)
				}
			}
		})
	}
}

func TestCostBreakdown_CostPerHour(t *testing.T) {
	cost := CostBreakdown{TotalCost: 20.0}

	tests := []struct {
		name     string
		duration time.Duration
		want     float64
	}{
		{
			name:     "2 hours",
			duration: 2 * time.Hour,
			want:     10.0,
		},
		{
			name:     "30 minutes",
			duration: 30 * time.Minute,
			want:     40.0,
		},
		{
			name:     "zero duration",
			duration: 0,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cost.CostPerHour(tt.duration)
			if got != tt.want {
				t.Errorf("CostBreakdown.CostPerHour() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestPartitionAnalysis_IsAWS(t *testing.T) {
	tests := []struct {
		name      string
		partition PartitionAnalysis
		want      bool
	}{
		{
			name:      "AWS partition",
			partition: PartitionAnalysis{Type: "aws"},
			want:      true,
		},
		{
			name:      "local partition",
			partition: PartitionAnalysis{Type: "local"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.partition.IsAWS()
			if got != tt.want {
				t.Errorf("PartitionAnalysis.IsAWS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartitionAnalysis_TotalExecutionTime(t *testing.T) {
	partition := PartitionAnalysis{
		EstimatedWaitTime: 30 * time.Minute,
		StartupTime:       5 * time.Minute,
	}

	jobDuration := 2 * time.Hour
	expected := 30*time.Minute + 5*time.Minute + 2*time.Hour

	got := partition.TotalExecutionTime(jobDuration)
	if got != expected {
		t.Errorf("PartitionAnalysis.TotalExecutionTime() = %v, want %v", got, expected)
	}
}

func TestPartitionAnalysis_UtilizationRate(t *testing.T) {
	tests := []struct {
		name      string
		partition PartitionAnalysis
		want      float64
	}{
		{
			name: "50% utilization",
			partition: PartitionAnalysis{
				TotalNodes:     10,
				AvailableNodes: 5,
			},
			want: 0.5,
		},
		{
			name: "zero nodes",
			partition: PartitionAnalysis{
				TotalNodes:     0,
				AvailableNodes: 0,
			},
			want: 0.0,
		},
		{
			name: "fully utilized",
			partition: PartitionAnalysis{
				TotalNodes:     10,
				AvailableNodes: 0,
			},
			want: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.partition.UtilizationRate()
			if got != tt.want {
				t.Errorf("PartitionAnalysis.UtilizationRate() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestPartitionAnalysis_Validate(t *testing.T) {
	tests := []struct {
		name      string
		partition PartitionAnalysis
		wantError bool
	}{
		{
			name: "valid partition analysis",
			partition: PartitionAnalysis{
				Name:              "gpu",
				Type:              "aws",
				QueueDepth:        5,
				EstimatedWaitTime: 10 * time.Minute,
				StartupTime:       2 * time.Minute,
				AvailableNodes:    8,
				TotalNodes:        10,
				EstimatedCost:     &CostBreakdown{TotalCost: 50.0},
			},
			wantError: false,
		},
		{
			name: "empty name",
			partition: PartitionAnalysis{
				Type: "aws",
			},
			wantError: true,
		},
		{
			name: "invalid type",
			partition: PartitionAnalysis{
				Name: "gpu",
				Type: "invalid",
			},
			wantError: true,
		},
		{
			name: "negative queue depth",
			partition: PartitionAnalysis{
				Name:       "gpu",
				Type:       "aws",
				QueueDepth: -1,
			},
			wantError: true,
		},
		{
			name: "available > total nodes",
			partition: PartitionAnalysis{
				Name:           "gpu",
				Type:           "aws",
				AvailableNodes: 15,
				TotalNodes:     10,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.partition.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("PartitionAnalysis.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDecisionWeights_Validate(t *testing.T) {
	tests := []struct {
		name      string
		weights   DecisionWeights
		wantError bool
	}{
		{
			name: "valid weights",
			weights: DecisionWeights{
				CostWeight:       0.3,
				TimeWeight:       0.7,
				TimeValuePerHour: 50.0,
			},
			wantError: false,
		},
		{
			name: "cost weight too high",
			weights: DecisionWeights{
				CostWeight: 1.5,
				TimeWeight: 0.5,
			},
			wantError: true,
		},
		{
			name: "negative time value",
			weights: DecisionWeights{
				CostWeight:       0.3,
				TimeWeight:       0.7,
				TimeValuePerHour: -10.0,
			},
			wantError: true,
		},
		{
			name: "extremely high time value",
			weights: DecisionWeights{
				CostWeight:       0.3,
				TimeWeight:       0.7,
				TimeValuePerHour: 20000.0,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.weights.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("DecisionWeights.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDecisionWeights_Normalize(t *testing.T) {
	weights := DecisionWeights{
		CostWeight:       0.6,
		TimeWeight:       0.8,
		ReliabilityWeight: 0.6,
	}

	weights.Normalize()

	total := weights.CostWeight + weights.TimeWeight + weights.ReliabilityWeight
	tolerance := 0.0001

	if total < 1.0-tolerance || total > 1.0+tolerance {
		t.Errorf("Normalized weights don't sum to 1.0, got %f", total)
	}
}

func TestRecommendation_Validate(t *testing.T) {
	tests := []struct {
		name           string
		recommendation Recommendation
		wantError      bool
	}{
		{
			name: "valid recommendation",
			recommendation: Recommendation{
				Preferred:  RecommendationAWS,
				Confidence: 0.85,
				Reasoning:  []string{"AWS is faster", "Lower cost"},
			},
			wantError: false,
		},
		{
			name: "invalid preferred type",
			recommendation: Recommendation{
				Preferred:  "invalid",
				Confidence: 0.85,
				Reasoning:  []string{"Some reason"},
			},
			wantError: true,
		},
		{
			name: "confidence too high",
			recommendation: Recommendation{
				Preferred:  RecommendationLocal,
				Confidence: 1.5,
				Reasoning:  []string{"Some reason"},
			},
			wantError: true,
		},
		{
			name: "no reasoning",
			recommendation: Recommendation{
				Preferred:  RecommendationLocal,
				Confidence: 0.75,
				Reasoning:  []string{},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.recommendation.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Recommendation.Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestRecommendation_IsAWSRecommended(t *testing.T) {
	tests := []struct {
		name           string
		recommendation Recommendation
		want           bool
	}{
		{
			name:           "AWS recommended",
			recommendation: Recommendation{Preferred: RecommendationAWS},
			want:           true,
		},
		{
			name:           "local recommended",
			recommendation: Recommendation{Preferred: RecommendationLocal},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.recommendation.IsAWSRecommended()
			if got != tt.want {
				t.Errorf("Recommendation.IsAWSRecommended() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalysis_IsStale(t *testing.T) {
	now := time.Now()
	analysis := Analysis{
		Timestamp: now.Add(-30 * time.Minute),
	}

	tests := []struct {
		name   string
		maxAge time.Duration
		want   bool
	}{
		{
			name:   "is stale",
			maxAge: 15 * time.Minute,
			want:   true,
		},
		{
			name:   "not stale",
			maxAge: 1 * time.Hour,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analysis.IsStale(tt.maxAge)
			if got != tt.want {
				t.Errorf("Analysis.IsStale() = %v, want %v", got, tt.want)
			}
		})
	}
}