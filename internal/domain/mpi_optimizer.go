package domain

import (
	"fmt"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// MPIOptimizer provides MPI optimization recommendations for different research domains.
type MPIOptimizer struct {
	domainConfigs map[string]DomainProfile
}

// NewMPIOptimizer creates a new MPI optimizer.
func NewMPIOptimizer() *MPIOptimizer {
	optimizer := &MPIOptimizer{
		domainConfigs: make(map[string]DomainProfile),
	}

	optimizer.initializeDomainConfigs()
	return optimizer
}

// GetDomainConfiguration returns MPI configuration for a research domain.
func (m *MPIOptimizer) GetDomainConfiguration(domain string) *DomainProfile {
	if config, exists := m.domainConfigs[domain]; exists {
		return &config
	}
	return nil
}

// OptimizeForDomain generates MPI configuration optimized for a specific research domain.
func (m *MPIOptimizer) OptimizeForDomain(domain string, job *types.JobRequest) (*types.MPIConfig, error) {
	profile := m.GetDomainConfiguration(domain)
	if profile == nil {
		return m.generateDefaultMPIConfig(job), nil
	}

	config := &types.MPIConfig{
		IsMPIJob:             job.Nodes > 1,
		ProcessCount:         job.TotalTasks(),
		ProcessesPerNode:     job.NTasksPerNode,
		CommunicationPattern: profile.MPICommunicationPattern,
		MPILibrary:           profile.PreferredMPILibrary,
		RequiresGangScheduling: profile.RequiresGangScheduling,
		RequiresEFA:          profile.RequiresEFA,
		MPITuningParams:      profile.MPITuningParams,
	}

	// Set EFA generation based on requirements
	if profile.RequiresEFA {
		config.EFAGeneration = 2 // Latest EFA generation
	}

	return config, nil
}

// generateDefaultMPIConfig creates a conservative MPI configuration for unknown domains.
func (m *MPIOptimizer) generateDefaultMPIConfig(job *types.JobRequest) *types.MPIConfig {
	return &types.MPIConfig{
		IsMPIJob:             job.Nodes > 1,
		ProcessCount:         job.TotalTasks(),
		ProcessesPerNode:     job.NTasksPerNode,
		CommunicationPattern: "unknown",
		MPILibrary:           "OpenMPI",
		RequiresGangScheduling: job.Nodes > 1, // Conservative - require gang scheduling for multi-node
		RequiresEFA:          false,           // Conservative - don't require EFA by default
		MPITuningParams: map[string]string{
			"btl": "vader,tcp",
		},
	}
}

// initializeDomainConfigs sets up MPI configurations for research domains.
func (m *MPIOptimizer) initializeDomainConfigs() {
	// Climate modeling MPI configuration
	m.domainConfigs["climate_modeling"] = DomainProfile{
		Name:                    "climate_modeling",
		MPICommunicationPattern: "nearest_neighbor",
		OptimalInstanceTypes:    []string{"c5n.18xlarge", "m5n.24xlarge"},
		ProcessPlacement:        "topology_aware",
		RequiresGangScheduling:  true,
		RequiresEFA:            true,
		PreferredMPILibrary:    "OpenMPI",
		MPITuningParams: map[string]string{
			"btl":                    "vader,tcp",
			"btl_tcp_if_include":     "eth0",
			"mpi_leave_pinned":       "1",
			"btl_tcp_eager_limit":    "32768",
			"btl_tcp_rndv_eager_limit": "32768",
		},
	}

	// Machine learning MPI configuration
	m.domainConfigs["machine_learning"] = DomainProfile{
		Name:                    "machine_learning",
		MPICommunicationPattern: "all_reduce",
		OptimalInstanceTypes:    []string{"p3dn.24xlarge", "p4d.24xlarge"},
		ProcessPlacement:        "gpu_topology_aware",
		RequiresGangScheduling:  true,
		RequiresEFA:            true,
		PreferredMPILibrary:    "NCCL",
		MPITuningParams: map[string]string{
			"NCCL_ALGO":              "Ring",
			"NCCL_MIN_NCHANNELS":     "4",
			"NCCL_MAX_NCHANNELS":     "16",
			"NCCL_TREE_THRESHOLD":    "0",
			"NCCL_IB_DISABLE":        "0",
		},
	}

	// Bioinformatics MPI configuration
	m.domainConfigs["bioinformatics"] = DomainProfile{
		Name:                    "bioinformatics",
		MPICommunicationPattern: "embarrassingly_parallel",
		OptimalInstanceTypes:    []string{"c5.24xlarge", "r5.12xlarge"},
		ProcessPlacement:        "cpu_dense",
		RequiresGangScheduling:  false,
		RequiresEFA:            false,
		PreferredMPILibrary:    "OpenMPI",
		MPITuningParams: map[string]string{
			"btl":                "vader,tcp",
			"mpi_warn_on_fork":   "0",
		},
	}

	// Computational physics MPI configuration
	m.domainConfigs["computational_physics"] = DomainProfile{
		Name:                    "computational_physics",
		MPICommunicationPattern: "tightly_coupled",
		OptimalInstanceTypes:    []string{"c5n.18xlarge", "c6in.16xlarge"},
		ProcessPlacement:        "numa_aware",
		RequiresGangScheduling:  true,
		RequiresEFA:            true,
		PreferredMPILibrary:    "Intel MPI",
		MPITuningParams: map[string]string{
			"I_MPI_FABRICS":          "shm:ofi",
			"I_MPI_OFI_PROVIDER":     "efa",
			"I_MPI_PIN_DOMAIN":       "omp",
		},
	}
}

// AnalyzeCommunicationPattern analyzes MPI communication requirements.
func (m *MPIOptimizer) AnalyzeCommunicationPattern(domain string, job *types.JobRequest) CommunicationAnalysis {
	profile := m.GetDomainConfiguration(domain)
	if profile == nil {
		return CommunicationAnalysis{
			Pattern:           "unknown",
			Intensity:         "medium",
			LatencyRequirement: "medium",
			BandwidthRequirement: "medium",
		}
	}

	analysis := CommunicationAnalysis{
		Pattern: profile.MPICommunicationPattern,
	}

	// Determine communication characteristics based on domain
	switch profile.MPICommunicationPattern {
	case "nearest_neighbor":
		analysis.Intensity = "high"
		analysis.LatencyRequirement = "medium"
		analysis.BandwidthRequirement = "high"
		analysis.MessageSizeProfile = "medium"
		analysis.SynchronizationFreq = "frequent"

	case "all_reduce":
		analysis.Intensity = "very_high"
		analysis.LatencyRequirement = "low"
		analysis.BandwidthRequirement = "very_high"
		analysis.MessageSizeProfile = "large"
		analysis.SynchronizationFreq = "very_frequent"

	case "embarrassingly_parallel":
		analysis.Intensity = "low"
		analysis.LatencyRequirement = "high"
		analysis.BandwidthRequirement = "low"
		analysis.MessageSizeProfile = "small"
		analysis.SynchronizationFreq = "rare"

	case "tightly_coupled":
		analysis.Intensity = "very_high"
		analysis.LatencyRequirement = "ultra_low"
		analysis.BandwidthRequirement = "high"
		analysis.MessageSizeProfile = "small"
		analysis.SynchronizationFreq = "continuous"
	}

	return analysis
}

// CommunicationAnalysis describes MPI communication characteristics.
type CommunicationAnalysis struct {
	Pattern               string `json:"pattern"`
	Intensity             string `json:"intensity"`
	LatencyRequirement    string `json:"latency_requirement"`
	BandwidthRequirement  string `json:"bandwidth_requirement"`
	MessageSizeProfile    string `json:"message_size_profile"`
	SynchronizationFreq   string `json:"synchronization_frequency"`
}

// RecommendNetworkConfiguration suggests network settings based on MPI requirements.
func (m *MPIOptimizer) RecommendNetworkConfiguration(domain string, job *types.JobRequest) types.NetworkConfig {
	analysis := m.AnalyzeCommunicationPattern(domain, job)

	config := types.NetworkConfig{
		EnhancedNetworking: true, // Always enable for AWS
	}

	// Configure based on communication analysis
	switch analysis.LatencyRequirement {
	case "ultra_low":
		config.NetworkLatencyClass = "ultra-low"
		config.EnableEFA = true
		config.PlacementGroupType = "cluster"
	case "low":
		config.NetworkLatencyClass = "low"
		config.EnableEFA = true
		config.PlacementGroupType = "cluster"
	case "medium":
		config.NetworkLatencyClass = "medium"
		config.EnableEFA = job.Nodes > 8 // EFA for larger jobs
		config.PlacementGroupType = "cluster"
	default:
		config.NetworkLatencyClass = "high"
		config.EnableEFA = false
		config.PlacementGroupType = "spread"
	}

	// Set bandwidth requirements
	config.BandwidthRequirement = analysis.BandwidthRequirement

	// Enable SR-IOV for high-performance networking
	config.EnableSRIOV = config.EnableEFA || analysis.BandwidthRequirement == "very_high"

	return config
}

// ValidateMPIConfiguration checks if MPI configuration is feasible.
func (m *MPIOptimizer) ValidateMPIConfiguration(config *types.MPIConfig, job *types.JobRequest) error {
	if !config.IsMPIJob && job.Nodes > 1 {
		return fmt.Errorf("multi-node job should be configured as MPI job")
	}

	if config.ProcessCount != job.TotalTasks() {
		return fmt.Errorf("MPI process count (%d) doesn't match job total tasks (%d)",
			config.ProcessCount, job.TotalTasks())
	}

	if config.ProcessesPerNode != job.NTasksPerNode {
		return fmt.Errorf("processes per node (%d) doesn't match job tasks per node (%d)",
			config.ProcessesPerNode, job.NTasksPerNode)
	}

	return nil
}