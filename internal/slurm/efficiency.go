package slurm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/errors"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// GetUserJobEfficiency retrieves detailed job efficiency data for a user.
func (c *Client) GetUserJobEfficiency(ctx context.Context, username string, days int) ([]types.JobEfficiencyData, error) {
	if username == "" {
		return nil, errors.NewValidationError("GetUserJobEfficiency", "username cannot be empty", nil)
	}
	if days <= 0 || days > 365 {
		return nil, errors.NewValidationError("GetUserJobEfficiency", "days must be between 1 and 365", nil)
	}

	startTime := time.Now().AddDate(0, 0, -days)

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout*3) // Longer timeout for detailed queries
	defer cancel()

	// Use comprehensive sacct format for efficiency analysis
	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/sacct",
		"--user", username,
		"--starttime", startTime.Format("2006-01-02"),
		"--format=JobID,JobName,Submit,Start,End,State,ExitCode,ReqCPUs,ReqMem,ReqTime,TotalCPU,CPUTime,MaxRSS,Elapsed,Partition",
		"--units=M", // Memory in MB
		"--noheader",
		"--parsable2",
	)

	output, err := cmd.Output()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSLURMError("GetUserJobEfficiency", "sacct command timed out", err)
		}
		return nil, errors.NewSLURMError("GetUserJobEfficiency", "sacct command failed", err)
	}

	return c.parseJobEfficiencyData(string(output))
}

// parseJobEfficiencyData parses sacct output into JobEfficiencyData structures.
func (c *Client) parseJobEfficiencyData(output string) ([]types.JobEfficiencyData, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	jobs := make([]types.JobEfficiencyData, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		job, err := c.parseEfficiencyLine(line)
		if err != nil {
			// Log warning but continue parsing
			fmt.Printf("Warning: failed to parse efficiency line: %v\n", err)
			continue
		}

		// Only include completed jobs for efficiency analysis
		if job.ExitCode == 0 && job.ActualTime > 0 {
			jobs = append(jobs, *job)
		}
	}

	return jobs, nil
}

// parseEfficiencyLine parses a single line of detailed sacct output.
func (c *Client) parseEfficiencyLine(line string) (*types.JobEfficiencyData, error) {
	fields := strings.Split(line, "|")
	if len(fields) < 15 {
		return nil, fmt.Errorf("insufficient fields in efficiency line: expected 15, got %d", len(fields))
	}

	job := &types.JobEfficiencyData{
		JobID:   fields[0],
		JobName: fields[1],
	}

	// Parse time fields
	var err error
	if job.SubmissionTime, err = c.parseSLURMTimestamp(fields[2]); err != nil {
		return nil, fmt.Errorf("invalid submission time: %w", err)
	}

	startTime, _ := c.parseSLURMTimestamp(fields[3])
	endTime, _ := c.parseSLURMTimestamp(fields[4])
	if !endTime.IsZero() && !startTime.IsZero() {
		job.QueueWaitTime = startTime.Sub(job.SubmissionTime)
	}

	// Parse exit code
	if job.ExitCode, err = strconv.Atoi(fields[6]); err != nil {
		job.ExitCode = -1 // Unknown exit code
	}

	// Parse resource requests
	if job.RequestedCPUs, err = strconv.Atoi(fields[7]); err != nil {
		return nil, fmt.Errorf("invalid requested CPUs: %s", fields[7])
	}

	if job.RequestedMemoryMB, err = types.ParseMemoryString(fields[8]); err != nil {
		return nil, fmt.Errorf("invalid requested memory: %w", err)
	}

	if job.RequestedTime, err = types.ParseSLURMTime(fields[9]); err != nil {
		return nil, fmt.Errorf("invalid requested time: %w", err)
	}

	// Parse actual usage
	if job.TotalCPUTime, err = types.ParseSLURMTime(fields[10]); err != nil {
		return nil, fmt.Errorf("invalid total CPU time: %w", err)
	}

	if job.CPUTimeAvailable, err = types.ParseSLURMTime(fields[11]); err != nil {
		return nil, fmt.Errorf("invalid CPU time available: %w", err)
	}

	if job.MaxMemoryUsedMB, err = types.ParseMemoryString(fields[12]); err != nil {
		return nil, fmt.Errorf("invalid max memory used: %w", err)
	}

	if job.ActualTime, err = types.ParseSLURMTime(fields[13]); err != nil {
		return nil, fmt.Errorf("invalid elapsed time: %w", err)
	}

	job.Partition = fields[14]

	// Determine execution platform
	job.ExecutionPlatform = "local"
	if strings.Contains(strings.ToLower(job.Partition), "aws") {
		job.ExecutionPlatform = "aws"
	}

	// Calculate all efficiency metrics
	job.CalculateEfficiencies()

	// Validate the parsed data
	if err := job.Validate(); err != nil {
		return nil, fmt.Errorf("invalid job efficiency data: %w", err)
	}

	return job, nil
}

// GetJobScriptHash calculates a hash of the job script content for similarity detection.
func (c *Client) GetJobScriptHash(scriptPath string) (string, error) {
	if scriptPath == "" {
		return "", fmt.Errorf("script path cannot be empty")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to read script file: %w", err)
	}

	// Normalize script content for comparison
	normalized := c.normalizeScriptContent(content)
	hash := sha256.Sum256(normalized)
	return fmt.Sprintf("%x", hash), nil
}

// normalizeScriptContent normalizes script content for consistent hashing.
func (c *Client) normalizeScriptContent(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var normalized []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments (except SBATCH directives)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#SBATCH") {
			continue
		}

		// Keep SBATCH directives and executable commands
		normalized = append(normalized, line)
	}

	return []byte(strings.Join(normalized, "\n"))
}

// GetCurrentUser returns the current system user for job history tracking.
func GetCurrentUser() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	return currentUser.Username, nil
}

// FindJobScriptPath attempts to find the script path for a given job ID.
func (c *Client) FindJobScriptPath(ctx context.Context, jobID string) (string, error) {
	if jobID == "" {
		return "", fmt.Errorf("job ID cannot be empty")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Try to get job details including script path
	cmd := exec.CommandContext(timeoutCtx, c.binPath+"/scontrol", "show", "job", jobID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get job details: %w", err)
	}

	// Parse script path from scontrol output
	return c.parseScriptPathFromScontrol(string(output)), nil
}

// parseScriptPathFromScontrol extracts script path from scontrol show job output.
func (c *Client) parseScriptPathFromScontrol(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Command=") {
			// Extract the command/script path
			parts := strings.SplitN(line, "Command=", 2)
			if len(parts) == 2 {
				command := strings.TrimSpace(parts[1])
				// If it's a script file, return the path
				if strings.HasSuffix(command, ".sbatch") || strings.HasSuffix(command, ".sh") {
					return command
				}
				// For complex commands, try to extract script name
				fields := strings.Fields(command)
				if len(fields) > 0 && (strings.HasSuffix(fields[0], ".sbatch") || strings.HasSuffix(fields[0], ".sh")) {
					return fields[0]
				}
			}
		}
	}
	return "" // No script path found
}

// parseSLURMTimestamp parses SLURM timestamp strings to time.Time.
func (c *Client) parseSLURMTimestamp(timestamp string) (time.Time, error) {
	if timestamp == "" || timestamp == "N/A" || timestamp == "Unknown" {
		return time.Time{}, nil
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
					t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
			} else if format == "01/02 15:04:05" {
				now := time.Now()
				return time.Date(now.Year(), t.Month(), t.Day(),
					t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
			}
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp format: %s", timestamp)
}