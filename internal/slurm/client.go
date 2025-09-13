package slurm

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/errors"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

const (
	defaultSLURMBinPath = "/usr/bin"
	defaultTimeout      = 30 * time.Second
)

// Client provides access to SLURM data and operations with robust error handling.
type Client struct {
	binPath string
	timeout time.Duration
}

// NewClient creates a new SLURM client with validation.
func NewClient(binPath string) *Client {
	if binPath == "" {
		binPath = defaultSLURMBinPath
	}
	return &Client{
		binPath: binPath,
		timeout: defaultTimeout,
	}
}

// SetTimeout configures the command timeout for SLURM operations.
func (c *Client) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		c.timeout = timeout
	}
}

// GetPartitionInfo retrieves information about a SLURM partition with validation.
func (c *Client) GetPartitionInfo(ctx context.Context, partition string) (*types.PartitionInfo, error) {
	if partition == "" {
		return nil, errors.NewValidationError("GetPartitionInfo", "partition name cannot be empty", nil)
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/sinfo",
		"-p", partition,
		"--Format=nodes,cpus,memory,features,state,time,nodelist",
		"--noheader",
	)

	output, err := cmd.Output()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSLURMError("GetPartitionInfo", "sinfo command timed out", err)
		}
		return nil, errors.NewSLURMError("GetPartitionInfo", "sinfo command failed", err)
	}

	info, err := c.parsePartitionInfo(partition, string(output))
	if err != nil {
		return nil, errors.NewSLURMError("GetPartitionInfo", "failed to parse partition info", err)
	}

	return info, nil
}

// GetQueueInfo retrieves information about the job queue for a partition.
func (c *Client) GetQueueInfo(ctx context.Context, partition string) (*types.QueueInfo, error) {
	if partition == "" {
		return nil, errors.NewValidationError("GetQueueInfo", "partition name cannot be empty", nil)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/squeue",
		"-p", partition,
		"--start",
		"--Format=jobid,name,user,state,nodes,cpus,timelimit,priority,submittime,starttime",
		"--noheader",
	)

	output, err := cmd.Output()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSLURMError("GetQueueInfo", "squeue command timed out", err)
		}
		return nil, errors.NewSLURMError("GetQueueInfo", "squeue command failed", err)
	}

	info, err := c.parseQueueInfo(partition, string(output))
	if err != nil {
		return nil, errors.NewSLURMError("GetQueueInfo", "failed to parse queue info", err)
	}

	return info, nil
}

// parsePartitionInfo parses sinfo output into PartitionInfo structure with validation.
func (c *Client) parsePartitionInfo(partitionName, output string) (*types.PartitionInfo, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return nil, fmt.Errorf("no partition information found for %s", partitionName)
	}

	info := &types.PartitionInfo{
		Name: partitionName,
		ResourcesPerNode: types.NodeResources{
			TRES: make(map[string]int),
		},
	}

	// Parse each line (could have multiple lines for different node states)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if err := c.parsePartitionLine(line, info); err != nil {
			// Log warning but continue parsing
			fmt.Printf("Warning: failed to parse partition line '%s': %v\n", line, err)
			continue
		}
	}

	// Validate the parsed information
	if info.TotalNodes == 0 {
		return nil, fmt.Errorf("no valid nodes found in partition %s", partitionName)
	}

	return info, nil
}

// parsePartitionLine parses a single line of sinfo output.
func (c *Client) parsePartitionLine(line string, info *types.PartitionInfo) error {
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return fmt.Errorf("insufficient fields in line: %s", line)
	}

	// Parse nodes count
	nodeCount, err := c.parseNodeCount(fields[0])
	if err != nil {
		return fmt.Errorf("invalid node specification: %w", err)
	}

	// Parse CPUs
	cpus, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("invalid CPU count: %s", fields[1])
	}

	// Parse state
	stateStr := fields[4]
	state := c.parseNodeState(stateStr)

	// Update totals
	info.TotalNodes += nodeCount
	if info.ResourcesPerNode.CPUs == 0 {
		info.ResourcesPerNode.CPUs = cpus
		info.ResourcesPerNode.Memory = fields[2]
		info.ResourcesPerNode.Features = strings.Split(fields[3], ",")
	}

	// Count nodes by state
	c.updateNodeStateCounts(info, state, nodeCount)

	// Set partition state (use the most restrictive state found)
	if info.State == "" || state == types.NodeStateDown {
		info.State = types.PartitionState(state)
	}

	return nil
}

// parseNodeState converts string state to NodeState constant.
func (c *Client) parseNodeState(stateStr string) types.NodeState {
	stateStr = strings.ToUpper(stateStr)
	switch {
	case strings.Contains(stateStr, "IDLE"):
		return types.NodeStateIdle
	case strings.Contains(stateStr, "ALLOC"):
		return types.NodeStateAllocated
	case strings.Contains(stateStr, "MIX"):
		return types.NodeStateMixed
	case strings.Contains(stateStr, "DOWN"):
		return types.NodeStateDown
	case strings.Contains(stateStr, "DRAIN"):
		return types.NodeStateDraining
	default:
		return types.NodeStateDown // Conservative default
	}
}

// updateNodeStateCounts updates node counts based on state.
func (c *Client) updateNodeStateCounts(info *types.PartitionInfo, state types.NodeState, count int) {
	switch state {
	case types.NodeStateIdle:
		info.IdleNodes += count
	case types.NodeStateAllocated:
		info.AllocatedNodes += count
	case types.NodeStateMixed:
		info.MixedNodes += count
	case types.NodeStateDown, types.NodeStateDraining:
		info.DownNodes += count
	}
}

// parseQueueInfo parses squeue output into QueueInfo structure with validation.
func (c *Client) parseQueueInfo(partitionName, output string) (*types.QueueInfo, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	info := &types.QueueInfo{
		PartitionName: partitionName,
		PendingJobs:   make([]types.JobSummary, 0),
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		job, err := c.parseJobLine(line)
		if err != nil {
			// Log warning but continue parsing
			fmt.Printf("Warning: failed to parse job line '%s': %v\n", line, err)
			continue
		}

		// Count job states
		switch job.State {
		case types.JobStateRunning:
			info.JobsRunning++
		case types.JobStatePending:
			info.JobsPending++
			info.PendingJobs = append(info.PendingJobs, *job)
		}
	}

	// Calculate estimated wait time
	info.EstimatedWaitTime = c.calculateEstimatedWaitTime(info.PendingJobs)
	info.JobsAhead = info.JobsPending

	return info, nil
}

// parseJobLine parses a single line of squeue output.
func (c *Client) parseJobLine(line string) (*types.JobSummary, error) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return nil, fmt.Errorf("insufficient fields in job line")
	}

	job := &types.JobSummary{
		JobID:   fields[0],
		JobName: fields[1],
		User:    fields[2],
		State:   c.parseJobState(fields[3]),
	}

	// Parse numeric fields with validation
	var err error
	if job.Nodes, err = strconv.Atoi(fields[4]); err != nil {
		return nil, fmt.Errorf("invalid nodes count: %s", fields[4])
	}
	if job.CPUs, err = strconv.Atoi(fields[5]); err != nil {
		return nil, fmt.Errorf("invalid CPU count: %s", fields[5])
	}
	if job.Priority, err = strconv.Atoi(fields[7]); err != nil {
		// Priority might be "N/A", so don't fail
		job.Priority = 0
	}

	// Parse time fields
	job.TimeLimit = c.parseSlurmTime(fields[6])
	job.SubmitTime = c.parseSlurmTimestamp(fields[8])
	if len(fields) > 9 && fields[9] != "N/A" {
		job.StartTime = c.parseSlurmTimestamp(fields[9])
		job.EstStartTime = job.StartTime
	}

	return job, nil
}

// parseJobState converts string state to JobState constant.
func (c *Client) parseJobState(stateStr string) types.JobState {
	stateStr = strings.ToUpper(stateStr)
	switch stateStr {
	case "RUNNING", "R":
		return types.JobStateRunning
	case "PENDING", "PD":
		return types.JobStatePending
	case "COMPLETED", "CD":
		return types.JobStateCompleted
	case "FAILED", "F":
		return types.JobStateFailed
	case "CANCELLED", "CA":
		return types.JobStateCancelled
	default:
		return types.JobStatePending // Conservative default
	}
}

// parseNodeCount extracts the number of nodes from SLURM node specification with validation.
func (c *Client) parseNodeCount(nodeSpec string) (int, error) {
	if nodeSpec == "" {
		return 0, fmt.Errorf("empty node specification")
	}

	// Handle simple number
	if !strings.Contains(nodeSpec, "[") {
		if count, err := strconv.Atoi(nodeSpec); err == nil {
			if count < 0 {
				return 0, fmt.Errorf("negative node count: %d", count)
			}
			return count, nil
		}
		return 1, nil // Default for unparseable simple specs
	}

	// Extract range from brackets
	rangeRegex := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	if matches := rangeRegex.FindStringSubmatch(nodeSpec); len(matches) == 3 {
		start, err1 := strconv.Atoi(matches[1])
		end, err2 := strconv.Atoi(matches[2])
		if err1 == nil && err2 == nil && end >= start {
			return end - start + 1, nil
		}
	}

	// Count comma-separated nodes in brackets
	listRegex := regexp.MustCompile(`\[([^\]]+)\]`)
	if matches := listRegex.FindStringSubmatch(nodeSpec); len(matches) == 2 {
		nodes := strings.Split(matches[1], ",")
		return len(nodes), nil
	}

	return 1, nil // Conservative fallback
}

// parseSlurmTime parses SLURM time format to time.Duration with validation.
func (c *Client) parseSlurmTime(timeStr string) time.Duration {
	if timeStr == "" || timeStr == "N/A" || timeStr == "UNLIMITED" {
		return 0
	}

	// Handle days format first
	if strings.Contains(timeStr, "-") {
		parts := strings.SplitN(timeStr, "-", 2)
		if len(parts) == 2 {
			days, err := strconv.Atoi(parts[0])
			if err == nil && days >= 0 {
				daysDuration := time.Duration(days) * 24 * time.Hour
				timePart := c.parseSlurmTime(parts[1])
				return daysDuration + timePart
			}
		}
	}

	// Handle HH:MM:SS format
	if strings.Count(timeStr, ":") == 2 {
		parts := strings.Split(timeStr, ":")
		if len(parts) == 3 {
			hours, err1 := strconv.Atoi(parts[0])
			minutes, err2 := strconv.Atoi(parts[1])
			seconds, err3 := strconv.Atoi(parts[2])
			if err1 == nil && err2 == nil && err3 == nil &&
				hours >= 0 && minutes >= 0 && seconds >= 0 &&
				minutes < 60 && seconds < 60 {
				return time.Duration(hours)*time.Hour +
					   time.Duration(minutes)*time.Minute +
					   time.Duration(seconds)*time.Second
			}
		}
	}

	// Handle MM:SS format
	if strings.Count(timeStr, ":") == 1 {
		parts := strings.Split(timeStr, ":")
		if len(parts) == 2 {
			minutes, err1 := strconv.Atoi(parts[0])
			seconds, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && minutes >= 0 && seconds >= 0 && seconds < 60 {
				return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
			}
		}
	}

	// Try to parse as minutes
	if minutes, err := strconv.Atoi(timeStr); err == nil && minutes >= 0 {
		return time.Duration(minutes) * time.Minute
	}

	return 0 // Return 0 for unparseable time strings
}

// parseSlurmTimestamp parses SLURM timestamp to time.Time with better error handling.
func (c *Client) parseSlurmTimestamp(timestamp string) time.Time {
	if timestamp == "" || timestamp == "N/A" || timestamp == "Unknown" {
		return time.Time{}
	}

	// Try common SLURM timestamp formats
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"01/02 15:04:05",
		"15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			// If no date information, assume current date
			if format == "15:04:05" {
				now := time.Now()
				return time.Date(now.Year(), now.Month(), now.Day(),
					t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			} else if format == "01/02 15:04:05" {
				now := time.Now()
				return time.Date(now.Year(), t.Month(), t.Day(),
					t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			}
			return t
		}
	}

	return time.Time{} // Return zero time for unparseable timestamps
}

// calculateEstimatedWaitTime estimates wait time based on queue state with improved logic.
func (c *Client) calculateEstimatedWaitTime(pendingJobs []types.JobSummary) time.Duration {
	if len(pendingJobs) == 0 {
		return 0
	}

	// Sort jobs by priority (higher priority = lower number in SLURM)
	// This is a simplified estimation - could be enhanced with more sophisticated algorithms
	var totalWaitTime time.Duration
	var jobsWithTimeLimit int

	for _, job := range pendingJobs {
		if job.TimeLimit > 0 {
			totalWaitTime += job.TimeLimit
			jobsWithTimeLimit++
		}
	}

	// Calculate average wait time, assuming some overlap in job execution
	if jobsWithTimeLimit > 0 {
		avgJobTime := totalWaitTime / time.Duration(jobsWithTimeLimit)
		// Assume 70% efficiency due to resource fragmentation
		return time.Duration(float64(avgJobTime) * 0.7 * float64(len(pendingJobs)))
	}

	// Fallback: assume 1 hour per job
	return time.Duration(len(pendingJobs)) * time.Hour
}

// TestConnection tests if SLURM commands are accessible with detailed error reporting.
func (c *Client) TestConnection(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/sinfo", "--version")
	if err := cmd.Run(); err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return errors.NewSLURMError("TestConnection", "SLURM command timed out", err)
		}
		return errors.NewSLURMError("TestConnection",
			fmt.Sprintf("SLURM commands not accessible at %s", c.binPath), err)
	}
	return nil
}

// GetClusterInfo retrieves general cluster information with structured parsing.
func (c *Client) GetClusterInfo(ctx context.Context) (map[string]interface{}, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/scontrol", "show", "config")
	output, err := cmd.Output()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSLURMError("GetClusterInfo", "scontrol command timed out", err)
		}
		return nil, errors.NewSLURMError("GetClusterInfo", "scontrol show config failed", err)
	}

	return c.parseClusterConfig(string(output)), nil
}

// parseClusterConfig parses scontrol show config output.
func (c *Client) parseClusterConfig(output string) map[string]interface{} {
	info := make(map[string]interface{})

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Try to parse numbers
				if intVal, err := strconv.Atoi(value); err == nil {
					info[key] = intVal
				} else if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
					info[key] = floatVal
				} else {
					info[key] = value
				}
			}
		}
	}

	return info
}

// GetJobHistory retrieves historical job completion data for analysis.
func (c *Client) GetJobHistory(ctx context.Context, partition string, days int) ([]types.JobSummary, error) {
	if partition == "" {
		return nil, errors.NewValidationError("GetJobHistory", "partition name cannot be empty", nil)
	}
	if days <= 0 || days > 365 {
		return nil, errors.NewValidationError("GetJobHistory", "days must be between 1 and 365", nil)
	}

	startTime := time.Now().AddDate(0, 0, -days)

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout*2) // Double timeout for historical queries
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/sacct",
		"--partition", partition,
		"--starttime", startTime.Format("2006-01-02"),
		"--format=JobID,JobName,User,State,ReqNodes,ReqCPUs,Timelimit,Submit,Start,End,Elapsed",
		"--noheader",
		"--parsable2",
	)

	output, err := cmd.Output()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSLURMError("GetJobHistory", "sacct command timed out", err)
		}
		return nil, errors.NewSLURMError("GetJobHistory", "sacct command failed", err)
	}

	return c.parseJobHistory(string(output)), nil
}

// parseJobHistory parses sacct output into job history with validation.
func (c *Client) parseJobHistory(output string) []types.JobSummary {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	jobs := make([]types.JobSummary, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		job, err := c.parseHistoryJobLine(line)
		if err != nil {
			// Log warning but continue parsing
			fmt.Printf("Warning: failed to parse history job line: %v\n", err)
			continue
		}

		jobs = append(jobs, *job)
	}

	return jobs
}

// parseHistoryJobLine parses a single line of sacct output.
func (c *Client) parseHistoryJobLine(line string) (*types.JobSummary, error) {
	fields := strings.Split(line, "|")
	if len(fields) < 11 {
		return nil, fmt.Errorf("insufficient fields in history line")
	}

	job := &types.JobSummary{
		JobID:   fields[0],
		JobName: fields[1],
		User:    fields[2],
		State:   c.parseJobState(fields[3]),
	}

	// Parse numeric fields
	var err error
	if job.Nodes, err = strconv.Atoi(fields[4]); err != nil {
		job.Nodes = 0 // Default for parsing errors
	}
	if job.CPUs, err = strconv.Atoi(fields[5]); err != nil {
		job.CPUs = 0 // Default for parsing errors
	}

	// Parse time fields
	job.TimeLimit = c.parseSlurmTime(fields[6])
	job.SubmitTime = c.parseSlurmTimestamp(fields[7])
	job.StartTime = c.parseSlurmTimestamp(fields[8])

	return job, nil
}