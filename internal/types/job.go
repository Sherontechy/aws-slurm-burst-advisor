package types

import (
	"fmt"
	"time"
)

// JobRequest represents the resource requirements for a SLURM job submission.
type JobRequest struct {
	JobName       string            `json:"job_name" yaml:"job_name"`
	Nodes         int               `json:"nodes" yaml:"nodes"`
	CPUsPerTask   int               `json:"cpus_per_task" yaml:"cpus_per_task"`
	NTasksPerNode int               `json:"ntasks_per_node" yaml:"ntasks_per_node"`
	TimeLimit     time.Duration     `json:"time_limit" yaml:"time_limit"`
	Memory        string            `json:"memory" yaml:"memory"`
	TRES          map[string]int    `json:"tres" yaml:"tres"` // Trackable RESource (GPUs, etc.)
	Account       string            `json:"account" yaml:"account"`
	QOS           string            `json:"qos" yaml:"qos"`
	Features      []string          `json:"features" yaml:"features"`
	Constraints   []string          `json:"constraints" yaml:"constraints"`
}

// Validate checks if the job request has valid parameters.
func (j *JobRequest) Validate() error {
	if j.Nodes <= 0 {
		return fmt.Errorf("nodes must be positive, got %d", j.Nodes)
	}
	if j.CPUsPerTask <= 0 {
		return fmt.Errorf("cpus_per_task must be positive, got %d", j.CPUsPerTask)
	}
	if j.NTasksPerNode <= 0 {
		j.NTasksPerNode = 1 // Default to 1 task per node
	}
	if j.TimeLimit <= 0 {
		return fmt.Errorf("time_limit must be positive, got %v", j.TimeLimit)
	}
	return nil
}

// TotalCPUs returns the total number of CPUs requested.
func (j *JobRequest) TotalCPUs() int {
	return j.Nodes * j.NTasksPerNode * j.CPUsPerTask
}

// TotalTasks returns the total number of tasks.
func (j *JobRequest) TotalTasks() int {
	return j.Nodes * j.NTasksPerNode
}

// HasGPUs returns true if the job requests GPU resources.
func (j *JobRequest) HasGPUs() bool {
	gpus, exists := j.TRES["gpu"]
	return exists && gpus > 0
}

// TotalGPUs returns the total number of GPUs requested.
func (j *JobRequest) TotalGPUs() int {
	if !j.HasGPUs() {
		return 0
	}
	return j.Nodes * j.TRES["gpu"]
}

// BatchScript represents a parsed SLURM batch script.
type BatchScript struct {
	Filename      string            `json:"filename" yaml:"filename"`
	JobName       string            `json:"job_name" yaml:"job_name"`
	Partition     string            `json:"partition" yaml:"partition"`
	Nodes         int               `json:"nodes" yaml:"nodes"`
	NTasksPerNode int               `json:"ntasks_per_node" yaml:"ntasks_per_node"`
	CPUsPerTask   int               `json:"cpus_per_task" yaml:"cpus_per_task"`
	TimeLimit     time.Duration     `json:"time_limit" yaml:"time_limit"`
	Memory        string            `json:"memory" yaml:"memory"`
	GRES          map[string]int    `json:"gres" yaml:"gres"`
	Account       string            `json:"account" yaml:"account"`
	QOS           string            `json:"qos" yaml:"qos"`
	Features      []string          `json:"features" yaml:"features"`
	Constraints   []string          `json:"constraints" yaml:"constraints"`
	RawDirectives map[string]string `json:"raw_directives" yaml:"raw_directives"`
}

// ToJobRequest converts a BatchScript to a JobRequest.
func (b *BatchScript) ToJobRequest() *JobRequest {
	return &JobRequest{
		JobName:       b.JobName,
		Nodes:         b.Nodes,
		CPUsPerTask:   b.CPUsPerTask,
		NTasksPerNode: b.NTasksPerNode,
		TimeLimit:     b.TimeLimit,
		Memory:        b.Memory,
		TRES:          b.GRES,
		Account:       b.Account,
		QOS:           b.QOS,
		Features:      b.Features,
		Constraints:   b.Constraints,
	}
}

// IsArrayJob returns true if this is a SLURM job array.
func (b *BatchScript) IsArrayJob() bool {
	_, exists := b.RawDirectives["array"]
	return exists
}

// IsExclusive returns true if the job requires exclusive node access.
func (b *BatchScript) IsExclusive() bool {
	exclusive, exists := b.RawDirectives["exclusive"]
	return exists && exclusive == "true"
}

// HasDependencies returns true if the job has dependencies on other jobs.
func (b *BatchScript) HasDependencies() bool {
	_, exists := b.RawDirectives["dependency"]
	return exists
}