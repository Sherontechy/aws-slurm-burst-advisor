package domain

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// DomainDetector analyzes jobs to classify research domains.
type DomainDetector struct {
	scriptPatterns   map[string]DomainPattern
	resourcePatterns map[string]ResourcePattern
	domainProfiles   map[string]DomainProfile
}

// DomainPattern represents patterns for detecting research domains from scripts.
type DomainPattern struct {
	Keywords       []string         `json:"keywords"`
	Executables    []string         `json:"executables"`
	ModulePatterns []regexp.Regexp  `json:"module_patterns"`
	FileExtensions []string         `json:"file_extensions"`
}

// ResourcePattern represents resource usage patterns that indicate research domains.
type ResourcePattern struct {
	CPUMemoryRatio     float64 `json:"cpu_memory_ratio"`
	TypicalNodeCount   int     `json:"typical_node_count"`
	GPURequired        bool    `json:"gpu_required"`
	NetworkIntensive   bool    `json:"network_intensive"`
	MemoryIntensive    bool    `json:"memory_intensive"`
}

// DomainProfile contains optimization profiles for research domains.
type DomainProfile struct {
	Name                    string                     `json:"name"`
	MPICommunicationPattern string                     `json:"mpi_communication_pattern"`
	NetworkRequirements     types.NetworkRequirements `json:"network_requirements"`
	OptimalInstanceTypes    []string                   `json:"optimal_instance_types"`
	MPILibraryPreferences   []string                   `json:"mpi_library_preferences"`
	ProcessPlacement        string                     `json:"process_placement"`
	RequiresGangScheduling  bool                       `json:"requires_gang_scheduling"`
	RequiresEFA            bool                       `json:"requires_efa"`
	PreferredMPILibrary    string                     `json:"preferred_mpi_library"`
	MPITuningParams        map[string]string          `json:"mpi_tuning_params"`
}

// NewDomainDetector creates a new domain detector with predefined patterns.
func NewDomainDetector() *DomainDetector {
	detector := &DomainDetector{
		scriptPatterns:   make(map[string]DomainPattern),
		resourcePatterns: make(map[string]ResourcePattern),
		domainProfiles:   make(map[string]DomainProfile),
	}

	detector.initializeDomainPatterns()
	detector.initializeDomainProfiles()

	return detector
}

// DetectDomain analyzes a job to determine its research domain.
func (d *DomainDetector) DetectDomain(scriptPath string, job *types.JobRequest) *types.DomainClassification {
	detectionScore := make(map[string]float64)

	// Analyze script content if available
	if scriptPath != "" {
		scriptScore := d.analyzeScriptContent(scriptPath)
		for domain, score := range scriptScore {
			detectionScore[domain] += score * 0.6 // 60% weight for script analysis
		}
	}

	// Analyze resource patterns
	resourceScore := d.analyzeResourcePattern(job)
	for domain, score := range resourceScore {
		detectionScore[domain] += score * 0.4 // 40% weight for resource patterns
	}

	// Find best match
	bestDomain := ""
	bestScore := 0.0
	for domain, score := range detectionScore {
		if score > bestScore {
			bestDomain = domain
			bestScore = score
		}
	}

	if bestScore < 0.3 { // Minimum confidence threshold
		return &types.DomainClassification{
			Domain:     "unknown",
			Confidence: 0.0,
			DetectionMethods: []string{"insufficient_data"},
		}
	}

	// Get domain characteristics
	characteristics := d.getDomainCharacteristics(bestDomain)

	return &types.DomainClassification{
		Domain:              bestDomain,
		Confidence:          bestScore,
		DetectionMethods:    []string{"script_analysis", "resource_pattern"},
		DomainCharacteristics: characteristics,
	}
}

// analyzeScriptContent analyzes script content for domain indicators.
func (d *DomainDetector) analyzeScriptContent(scriptPath string) map[string]float64 {
	scores := make(map[string]float64)

	file, err := os.Open(scriptPath)
	if err != nil {
		return scores
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	content := ""
	for scanner.Scan() {
		content += strings.ToLower(scanner.Text()) + "\n"
	}

	// Check each domain pattern
	for domain, pattern := range d.scriptPatterns {
		score := 0.0

		// Check for keywords
		for _, keyword := range pattern.Keywords {
			if strings.Contains(content, strings.ToLower(keyword)) {
				score += 0.2
			}
		}

		// Check for executables
		for _, executable := range pattern.Executables {
			if strings.Contains(content, executable) {
				score += 0.3
			}
		}

		// Check for file extensions in script
		scriptName := filepath.Base(scriptPath)
		for _, ext := range pattern.FileExtensions {
			if strings.HasSuffix(scriptName, ext) {
				score += 0.1
			}
		}

		scores[domain] = score
	}

	return scores
}

// analyzeResourcePattern analyzes resource requirements for domain indicators.
func (d *DomainDetector) analyzeResourcePattern(job *types.JobRequest) map[string]float64 {
	scores := make(map[string]float64)

	// Calculate job characteristics
	totalCPUs := job.TotalCPUs()
	hasGPUs := job.HasGPUs()
	nodeCount := job.Nodes

	// Score based on resource patterns
	for domain, pattern := range d.resourcePatterns {
		score := 0.0

		// GPU requirement matching
		if pattern.GPURequired == hasGPUs {
			score += 0.3
		}

		// Node count patterns
		if nodeCount >= pattern.TypicalNodeCount/2 && nodeCount <= pattern.TypicalNodeCount*2 {
			score += 0.2
		}

		// Memory intensity (simplified heuristic)
		if job.Memory != "" {
			memoryMB, _ := types.ParseMemoryString(job.Memory)
			memoryPerCPU := float64(memoryMB) / 1024.0 / float64(totalCPUs)

			if pattern.MemoryIntensive && memoryPerCPU > 4.0 {
				score += 0.3
			} else if !pattern.MemoryIntensive && memoryPerCPU <= 4.0 {
				score += 0.2
			}
		}

		scores[domain] = score
	}

	return scores
}

// getDomainCharacteristics returns characteristics for a detected domain.
func (d *DomainDetector) getDomainCharacteristics(domain string) types.DomainCharacteristics {
	profile, exists := d.domainProfiles[domain]
	if !exists {
		return types.DomainCharacteristics{
			TypicalCommunicationPattern: "unknown",
			NetworkSensitivity:         "medium",
			ScalingBehavior:           "linear",
			MemoryIntensity:           "medium",
			ComputeIntensity:          "medium",
		}
	}

	return types.DomainCharacteristics{
		TypicalCommunicationPattern: profile.MPICommunicationPattern,
		NetworkSensitivity:         d.getNetworkSensitivity(profile),
		ScalingBehavior:           d.getScalingBehavior(profile),
		MemoryIntensity:           d.getMemoryIntensity(profile),
		ComputeIntensity:          d.getComputeIntensity(profile),
		OptimalInstanceFamilies:   d.getInstanceFamilies(profile.OptimalInstanceTypes),
	}
}

// initializeDomainPatterns sets up detection patterns for research domains.
func (d *DomainDetector) initializeDomainPatterns() {
	// Climate modeling patterns
	d.scriptPatterns["climate_modeling"] = DomainPattern{
		Keywords:       []string{"wrf", "gromacs", "namd", "climate", "weather", "atmospheric"},
		Executables:    []string{"wrf.exe", "gmx", "namd2", "real.exe"},
		FileExtensions: []string{".mdp", ".pdb", ".psf"},
	}

	// Machine learning patterns
	d.scriptPatterns["machine_learning"] = DomainPattern{
		Keywords:       []string{"pytorch", "tensorflow", "training", "model", "neural", "deep learning"},
		Executables:    []string{"python", "torchrun", "horovodrun"},
		FileExtensions: []string{".py", ".ipynb"},
	}

	// Bioinformatics patterns
	d.scriptPatterns["bioinformatics"] = DomainPattern{
		Keywords:       []string{"blast", "bwa", "samtools", "gatk", "genome", "sequence"},
		Executables:    []string{"blastp", "bwa", "samtools", "gatk"},
		FileExtensions: []string{".fasta", ".fastq", ".sam", ".bam"},
	}

	// Computational physics patterns
	d.scriptPatterns["computational_physics"] = DomainPattern{
		Keywords:       []string{"lammps", "quantum", "vasp", "gaussian", "dft"},
		Executables:    []string{"lmp", "vasp", "g16", "qe.x"},
		FileExtensions: []string{".in", ".inp", ".com"},
	}
}

// initializeDomainProfiles sets up optimization profiles for research domains.
func (d *DomainDetector) initializeDomainProfiles() {
	// Climate modeling profile
	d.domainProfiles["climate_modeling"] = DomainProfile{
		Name:                    "climate_modeling",
		MPICommunicationPattern: "nearest_neighbor",
		OptimalInstanceTypes:    []string{"c5n.18xlarge", "m5n.24xlarge"},
		MPILibraryPreferences:   []string{"OpenMPI", "Intel MPI"},
		ProcessPlacement:        "topology_aware",
		RequiresGangScheduling:  true,
		RequiresEFA:            true,
		PreferredMPILibrary:    "OpenMPI",
		MPITuningParams: map[string]string{
			"btl":              "vader,tcp",
			"btl_tcp_if_include": "eth0",
			"mpi_leave_pinned":   "1",
		},
	}

	// Machine learning profile
	d.domainProfiles["machine_learning"] = DomainProfile{
		Name:                    "machine_learning",
		MPICommunicationPattern: "all_reduce",
		OptimalInstanceTypes:    []string{"p3dn.24xlarge", "p4d.24xlarge"},
		MPILibraryPreferences:   []string{"NCCL", "OpenMPI"},
		ProcessPlacement:        "gpu_topology_aware",
		RequiresGangScheduling:  true,
		RequiresEFA:            true,
		PreferredMPILibrary:    "NCCL",
		MPITuningParams: map[string]string{
			"NCCL_ALGO":              "Ring",
			"NCCL_MIN_NCHANNELS":     "4",
			"NCCL_MAX_NCHANNELS":     "16",
		},
	}

	// Bioinformatics profile
	d.domainProfiles["bioinformatics"] = DomainProfile{
		Name:                    "bioinformatics",
		MPICommunicationPattern: "embarrassingly_parallel",
		OptimalInstanceTypes:    []string{"c5.24xlarge", "r5.12xlarge"},
		MPILibraryPreferences:   []string{"OpenMPI", "MPICH"},
		ProcessPlacement:        "cpu_dense",
		RequiresGangScheduling:  false,
		RequiresEFA:            false,
		PreferredMPILibrary:    "OpenMPI",
		MPITuningParams: map[string]string{
			"btl": "vader,tcp",
		},
	}

	// Add resource patterns
	d.resourcePatterns["climate_modeling"] = ResourcePattern{
		CPUMemoryRatio:   4.0, // 4GB per CPU typical
		TypicalNodeCount: 16,
		GPURequired:      false,
		NetworkIntensive: true,
		MemoryIntensive:  true,
	}

	d.resourcePatterns["machine_learning"] = ResourcePattern{
		CPUMemoryRatio:   8.0, // 8GB per CPU for large models
		TypicalNodeCount: 8,
		GPURequired:      true,
		NetworkIntensive: true,
		MemoryIntensive:  true,
	}

	d.resourcePatterns["bioinformatics"] = ResourcePattern{
		CPUMemoryRatio:   6.0, // 6GB per CPU for sequence data
		TypicalNodeCount: 4,
		GPURequired:      false,
		NetworkIntensive: false,
		MemoryIntensive:  true,
	}
}

// Helper functions for domain characteristics
func (d *DomainDetector) getNetworkSensitivity(profile DomainProfile) string {
	if profile.RequiresEFA {
		return "high"
	}
	if profile.MPICommunicationPattern == "all_reduce" {
		return "high"
	}
	if profile.MPICommunicationPattern == "nearest_neighbor" {
		return "medium"
	}
	return "low"
}

func (d *DomainDetector) getScalingBehavior(profile DomainProfile) string {
	switch profile.MPICommunicationPattern {
	case "nearest_neighbor":
		return "strong_scaling"
	case "all_reduce":
		return "weak_scaling"
	case "embarrassingly_parallel":
		return "linear"
	default:
		return "unknown"
	}
}

func (d *DomainDetector) getMemoryIntensity(profile DomainProfile) string {
	for _, instanceType := range profile.OptimalInstanceTypes {
		if strings.HasPrefix(instanceType, "r") { // r5, r6i families
			return "high"
		}
	}
	return "medium"
}

func (d *DomainDetector) getComputeIntensity(profile DomainProfile) string {
	for _, instanceType := range profile.OptimalInstanceTypes {
		if strings.HasPrefix(instanceType, "c") { // c5, c6i families
			return "high"
		}
	}
	return "medium"
}

func (d *DomainDetector) getInstanceFamilies(instanceTypes []string) []string {
	families := make(map[string]bool)
	for _, instanceType := range instanceTypes {
		parts := strings.Split(instanceType, ".")
		if len(parts) > 0 {
			families[parts[0]] = true
		}
	}

	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	return result
}

// GetDomainProfile returns the optimization profile for a domain.
func (d *DomainDetector) GetDomainProfile(domain string) (*DomainProfile, error) {
	profile, exists := d.domainProfiles[domain]
	if !exists {
		return nil, fmt.Errorf("no profile found for domain: %s", domain)
	}
	return &profile, nil
}

// ListSupportedDomains returns all supported research domains.
func (d *DomainDetector) ListSupportedDomains() []string {
	domains := make([]string, 0, len(d.domainProfiles))
	for domain := range d.domainProfiles {
		domains = append(domains, domain)
	}
	return domains
}