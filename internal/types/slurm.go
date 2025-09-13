package types

import (
	"time"
)

// PartitionState represents the state of a SLURM partition.
type PartitionState string

const (
	PartitionStateUp      PartitionState = "UP"
	PartitionStateDown    PartitionState = "DOWN"
	PartitionStateDrain   PartitionState = "DRAIN"
	PartitionStateInactive PartitionState = "INACTIVE"
)

// NodeState represents the state of a SLURM node.
type NodeState string

const (
	NodeStateIdle      NodeState = "IDLE"
	NodeStateAllocated NodeState = "ALLOCATED"
	NodeStateMixed     NodeState = "MIXED"
	NodeStateDown      NodeState = "DOWN"
	NodeStateDraining  NodeState = "DRAINING"
)

// JobState represents the state of a SLURM job.
type JobState string

const (
	JobStatePending   JobState = "PENDING"
	JobStateRunning   JobState = "RUNNING"
	JobStateCompleted JobState = "COMPLETED"
	JobStateFailed    JobState = "FAILED"
	JobStateCancelled JobState = "CANCELLED"
)

// PartitionInfo contains information about a SLURM partition.
type PartitionInfo struct {
	Name             string         `json:"name" yaml:"name"`
	State            PartitionState `json:"state" yaml:"state"`
	TotalNodes       int            `json:"total_nodes" yaml:"total_nodes"`
	IdleNodes        int            `json:"idle_nodes" yaml:"idle_nodes"`
	AllocatedNodes   int            `json:"allocated_nodes" yaml:"allocated_nodes"`
	MixedNodes       int            `json:"mixed_nodes" yaml:"mixed_nodes"`
	DownNodes        int            `json:"down_nodes" yaml:"down_nodes"`
	MaxTime          time.Duration  `json:"max_time" yaml:"max_time"`
	MaxNodes         int            `json:"max_nodes" yaml:"max_nodes"`
	MaxCPUs          int            `json:"max_cpus" yaml:"max_cpus"`
	ResourcesPerNode NodeResources  `json:"resources_per_node" yaml:"resources_per_node"`
}

// AvailableNodes returns the number of nodes available for scheduling.
func (p *PartitionInfo) AvailableNodes() int {
	return p.IdleNodes + p.MixedNodes
}

// UtilizationPercent returns the utilization percentage of the partition.
func (p *PartitionInfo) UtilizationPercent() float64 {
	if p.TotalNodes == 0 {
		return 0.0
	}
	usedNodes := p.AllocatedNodes + p.MixedNodes
	return float64(usedNodes) / float64(p.TotalNodes) * 100.0
}

// IsHealthy returns true if the partition is in a healthy state.
func (p *PartitionInfo) IsHealthy() bool {
	return p.State == PartitionStateUp && p.DownNodes < p.TotalNodes/2
}

// QueueInfo contains information about the current job queue state.
type QueueInfo struct {
	PartitionName     string        `json:"partition_name" yaml:"partition_name"`
	JobsAhead         int           `json:"jobs_ahead" yaml:"jobs_ahead"`
	EstimatedWaitTime time.Duration `json:"estimated_wait_time" yaml:"estimated_wait_time"`
	JobsRunning       int           `json:"jobs_running" yaml:"jobs_running"`
	JobsPending       int           `json:"jobs_pending" yaml:"jobs_pending"`
	PendingJobs       []JobSummary  `json:"pending_jobs" yaml:"pending_jobs"`
	AverageQueueTime  time.Duration `json:"average_queue_time" yaml:"average_queue_time"`
}

// IsEmpty returns true if there are no jobs in the queue.
func (q *QueueInfo) IsEmpty() bool {
	return q.JobsPending == 0 && q.JobsRunning == 0
}

// QueueDepth returns the total number of jobs in the queue.
func (q *QueueInfo) QueueDepth() int {
	return q.JobsPending + q.JobsRunning
}

// JobSummary represents a summary of job information from SLURM.
type JobSummary struct {
	JobID        string        `json:"job_id" yaml:"job_id"`
	JobName      string        `json:"job_name" yaml:"job_name"`
	User         string        `json:"user" yaml:"user"`
	State        JobState      `json:"state" yaml:"state"`
	Nodes        int           `json:"nodes" yaml:"nodes"`
	CPUs         int           `json:"cpus" yaml:"cpus"`
	TimeLimit    time.Duration `json:"time_limit" yaml:"time_limit"`
	Priority     int           `json:"priority" yaml:"priority"`
	SubmitTime   time.Time     `json:"submit_time" yaml:"submit_time"`
	StartTime    time.Time     `json:"start_time" yaml:"start_time"`
	EstStartTime time.Time     `json:"est_start_time" yaml:"est_start_time"`
}

// NodeResources represents resources available on a SLURM node.
type NodeResources struct {
	CPUs     int               `json:"cpus" yaml:"cpus"`
	Memory   string            `json:"memory" yaml:"memory"`
	TRES     map[string]int    `json:"tres" yaml:"tres"`
	Features []string          `json:"features" yaml:"features"`
}

// SlurmNode represents a SLURM compute node.
type SlurmNode struct {
	Name      string            `json:"name" yaml:"name"`
	State     NodeState         `json:"state" yaml:"state"`
	CPUs      int               `json:"cpus" yaml:"cpus"`
	CPUsAlloc int               `json:"cpus_alloc" yaml:"cpus_alloc"`
	Memory    string            `json:"memory" yaml:"memory"`
	GRES      map[string]int    `json:"gres" yaml:"gres"`
	Features  []string          `json:"features" yaml:"features"`
	Partition string            `json:"partition" yaml:"partition"`
}

// IsAvailable returns true if the node is available for job scheduling.
func (n *SlurmNode) IsAvailable() bool {
	return n.State == NodeStateIdle || n.State == NodeStateMixed
}

// CPUUtilization returns the CPU utilization percentage.
func (n *SlurmNode) CPUUtilization() float64 {
	if n.CPUs == 0 {
		return 0.0
	}
	return float64(n.CPUsAlloc) / float64(n.CPUs) * 100.0
}