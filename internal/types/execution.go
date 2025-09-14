package types

import (
	"fmt"
	"time"
)

// ExecutionPlan represents a complete execution plan for aws-slurm-burst plugin.
// This format matches the schema expected by aws-slurm-burst for seamless integration.
type ExecutionPlan struct {
	// Metadata
	JobMetadata          JobMetadata          `json:"job_metadata"`
	ASBAVersion          string               `json:"asba_version"`
	GeneratedAt          time.Time            `json:"generated_at"`

	// Core execution configuration
	ShouldBurst          bool                 `json:"should_burst"`
	InstanceSpecification InstanceSpec        `json:"instance_specification"`
	MPIConfiguration     MPIConfig           `json:"mpi_configuration"`
	NetworkConfiguration NetworkConfig       `json:"network_configuration"`
	CostConstraints      CostConstraints     `json:"cost_constraints"`
	PerformanceTarget    PerformanceTarget   `json:"performance_target"`

	// ASBA analysis context
	RecommendationReasoning []string          `json:"recommendation_reasoning"`
	ConfidenceLevel      float64             `json:"confidence_level"`
	OptimizationApplied  []string            `json:"optimization_applied"`
}

// JobMetadata contains information about the job being planned.
type JobMetadata struct {
	JobName        string            `json:"job_name"`
	ScriptPath     string            `json:"script_path"`
	ScriptHash     string            `json:"script_hash"`
	User           string            `json:"user"`
	Account        string            `json:"account"`
	DetectedDomain string            `json:"detected_domain"`
	WorkloadType   string            `json:"workload_type"`
}

// InstanceSpec defines AWS instance requirements.
type InstanceSpec struct {
	InstanceTypes      []string    `json:"instance_types"`
	InstanceCount      int         `json:"instance_count"`
	PurchasingOption   string      `json:"purchasing_option"` // "spot", "on-demand", "mixed"
	MaxSpotPrice       float64     `json:"max_spot_price"`
	SubnetIDs          []string    `json:"subnet_ids"`
	LaunchTemplateName string      `json:"launch_template_name"`
	SpotInstanceConfig SpotConfig  `json:"spot_instance_config"`
	PlacementGroup     string      `json:"placement_group"`
	AvailabilityZones  []string    `json:"availability_zones"`
}

// SpotConfig defines spot instance configuration.
type SpotConfig struct {
	EnableSpot           bool    `json:"enable_spot"`
	SpotFleetRequest     bool    `json:"spot_fleet_request"`
	MaxSpotPrice         float64 `json:"max_spot_price"`
	SpotInterruptionTolerance float64 `json:"spot_interruption_tolerance"`
	FallbackToOnDemand   bool    `json:"fallback_to_on_demand"`
}

// MPIConfig defines MPI runtime configuration.
type MPIConfig struct {
	IsMPIJob             bool              `json:"is_mpi_job"`
	ProcessCount         int               `json:"process_count"`
	ProcessesPerNode     int               `json:"processes_per_node"`
	CommunicationPattern string            `json:"communication_pattern"`
	MPILibrary           string            `json:"mpi_library"`
	MPITuningParams      map[string]string `json:"mpi_tuning_params"`
	RequiresGangScheduling bool            `json:"requires_gang_scheduling"`
	RequiresEFA          bool              `json:"requires_efa"`
	EFAGeneration        int               `json:"efa_generation"`
}

// NetworkConfig defines network optimization requirements.
type NetworkConfig struct {
	PlacementGroupType   string `json:"placement_group_type"` // "cluster", "partition", "spread"
	EnhancedNetworking   bool   `json:"enhanced_networking"`
	NetworkLatencyClass  string `json:"network_latency_class"` // "ultra-low", "low", "medium", "high"
	BandwidthRequirement string `json:"bandwidth_requirement"` // "low", "medium", "high", "very_high"
	EnableEFA            bool   `json:"enable_efa"`
	EnableSRIOV          bool   `json:"enable_sr_iov"`
}

// CostConstraints defines budget and cost limitations.
type CostConstraints struct {
	MaxTotalCost     float64 `json:"max_total_cost"`
	MaxDurationHours float64 `json:"max_duration_hours"`
	PreferSpot       bool    `json:"prefer_spot"`
	BudgetAccount    string  `json:"budget_account"`
	CostTolerance    float64 `json:"cost_tolerance"` // Acceptable cost premium for performance
}

// PerformanceTarget defines expected performance characteristics.
type PerformanceTarget struct {
	ExpectedRuntime      time.Duration `json:"expected_runtime"`
	ScalingEfficiency    float64       `json:"scaling_efficiency"`
	NetworkUtilization   float64       `json:"network_utilization"`
	CPUEfficiencyTarget  float64       `json:"cpu_efficiency_target"`
	MemoryEfficiencyTarget float64     `json:"memory_efficiency_target"`
	PerformanceModel     string        `json:"performance_model"` // "linear", "strong_scaling", "weak_scaling"
}

// DomainClassification represents detected research domain.
type DomainClassification struct {
	Domain              string            `json:"domain"`
	Confidence          float64           `json:"confidence"`
	DetectionMethods    []string          `json:"detection_methods"`
	DomainCharacteristics DomainCharacteristics `json:"domain_characteristics"`
}

// DomainCharacteristics describes research domain properties.
type DomainCharacteristics struct {
	TypicalCommunicationPattern string   `json:"typical_communication_pattern"`
	NetworkSensitivity         string   `json:"network_sensitivity"`
	ScalingBehavior           string   `json:"scaling_behavior"`
	MemoryIntensity           string   `json:"memory_intensity"`
	ComputeIntensity          string   `json:"compute_intensity"`
	OptimalInstanceFamilies   []string `json:"optimal_instance_families"`
}

// NetworkRequirements defines network performance requirements.
type NetworkRequirements struct {
	LatencyTolerance   string `json:"latency_tolerance"`    // "low", "medium", "high"
	BandwidthNeeds     string `json:"bandwidth_needs"`      // "low", "medium", "high", "very_high"
	TopologyImportance string `json:"topology_importance"`  // "critical", "important", "minimal"
}

// Validate ensures the execution plan is complete and valid.
func (e *ExecutionPlan) Validate() error {
	if !e.ShouldBurst {
		return nil // Local execution doesn't need validation
	}

	if len(e.InstanceSpecification.InstanceTypes) == 0 {
		return fmt.Errorf("at least one instance type must be specified")
	}

	if e.InstanceSpecification.InstanceCount <= 0 {
		return fmt.Errorf("instance count must be positive: %d", e.InstanceSpecification.InstanceCount)
	}

	if e.MPIConfiguration.IsMPIJob {
		if e.MPIConfiguration.ProcessCount <= 0 {
			return fmt.Errorf("MPI process count must be positive: %d", e.MPIConfiguration.ProcessCount)
		}
		if e.MPIConfiguration.ProcessesPerNode <= 0 {
			return fmt.Errorf("processes per node must be positive: %d", e.MPIConfiguration.ProcessesPerNode)
		}
	}

	if e.CostConstraints.MaxTotalCost < 0 {
		return fmt.Errorf("max total cost cannot be negative: %f", e.CostConstraints.MaxTotalCost)
	}

	if e.PerformanceTarget.ScalingEfficiency < 0 || e.PerformanceTarget.ScalingEfficiency > 1 {
		return fmt.Errorf("scaling efficiency must be between 0 and 1: %f", e.PerformanceTarget.ScalingEfficiency)
	}

	return nil
}

// GetEstimatedCost calculates estimated cost based on instance specification and runtime.
func (e *ExecutionPlan) GetEstimatedCost() float64 {
	if !e.ShouldBurst {
		return 0.0
	}

	// Simplified cost estimation - would use actual AWS pricing in real implementation
	baseHourlyCost := 1.0 // Placeholder - would lookup actual instance pricing
	hours := e.PerformanceTarget.ExpectedRuntime.Hours()
	instanceCount := float64(e.InstanceSpecification.InstanceCount)

	estimatedCost := baseHourlyCost * hours * instanceCount

	// Apply spot discount if using spot instances
	if e.InstanceSpecification.PurchasingOption == "spot" {
		estimatedCost *= 0.7 // Typical 30% spot discount
	}

	return estimatedCost
}

// IsOptimizedForMPI returns true if the plan includes MPI optimizations.
func (e *ExecutionPlan) IsOptimizedForMPI() bool {
	return e.MPIConfiguration.IsMPIJob &&
		   (e.NetworkConfiguration.EnableEFA || e.NetworkConfiguration.EnhancedNetworking)
}

// GetRecommendedCommand returns the aws-slurm-burst command to execute this plan.
func (e *ExecutionPlan) GetRecommendedCommand(nodeList string) string {
	if !e.ShouldBurst {
		return fmt.Sprintf("sbatch %s", e.JobMetadata.ScriptPath)
	}

	// Generate aws-slurm-burst command
	cmd := fmt.Sprintf("aws-slurm-burst resume --execution-plan=%s %s",
		"execution-plan.json", nodeList)

	return cmd
}

// ToJSON serializes the execution plan to JSON format.
func (e *ExecutionPlan) ToJSON() ([]byte, error) {
	// Would use json.Marshal in real implementation
	return nil, fmt.Errorf("JSON serialization not implemented yet")
}

// FromJSON deserializes an execution plan from JSON.
func (e *ExecutionPlan) FromJSON(data []byte) error {
	// Would use json.Unmarshal in real implementation
	return fmt.Errorf("JSON deserialization not implemented yet")
}