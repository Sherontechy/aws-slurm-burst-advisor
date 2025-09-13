package analyzer

import (
	"fmt"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/config"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// DecisionEngine analyzes and compares different execution options
type DecisionEngine struct {
	weights types.DecisionWeights
}

// NewDecisionEngine creates a new decision engine with given weights
func NewDecisionEngine(weights types.DecisionWeights) *DecisionEngine {
	return &DecisionEngine{
		weights: weights,
	}
}

// Compare analyzes local vs AWS options and generates a recommendation
func (e *DecisionEngine) Compare(local, aws *types.PartitionAnalysis, job *types.JobRequest) *types.Recommendation {
	// Calculate total time for each option
	localTime := local.EstimatedWaitTime + job.TimeLimit
	awsTime := aws.StartupTime + job.TimeLimit

	// Calculate cost difference (positive = AWS more expensive)
	costDiff := aws.EstimatedCost.TotalCost - local.EstimatedCost.TotalCost

	// Calculate time difference (positive = AWS saves time)
	timeSavings := localTime - awsTime

	// Score each option
	localScore := e.calculateScore(local, job)
	awsScore := e.calculateScore(aws, job)

	// Determine preference
	preferred := types.RecommendationLocal
	if awsScore > localScore {
		preferred = types.RecommendationAWS
	}

	// Calculate confidence based on score difference
	confidence := e.calculateConfidence(localScore, awsScore)

	// Generate reasoning
	reasoning := e.generateReasoning(local, aws, job, costDiff, timeSavings)

	return &types.Recommendation{
		Preferred:      preferred,
		TimeSavings:    timeSavings,
		CostDifference: costDiff,
		Confidence:     confidence,
		Reasoning:      reasoning,
		Score:          awsScore - localScore,
	}
}

// calculateScore calculates a composite score for an execution option
func (e *DecisionEngine) calculateScore(analysis *types.PartitionAnalysis, job *types.JobRequest) float64 {
	score := 0.0

	// Time factor (lower wait time = higher score)
	waitTimeHours := float64(analysis.EstimatedWaitTime) / float64(time.Hour)
	timeScore := 100.0 - (waitTimeHours * 10.0) // Subtract 10 points per hour of wait time
	if timeScore < 0 {
		timeScore = 0
	}

	// Cost factor (lower cost = higher score) - normalize to $0-100 range
	maxCost := 100.0 // Assume max reasonable cost of $100
	costScore := 100.0 - (analysis.EstimatedCost.TotalCost/maxCost)*100.0
	if costScore < 0 {
		costScore = 0
	}

	// Availability factor
	availScore := 50.0 // Default neutral score
	if analysis.Type == "aws" {
		availScore = 90.0 // AWS generally has high availability
	} else if analysis.AvailableNodes > 0 && analysis.TotalNodes > 0 {
		availRatio := float64(analysis.AvailableNodes) / float64(analysis.TotalNodes)
		availScore = availRatio * 100.0
	}

	// Weighted combination
	score = (timeScore * e.weights.TimeWeight) +
		(costScore * e.weights.CostWeight) +
		(availScore * e.weights.ReliabilityWeight)

	return score
}

// calculateConfidence determines confidence level in the recommendation
func (e *DecisionEngine) calculateConfidence(localScore, awsScore float64) float64 {
	scoreDiff := localScore - awsScore
	if scoreDiff < 0 {
		scoreDiff = -scoreDiff
	}

	// Convert score difference to confidence (0-1)
	confidence := scoreDiff / 100.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.1 {
		confidence = 0.1 // Minimum confidence
	}

	return confidence
}

// generateReasoning provides human-readable explanations for the recommendation
func (e *DecisionEngine) generateReasoning(local, aws *types.PartitionAnalysis, job *types.JobRequest, costDiff float64, timeSavings time.Duration) []string {
	reasoning := make([]string, 0)

	// Time analysis
	if timeSavings > 30*time.Minute {
		reasoning = append(reasoning, fmt.Sprintf("Significant time savings: %v by using AWS", timeSavings))
	} else if timeSavings < -30*time.Minute {
		reasoning = append(reasoning, fmt.Sprintf("Local cluster is faster by %v", -timeSavings))
	}

	// Cost analysis
	if costDiff > 5.0 {
		reasoning = append(reasoning, fmt.Sprintf("AWS costs $%.2f more (%.1f%% increase)", costDiff, (costDiff/local.EstimatedCost.TotalCost)*100))
	} else if costDiff < -5.0 {
		reasoning = append(reasoning, fmt.Sprintf("AWS costs $%.2f less (%.1f%% savings)", -costDiff, (-costDiff/local.EstimatedCost.TotalCost)*100))
	}

	// Queue analysis
	if local.QueueDepth > 5 {
		reasoning = append(reasoning, fmt.Sprintf("Heavy queue load on local cluster (%d jobs ahead)", local.QueueDepth))
	}

	// Resource type analysis
	if job.TRES != nil && job.TRES["gpu"] > 0 {
		if aws.InstanceType != "" {
			reasoning = append(reasoning, fmt.Sprintf("GPU job using %s instances on AWS", aws.InstanceType))
		}
	}

	if len(reasoning) == 0 {
		reasoning = append(reasoning, "Decision based on overall cost/time optimization")
	}

	return reasoning
}

// LocalCostCalculator calculates costs for local cluster execution
type LocalCostCalculator struct {
	config config.LocalCostsConfig
}

// NewLocalCostCalculator creates a new local cost calculator
func NewLocalCostCalculator(config config.LocalCostsConfig) *LocalCostCalculator {
	return &LocalCostCalculator{
		config: config,
	}
}

// Calculate computes the total cost for running a job on local resources
func (c *LocalCostCalculator) Calculate(partition string, job *types.JobRequest, info *types.PartitionInfo) *types.CostBreakdown {
	partitionCost := c.getPartitionCost(partition)

	// Calculate runtime in hours
	runtimeHours := float64(job.TimeLimit) / float64(time.Hour)

	// Calculate resource costs
	cpuCost := float64(job.Nodes*job.CPUsPerTask) * partitionCost.CostPerCPUHour * runtimeHours
	nodeCost := float64(job.Nodes) * partitionCost.CostPerNodeHour * runtimeHours

	// GPU costs if applicable
	gpuCost := 0.0
	if job.TRES != nil && job.TRES["gpu"] > 0 {
		totalGPUs := float64(job.Nodes * job.TRES["gpu"])
		gpuCost = totalGPUs * partitionCost.CostPerGPUHour * runtimeHours
	}

	// Apply overhead factors
	baseCost := cpuCost + nodeCost + gpuCost
	maintenanceCost := baseCost * (partitionCost.MaintenanceFactor - 1.0)
	powerCost := baseCost * (partitionCost.PowerCostFactor - 1.0)

	totalCost := baseCost + maintenanceCost + powerCost

	return &types.CostBreakdown{
		ComputeCost:  cpuCost + gpuCost,
		NodeCost:     nodeCost,
		OverheadCost: maintenanceCost + powerCost,
		TotalCost:    totalCost,
	}
}

// getPartitionCost returns cost configuration for a partition
func (c *LocalCostCalculator) getPartitionCost(partition string) types.LocalPartitionCost {
	if cost, exists := c.config.Partitions[partition]; exists {
		return cost
	}

	// Return default costs
	return types.LocalPartitionCost{
		CostPerCPUHour:    0.05,
		CostPerNodeHour:   0.10,
		CostPerGPUHour:    2.50,
		MaintenanceFactor: 1.3,
		PowerCostFactor:   1.2,
	}
}

// AWSCostCalculator calculates costs for AWS execution
type AWSCostCalculator struct{}

// NewAWSCostCalculator creates a new AWS cost calculator
func NewAWSCostCalculator() *AWSCostCalculator {
	return &AWSCostCalculator{}
}

// Calculate computes the total cost for running a job on AWS
func (c *AWSCostCalculator) Calculate(job *types.JobRequest, partitionConfig *config.AWSPartitionInfo, pricing *types.AWSInstancePricing) *types.CostBreakdown {
	// Calculate runtime in hours
	runtimeHours := float64(job.TimeLimit) / float64(time.Hour)

	// Use spot pricing if available, otherwise on-demand
	hourlyRate := pricing.OnDemandPrice
	if pricing.SpotPrice > 0 {
		hourlyRate = pricing.SpotPrice
	}

	// Calculate compute cost
	computeCost := float64(job.Nodes) * hourlyRate * runtimeHours

	// Estimate data transfer costs (simplified)
	dataTransferCost := float64(job.Nodes) * 0.09 * 2.0 // $0.09/GB, estimate 2GB per node

	// AWS overhead (EBS, networking, etc.)
	overheadCost := computeCost * 0.05 // 5% overhead

	totalCost := computeCost + dataTransferCost + overheadCost

	return &types.CostBreakdown{
		ComputeCost:      computeCost,
		DataTransferCost: dataTransferCost,
		OverheadCost:     overheadCost,
		TotalCost:        totalCost,
	}
}