package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/domain"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// ExecutionPlanGenerator creates execution plans for aws-slurm-burst plugin integration.
type ExecutionPlanGenerator struct {
	analyzer       *HistoryAwareAnalyzer
	domainDetector *domain.DomainDetector
	mpiOptimizer   *domain.MPIOptimizer
	version        string
}

// NewExecutionPlanGenerator creates a new execution plan generator.
func NewExecutionPlanGenerator(analyzer *HistoryAwareAnalyzer, version string) *ExecutionPlanGenerator {
	return &ExecutionPlanGenerator{
		analyzer:       analyzer,
		domainDetector: domain.NewDomainDetector(),
		mpiOptimizer:   domain.NewMPIOptimizer(),
		version:        version,
	}
}

// GenerateExecutionPlan creates a complete execution plan for aws-slurm-burst.
func (e *ExecutionPlanGenerator) GenerateExecutionPlan(
	analysis *EnhancedAnalysis,
	job *types.JobRequest,
	scriptPath string,
) (*types.ExecutionPlan, error) {

	// Determine if we should burst to AWS
	shouldBurst := analysis.Current.Recommendation.IsAWSRecommended()

	plan := &types.ExecutionPlan{
		ShouldBurst: shouldBurst,
		ASBAVersion: e.version,
		GeneratedAt: time.Now(),
		JobMetadata: e.generateJobMetadata(job, scriptPath),
	}

	// If not bursting, return minimal plan for local execution
	if !shouldBurst {
		plan.RecommendationReasoning = analysis.Current.Recommendation.Reasoning
		return plan, nil
	}

	// Generate comprehensive AWS execution plan
	if err := e.populateAWSExecutionPlan(plan, analysis, job, scriptPath); err != nil {
		return nil, fmt.Errorf("failed to generate AWS execution plan: %w", err)
	}

	// Validate the generated plan
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("generated execution plan is invalid: %w", err)
	}

	return plan, nil
}

// generateJobMetadata creates job metadata from job request and script information.
func (e *ExecutionPlanGenerator) generateJobMetadata(job *types.JobRequest, scriptPath string) types.JobMetadata {
	metadata := types.JobMetadata{
		JobName:    job.JobName,
		ScriptPath: scriptPath,
		User:       job.Account, // Fallback to account if user not available
		Account:    job.Account,
	}

	// Generate script hash if script path is available
	if scriptPath != "" {
		if hash, err := e.calculateScriptHash(scriptPath); err == nil {
			metadata.ScriptHash = hash
		}
	}

	// Detect research domain
	if domain := e.domainDetector.DetectDomain(scriptPath, job); domain != nil {
		metadata.DetectedDomain = domain.Domain
	}

	// Classify workload type
	metadata.WorkloadType = e.classifyWorkloadType(job)

	return metadata
}

// populateAWSExecutionPlan fills in AWS-specific execution plan details.
func (e *ExecutionPlanGenerator) populateAWSExecutionPlan(
	plan *types.ExecutionPlan,
	analysis *EnhancedAnalysis,
	job *types.JobRequest,
	scriptPath string,
) error {

	// Generate instance specification
	instanceSpec, err := e.generateInstanceSpecification(analysis, job)
	if err != nil {
		return fmt.Errorf("failed to generate instance specification: %w", err)
	}
	plan.InstanceSpecification = *instanceSpec

	// Generate MPI configuration
	mpiConfig, err := e.generateMPIConfiguration(job, plan.JobMetadata.DetectedDomain)
	if err != nil {
		return fmt.Errorf("failed to generate MPI configuration: %w", err)
	}
	plan.MPIConfiguration = *mpiConfig

	// Generate network configuration
	networkConfig := e.generateNetworkConfiguration(plan.JobMetadata.DetectedDomain, mpiConfig)
	plan.NetworkConfiguration = *networkConfig

	// Generate cost constraints
	costConstraints := e.generateCostConstraints(analysis, job)
	plan.CostConstraints = *costConstraints

	// Generate performance targets
	performanceTarget := e.generatePerformanceTarget(analysis, job)
	plan.PerformanceTarget = *performanceTarget

	// Add reasoning and optimization info
	plan.RecommendationReasoning = analysis.Current.Recommendation.Reasoning
	plan.ConfidenceLevel = analysis.Current.Recommendation.Confidence

	if len(analysis.ResourceOptimizations) > 0 {
		plan.OptimizationApplied = make([]string, len(analysis.ResourceOptimizations))
		for i, opt := range analysis.ResourceOptimizations {
			plan.OptimizationApplied[i] = fmt.Sprintf("%s: %s → %s",
				opt.ResourceType, opt.CurrentValue, opt.SuggestedValue)
		}
	}

	return nil
}

// generateInstanceSpecification creates AWS instance specification.
func (e *ExecutionPlanGenerator) generateInstanceSpecification(
	analysis *EnhancedAnalysis,
	job *types.JobRequest,
) (*types.InstanceSpec, error) {

	// Use instance recommendations from analysis if available
	instanceTypes := []string{"m5.xlarge"} // Default fallback
	if len(analysis.InstanceRecommendations) > 0 {
		instanceTypes = []string{analysis.InstanceRecommendations[0].InstanceFamily + ".xlarge"}
	}

	// Determine purchasing option based on job characteristics
	purchasingOption := "spot"
	maxSpotPrice := 0.0
	if job.TimeLimit > 4*time.Hour {
		purchasingOption = "on-demand" // Long jobs prefer stability
	}

	spec := &types.InstanceSpec{
		InstanceTypes:    instanceTypes,
		InstanceCount:    job.Nodes,
		PurchasingOption: purchasingOption,
		MaxSpotPrice:     maxSpotPrice,
		PlacementGroup:   "cluster", // Default for MPI jobs
		SpotInstanceConfig: types.SpotConfig{
			EnableSpot:         purchasingOption == "spot",
			FallbackToOnDemand: true,
		},
	}

	return spec, nil
}

// generateMPIConfiguration creates MPI runtime configuration.
func (e *ExecutionPlanGenerator) generateMPIConfiguration(
	job *types.JobRequest,
	detectedDomain string,
) (*types.MPIConfig, error) {

	// Determine if this is an MPI job
	isMPIJob := job.Nodes > 1 || e.detectMPIFromJob(job)

	config := &types.MPIConfig{
		IsMPIJob:         isMPIJob,
		ProcessCount:     job.TotalTasks(),
		ProcessesPerNode: job.NTasksPerNode,
	}

	if isMPIJob {
		// Configure based on detected domain
		domainConfig := e.mpiOptimizer.GetDomainConfiguration(detectedDomain)
		if domainConfig != nil {
			config.CommunicationPattern = domainConfig.MPICommunicationPattern
			config.RequiresGangScheduling = domainConfig.RequiresGangScheduling
			config.RequiresEFA = domainConfig.RequiresEFA
			config.MPILibrary = domainConfig.PreferredMPILibrary
			config.MPITuningParams = domainConfig.MPITuningParams
		} else {
			// Default MPI configuration
			config.CommunicationPattern = "unknown"
			config.RequiresGangScheduling = true // Conservative default
			config.RequiresEFA = false
			config.MPILibrary = "OpenMPI"
		}
	}

	return config, nil
}

// generateNetworkConfiguration creates network optimization settings.
func (e *ExecutionPlanGenerator) generateNetworkConfiguration(
	detectedDomain string,
	mpiConfig *types.MPIConfig,
) *types.NetworkConfig {

	config := &types.NetworkConfig{
		EnhancedNetworking: true, // Always enable for AWS
	}

	// Configure based on MPI requirements
	if mpiConfig.IsMPIJob {
		if mpiConfig.RequiresEFA {
			config.EnableEFA = true
			config.NetworkLatencyClass = "ultra-low"
			config.PlacementGroupType = "cluster"
		} else {
			config.NetworkLatencyClass = "low"
			config.PlacementGroupType = "cluster"
		}

		// Domain-specific network optimization
		switch detectedDomain {
		case "machine_learning":
			config.BandwidthRequirement = "very_high"
			config.NetworkLatencyClass = "ultra-low"
		case "climate_modeling":
			config.BandwidthRequirement = "high"
			config.NetworkLatencyClass = "low"
		case "bioinformatics":
			config.BandwidthRequirement = "low"
			config.NetworkLatencyClass = "medium"
			config.PlacementGroupType = "spread" // Can spread across AZs
		default:
			config.BandwidthRequirement = "medium"
		}
	} else {
		// Single-node job
		config.NetworkLatencyClass = "medium"
		config.BandwidthRequirement = "low"
		config.PlacementGroupType = "spread"
	}

	return config
}

// generateCostConstraints creates cost limitations for the execution.
func (e *ExecutionPlanGenerator) generateCostConstraints(
	analysis *EnhancedAnalysis,
	job *types.JobRequest,
) *types.CostConstraints {

	// Use cost information from analysis
	maxCost := analysis.Current.BurstPartition.EstimatedCost.TotalCost * 1.2 // 20% buffer

	constraints := &types.CostConstraints{
		MaxTotalCost:     maxCost,
		MaxDurationHours: job.TimeLimit.Hours(),
		PreferSpot:       job.TimeLimit < 4*time.Hour, // Prefer spot for shorter jobs
		BudgetAccount:    job.Account,
		CostTolerance:    0.1, // 10% cost tolerance for performance
	}

	return constraints
}

// generatePerformanceTarget creates performance expectations.
func (e *ExecutionPlanGenerator) generatePerformanceTarget(
	analysis *EnhancedAnalysis,
	job *types.JobRequest,
) *types.PerformanceTarget {

	target := &types.PerformanceTarget{
		ExpectedRuntime:        job.TimeLimit,
		ScalingEfficiency:      0.8, // Conservative default
		CPUEfficiencyTarget:    75.0,
		MemoryEfficiencyTarget: 80.0,
		PerformanceModel:       "linear",
	}

	// Use historical data if available
	if analysis.HistoryInsights != nil && analysis.HistoryInsights.JobPattern != nil {
		pattern := analysis.HistoryInsights.JobPattern
		target.ExpectedRuntime = pattern.AvgRuntime
		target.CPUEfficiencyTarget = pattern.AvgCPUEfficiency
		target.MemoryEfficiencyTarget = pattern.AvgMemoryEfficiency

		// Adjust performance model based on workload type
		switch pattern.WorkloadType {
		case "cpu-bound":
			target.PerformanceModel = "strong_scaling"
			target.ScalingEfficiency = 0.85
		case "memory-bound":
			target.PerformanceModel = "weak_scaling"
			target.ScalingEfficiency = 0.7
		case "balanced":
			target.PerformanceModel = "linear"
			target.ScalingEfficiency = 0.8
		}
	}

	return target
}

// SaveExecutionPlan saves an execution plan to a JSON file.
func (e *ExecutionPlanGenerator) SaveExecutionPlan(plan *types.ExecutionPlan, outputPath string) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal execution plan: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write execution plan: %w", err)
	}

	return nil
}

// LoadExecutionPlan loads an execution plan from a JSON file.
func (e *ExecutionPlanGenerator) LoadExecutionPlan(inputPath string) (*types.ExecutionPlan, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read execution plan: %w", err)
	}

	var plan types.ExecutionPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal execution plan: %w", err)
	}

	// Validate loaded plan
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("loaded execution plan is invalid: %w", err)
	}

	return &plan, nil
}

// Helper functions
func (e *ExecutionPlanGenerator) calculateScriptHash(scriptPath string) (string, error) {
	// Placeholder - would implement actual script hashing
	return fmt.Sprintf("hash_%s", scriptPath), nil
}

func (e *ExecutionPlanGenerator) classifyWorkloadType(job *types.JobRequest) string {
	if job.HasGPUs() {
		return "gpu-bound"
	}
	if job.Nodes > 4 {
		return "distributed"
	}
	return "single-node"
}

func (e *ExecutionPlanGenerator) detectMPIFromJob(job *types.JobRequest) bool {
	// Simple heuristic - could be enhanced with script analysis
	return job.Nodes > 1 && job.NTasksPerNode > 1
}