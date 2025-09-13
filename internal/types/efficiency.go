package types

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// JobEfficiencyData represents detailed job execution efficiency metrics from SLURM sacct.
type JobEfficiencyData struct {
	JobID           string        `json:"job_id" db:"job_id"`
	JobName         string        `json:"job_name" db:"job_name"`
	User            string        `json:"user" db:"user"`
	ScriptPath      string        `json:"script_path" db:"script_path"`
	ScriptHash      string        `json:"script_hash" db:"script_hash"`
	SubmissionTime  time.Time     `json:"submission_time" db:"submission_time"`

	// Resource requests
	RequestedCPUs     int           `json:"requested_cpus" db:"req_cpus"`
	RequestedMemoryMB int64         `json:"requested_memory_mb" db:"req_memory_mb"`
	RequestedGPUs     int           `json:"requested_gpus" db:"req_gpus"`
	RequestedTime     time.Duration `json:"requested_time" db:"req_time_seconds"`

	// Actual usage from sacct
	ActualTime        time.Duration `json:"actual_time" db:"actual_time_seconds"`
	MaxMemoryUsedMB   int64         `json:"max_memory_used_mb" db:"max_memory_mb"`
	TotalCPUTime      time.Duration `json:"total_cpu_time" db:"total_cpu_seconds"`
	CPUTimeAvailable  time.Duration `json:"cpu_time_available" db:"cpu_time_available"`

	// Calculated efficiencies
	CPUEfficiency     float64       `json:"cpu_efficiency" db:"cpu_efficiency"`
	MemoryEfficiency  float64       `json:"memory_efficiency" db:"memory_efficiency"`
	TimeEfficiency    float64       `json:"time_efficiency" db:"time_efficiency"`

	// CPU-Memory relationship analysis
	RequestedCPUMemRatio float64    `json:"requested_cpu_mem_ratio" db:"req_cpu_mem_ratio"`
	ActualCPUMemRatio    float64    `json:"actual_cpu_mem_ratio" db:"actual_cpu_mem_ratio"`
	EffectiveCPUs        float64    `json:"effective_cpus" db:"effective_cpus"`

	// Execution context
	Partition         string        `json:"partition" db:"partition"`
	ExitCode          int           `json:"exit_code" db:"exit_code"`
	QueueWaitTime     time.Duration `json:"queue_wait_time" db:"queue_wait_seconds"`
	ExecutionPlatform string        `json:"execution_platform" db:"execution_platform"`

	// Workload classification
	WorkloadType      string        `json:"workload_type" db:"workload_type"`
	BottleneckType    string        `json:"bottleneck_type" db:"bottleneck_type"`
}

// CalculateEfficiencies computes efficiency metrics from raw SLURM data.
func (j *JobEfficiencyData) CalculateEfficiencies() {
	// CPU efficiency: TotalCPU / CPUTime * 100
	if j.CPUTimeAvailable > 0 {
		j.CPUEfficiency = float64(j.TotalCPUTime) / float64(j.CPUTimeAvailable) * 100.0
	}

	// Memory efficiency: MaxRSS / ReqMem * 100
	if j.RequestedMemoryMB > 0 {
		j.MemoryEfficiency = float64(j.MaxMemoryUsedMB) / float64(j.RequestedMemoryMB) * 100.0
	}

	// Time efficiency: ActualTime / RequestedTime * 100
	if j.RequestedTime > 0 {
		j.TimeEfficiency = float64(j.ActualTime) / float64(j.RequestedTime) * 100.0
	}

	// CPU-Memory ratios
	if j.RequestedCPUs > 0 {
		j.RequestedCPUMemRatio = float64(j.RequestedMemoryMB) / 1024.0 / float64(j.RequestedCPUs) // GB per CPU
	}

	// Effective CPUs used
	j.EffectiveCPUs = float64(j.RequestedCPUs) * j.CPUEfficiency / 100.0

	if j.EffectiveCPUs > 0 {
		j.ActualCPUMemRatio = float64(j.MaxMemoryUsedMB) / 1024.0 / j.EffectiveCPUs // GB per effective CPU
	}

	// Classify workload type
	j.WorkloadType = j.classifyWorkload()
	j.BottleneckType = j.identifyBottleneck()
}

// classifyWorkload determines if the job is CPU-bound, memory-bound, or balanced.
func (j *JobEfficiencyData) classifyWorkload() string {
	switch {
	case j.CPUEfficiency > 80 && j.MemoryEfficiency < 60:
		return "cpu-bound"
	case j.MemoryEfficiency > 80 && j.CPUEfficiency < 60:
		return "memory-bound"
	case j.CPUEfficiency > 70 && j.MemoryEfficiency > 70:
		return "balanced"
	case j.CPUEfficiency < 40 && j.MemoryEfficiency < 40:
		return "over-allocated"
	default:
		return "mixed"
	}
}

// identifyBottleneck determines the primary resource bottleneck.
func (j *JobEfficiencyData) identifyBottleneck() string {
	// Simple heuristic based on efficiency patterns
	if j.MemoryEfficiency > 90 {
		return "memory"
	}
	if j.CPUEfficiency > 90 {
		return "cpu"
	}
	if j.TimeEfficiency > 95 {
		return "time-limit"
	}
	if j.CPUEfficiency < 30 {
		return "cpu-over-allocation"
	}
	if j.MemoryEfficiency < 30 {
		return "memory-over-allocation"
	}
	return "balanced"
}

// IsSuccessful returns true if the job completed successfully.
func (j *JobEfficiencyData) IsSuccessful() bool {
	return j.ExitCode == 0
}

// GetOptimalCPUMemRatio returns the optimal CPU-to-memory ratio for AWS instance selection.
func (j *JobEfficiencyData) GetOptimalCPUMemRatio() float64 {
	if j.ActualCPUMemRatio > 0 {
		// Add 20% buffer to actual usage
		return j.ActualCPUMemRatio * 1.2
	}
	// Fallback to requested ratio if no actual data
	return j.RequestedCPUMemRatio
}

// SuggestAWSInstanceFamily recommends AWS instance family based on CPU-memory pattern.
func (j *JobEfficiencyData) SuggestAWSInstanceFamily() AWSInstanceFamilyRecommendation {
	optimalRatio := j.GetOptimalCPUMemRatio()

	switch {
	case optimalRatio < 3.0: // CPU-intensive, low memory per core
		return AWSInstanceFamilyRecommendation{
			Family:       "c5",
			Ratio:        2.0, // 2GB per vCPU
			Reasoning:    fmt.Sprintf("CPU-bound workload (%.1f%% CPU eff, %.1fGB per effective core)", j.CPUEfficiency, optimalRatio),
			CostBenefit:  "Optimized for CPU performance, lower memory cost",
		}
	case optimalRatio > 6.0: // Memory-intensive, high memory per core
		return AWSInstanceFamilyRecommendation{
			Family:       "r5",
			Ratio:        8.0, // 8GB per vCPU
			Reasoning:    fmt.Sprintf("Memory-bound workload (%.1f%% memory eff, %.1fGB per effective core)", j.MemoryEfficiency, optimalRatio),
			CostBenefit:  "High memory-to-CPU ratio, better price per GB",
		}
	default: // Balanced workload
		return AWSInstanceFamilyRecommendation{
			Family:       "m5",
			Ratio:        4.0, // 4GB per vCPU
			Reasoning:    fmt.Sprintf("Balanced workload (%.1fGB per effective core)", optimalRatio),
			CostBenefit:  "General-purpose instances for mixed workloads",
		}
	}
}

// AWSInstanceFamilyRecommendation contains instance family recommendation details.
type AWSInstanceFamilyRecommendation struct {
	Family      string  `json:"family"`
	Ratio       float64 `json:"cpu_memory_ratio"`
	Reasoning   string  `json:"reasoning"`
	CostBenefit string  `json:"cost_benefit"`
}

// JobPattern represents aggregated patterns for a specific script/job type.
type JobPattern struct {
	ScriptName    string    `json:"script_name"`
	ScriptHash    string    `json:"script_hash"`
	RunCount      int       `json:"run_count"`
	LastRun       time.Time `json:"last_run"`

	// CPU patterns
	AvgCPUEfficiency   float64 `json:"avg_cpu_efficiency"`
	CPUVariability     float64 `json:"cpu_variability"`
	TypicalEffectiveCPUs float64 `json:"typical_effective_cpus"`

	// Memory patterns
	AvgMemoryEfficiency float64 `json:"avg_memory_efficiency"`
	MemoryVariability   float64 `json:"memory_variability"`
	TypicalMemoryUsageGB float64 `json:"typical_memory_usage_gb"`

	// CPU-Memory relationship
	AvgRequestedRatio float64 `json:"avg_requested_ratio"`
	AvgActualRatio    float64 `json:"avg_actual_ratio"`
	WorkloadType      string  `json:"workload_type"`

	// Performance patterns
	AvgRuntime        time.Duration `json:"avg_runtime"`
	RuntimeVariability float64      `json:"runtime_variability"`
	SuccessRate       float64       `json:"success_rate"`

	// Platform performance
	LocalExecutions   int     `json:"local_executions"`
	AWSExecutions     int     `json:"aws_executions"`
	PreferredPlatform string  `json:"preferred_platform"` // Based on user choices
}

// Validate ensures the efficiency data is reasonable.
func (j *JobEfficiencyData) Validate() error {
	if j.JobID == "" {
		return fmt.Errorf("job_id cannot be empty")
	}
	if j.RequestedCPUs <= 0 {
		return fmt.Errorf("requested_cpus must be positive: %d", j.RequestedCPUs)
	}
	if j.RequestedMemoryMB <= 0 {
		return fmt.Errorf("requested_memory_mb must be positive: %d", j.RequestedMemoryMB)
	}
	if j.CPUEfficiency < 0 || j.CPUEfficiency > 200 { // Allow >100% for brief bursts
		return fmt.Errorf("cpu_efficiency out of range: %.2f", j.CPUEfficiency)
	}
	if j.MemoryEfficiency < 0 || j.MemoryEfficiency > 100 {
		return fmt.Errorf("memory_efficiency out of range: %.2f", j.MemoryEfficiency)
	}
	return nil
}

// ParseMemoryString converts SLURM memory strings (e.g., "64G", "1024M") to MB.
func ParseMemoryString(memStr string) (int64, error) {
	if memStr == "" || memStr == "0" {
		return 0, nil
	}

	// Remove any trailing characters and convert to uppercase
	memStr = strings.ToUpper(strings.TrimSpace(memStr))

	// Handle different units
	if strings.HasSuffix(memStr, "G") {
		gb, err := strconv.ParseFloat(strings.TrimSuffix(memStr, "G"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid memory format: %s", memStr)
		}
		return int64(gb * 1024), nil
	}

	if strings.HasSuffix(memStr, "M") {
		mb, err := strconv.ParseFloat(strings.TrimSuffix(memStr, "M"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid memory format: %s", memStr)
		}
		return int64(mb), nil
	}

	if strings.HasSuffix(memStr, "K") {
		kb, err := strconv.ParseFloat(strings.TrimSuffix(memStr, "K"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid memory format: %s", memStr)
		}
		return int64(kb / 1024), nil
	}

	// Assume MB if no unit specified
	mb, err := strconv.ParseFloat(memStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory format: %s", memStr)
	}
	return int64(mb), nil
}

// FormatMemoryMB converts MB to human-readable format.
func FormatMemoryMB(mb int64) string {
	if mb < 1024 {
		return fmt.Sprintf("%dM", mb)
	}
	gb := float64(mb) / 1024.0
	if gb < 1024 {
		return fmt.Sprintf("%.1fG", gb)
	}
	tb := gb / 1024.0
	return fmt.Sprintf("%.2fT", tb)
}

// ParseSLURMTime converts SLURM time format to time.Duration.
func ParseSLURMTime(timeStr string) (time.Duration, error) {
	if timeStr == "" || timeStr == "UNLIMITED" {
		return 0, nil
	}

	// Handle formats: "HH:MM:SS", "MM:SS", "SS", "DD-HH:MM:SS"
	if strings.Contains(timeStr, "-") {
		// Days format: "2-12:30:00"
		parts := strings.SplitN(timeStr, "-", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("invalid time format: %s", timeStr)
		}

		days, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid days in time format: %s", timeStr)
		}

		timePart, err := ParseSLURMTime(parts[1])
		if err != nil {
			return 0, err
		}

		return time.Duration(days)*24*time.Hour + timePart, nil
	}

	// Handle HH:MM:SS format
	parts := strings.Split(timeStr, ":")
	switch len(parts) {
	case 1:
		// Just seconds
		seconds, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid seconds: %s", parts[0])
		}
		return time.Duration(seconds) * time.Second, nil

	case 2:
		// MM:SS
		minutes, err1 := strconv.Atoi(parts[0])
		seconds, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("invalid MM:SS format: %s", timeStr)
		}
		return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second, nil

	case 3:
		// HH:MM:SS
		hours, err1 := strconv.Atoi(parts[0])
		minutes, err2 := strconv.Atoi(parts[1])
		seconds, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, fmt.Errorf("invalid HH:MM:SS format: %s", timeStr)
		}
		return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second, nil

	default:
		return 0, fmt.Errorf("unsupported time format: %s", timeStr)
	}
}