package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/errors"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// JobHistoryDB manages job execution history in a local SQLite database.
type JobHistoryDB struct {
	db       *sql.DB
	user     string
	dbPath   string
	readOnly bool
}

// NewJobHistoryDB creates a new job history database for the specified user.
func NewJobHistoryDB(username string) (*JobHistoryDB, error) {
	if username == "" {
		return nil, errors.NewValidationError("NewJobHistoryDB", "username cannot be empty", nil)
	}

	// Create .asba directory in user's home
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.NewConfigError("NewJobHistoryDB", "failed to get home directory", err)
	}

	asbaDir := filepath.Join(homeDir, ".asba")
	if err := os.MkdirAll(asbaDir, 0755); err != nil {
		return nil, errors.NewConfigError("NewJobHistoryDB", "failed to create .asba directory", err)
	}

	dbPath := filepath.Join(asbaDir, "jobs.db")

	// Open SQLite database
	db, err := sql.Open("sqlite3", dbPath+"?_timeout=30000&_journal_mode=WAL")
	if err != nil {
		return nil, errors.NewConfigError("NewJobHistoryDB", "failed to open database", err)
	}

	historyDB := &JobHistoryDB{
		db:     db,
		user:   username,
		dbPath: dbPath,
	}

	// Initialize schema
	if err := historyDB.initializeSchema(); err != nil {
		db.Close()
		return nil, errors.NewConfigError("NewJobHistoryDB", "failed to initialize database schema", err)
	}

	return historyDB, nil
}

// initializeSchema creates the database tables if they don't exist.
func (h *JobHistoryDB) initializeSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS job_history (
		job_id TEXT PRIMARY KEY,
		job_name TEXT,
		user TEXT,
		script_path TEXT,
		script_hash TEXT,
		submission_time INTEGER,

		-- Resource requests
		req_cpus INTEGER,
		req_memory_mb INTEGER,
		req_gpus INTEGER,
		req_time_seconds INTEGER,
		req_cpu_mem_ratio REAL,

		-- Actual usage
		actual_time_seconds INTEGER,
		max_memory_mb INTEGER,
		total_cpu_seconds INTEGER,
		cpu_time_available INTEGER,

		-- Calculated efficiencies
		cpu_efficiency REAL,
		memory_efficiency REAL,
		time_efficiency REAL,
		effective_cpus REAL,
		actual_cpu_mem_ratio REAL,

		-- Execution context
		partition TEXT,
		exit_code INTEGER,
		queue_wait_seconds INTEGER,
		execution_platform TEXT,

		-- Classification
		workload_type TEXT,
		bottleneck_type TEXT,

		-- Metadata
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_script_hash ON job_history(script_hash);
	CREATE INDEX IF NOT EXISTS idx_submission_time ON job_history(submission_time);
	CREATE INDEX IF NOT EXISTS idx_workload_type ON job_history(workload_type);
	CREATE INDEX IF NOT EXISTS idx_script_path ON job_history(script_path);
	CREATE INDEX IF NOT EXISTS idx_user ON job_history(user);

	-- Job patterns summary table
	CREATE TABLE IF NOT EXISTS job_patterns (
		script_hash TEXT PRIMARY KEY,
		script_name TEXT,
		run_count INTEGER,
		last_run INTEGER,

		-- CPU patterns
		avg_cpu_efficiency REAL,
		cpu_variability REAL,
		typical_effective_cpus REAL,

		-- Memory patterns
		avg_memory_efficiency REAL,
		memory_variability REAL,
		typical_memory_usage_gb REAL,

		-- CPU-Memory relationship
		avg_requested_ratio REAL,
		avg_actual_ratio REAL,
		workload_type TEXT,

		-- Performance patterns
		avg_runtime_seconds INTEGER,
		runtime_variability REAL,
		success_rate REAL,

		-- Platform preferences
		local_executions INTEGER DEFAULT 0,
		aws_executions INTEGER DEFAULT 0,
		preferred_platform TEXT,

		-- Metadata
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_pattern_workload ON job_patterns(workload_type);
	CREATE INDEX IF NOT EXISTS idx_pattern_script ON job_patterns(script_name);
	`

	if _, err := h.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create database schema: %w", err)
	}

	return nil
}

// StoreJobExecution stores a job execution record in the database.
func (h *JobHistoryDB) StoreJobExecution(job types.JobEfficiencyData) error {
	if h.readOnly {
		return nil // Skip storage in read-only mode
	}

	query := `
	INSERT OR REPLACE INTO job_history (
		job_id, job_name, user, script_path, script_hash, submission_time,
		req_cpus, req_memory_mb, req_gpus, req_time_seconds, req_cpu_mem_ratio,
		actual_time_seconds, max_memory_mb, total_cpu_seconds, cpu_time_available,
		cpu_efficiency, memory_efficiency, time_efficiency, effective_cpus, actual_cpu_mem_ratio,
		partition, exit_code, queue_wait_seconds, execution_platform,
		workload_type, bottleneck_type
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := h.db.Exec(query,
		job.JobID, job.JobName, job.User, job.ScriptPath, job.ScriptHash, job.SubmissionTime.Unix(),
		job.RequestedCPUs, job.RequestedMemoryMB, job.RequestedGPUs, int64(job.RequestedTime.Seconds()), job.RequestedCPUMemRatio,
		int64(job.ActualTime.Seconds()), job.MaxMemoryUsedMB, int64(job.TotalCPUTime.Seconds()), int64(job.CPUTimeAvailable.Seconds()),
		job.CPUEfficiency, job.MemoryEfficiency, job.TimeEfficiency, job.EffectiveCPUs, job.ActualCPUMemRatio,
		job.Partition, job.ExitCode, int64(job.QueueWaitTime.Seconds()), job.ExecutionPlatform,
		job.WorkloadType, job.BottleneckType,
	)

	if err != nil {
		return errors.NewConfigError("StoreJobExecution", "failed to store job execution", err)
	}

	// Update job patterns
	if err := h.updateJobPattern(job); err != nil {
		// Log warning but don't fail - patterns are supplementary
		fmt.Printf("Warning: failed to update job pattern: %v\n", err)
	}

	return nil
}

// FindSimilarJobs finds jobs similar to the given script hash and resource pattern.
func (h *JobHistoryDB) FindSimilarJobs(scriptHash string, resourcePattern types.JobRequest) ([]types.JobEfficiencyData, error) {
	var jobs []types.JobEfficiencyData

	// First, look for exact script matches
	exactMatches, err := h.getJobsByScriptHash(scriptHash)
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, exactMatches...)

	// If we have few exact matches, look for similar resource patterns
	if len(exactMatches) < 3 {
		similarJobs, err := h.getJobsBySimilarResources(resourcePattern)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, similarJobs...)
	}

	return jobs, nil
}

// getJobsByScriptHash retrieves jobs with the same script hash.
func (h *JobHistoryDB) getJobsByScriptHash(scriptHash string) ([]types.JobEfficiencyData, error) {
	query := `
	SELECT job_id, job_name, user, script_path, script_hash, submission_time,
		   req_cpus, req_memory_mb, req_gpus, req_time_seconds, req_cpu_mem_ratio,
		   actual_time_seconds, max_memory_mb, total_cpu_seconds, cpu_time_available,
		   cpu_efficiency, memory_efficiency, time_efficiency, effective_cpus, actual_cpu_mem_ratio,
		   partition, exit_code, queue_wait_seconds, execution_platform,
		   workload_type, bottleneck_type
	FROM job_history
	WHERE script_hash = ? AND exit_code = 0
	ORDER BY submission_time DESC
	LIMIT 20
	`

	rows, err := h.db.Query(query, scriptHash)
	if err != nil {
		return nil, errors.NewConfigError("getJobsByScriptHash", "failed to query job history", err)
	}
	defer rows.Close()

	return h.scanJobRows(rows)
}

// getJobsBySimilarResources finds jobs with similar resource patterns.
func (h *JobHistoryDB) getJobsBySimilarResources(pattern types.JobRequest) ([]types.JobEfficiencyData, error) {
	// Look for jobs with similar CPU/memory ratios (±50%)
	cpuMemRatio := float64(0) // Will be calculated from pattern
	if pattern.Nodes > 0 && pattern.CPUsPerTask > 0 {
		// Estimate memory from pattern if available
		totalCPUs := pattern.Nodes * pattern.CPUsPerTask
		if len(pattern.Memory) > 0 {
			if memMB, err := types.ParseMemoryString(pattern.Memory); err == nil {
				cpuMemRatio = float64(memMB) / 1024.0 / float64(totalCPUs)
			}
		}
	}

	query := `
	SELECT job_id, job_name, user, script_path, script_hash, submission_time,
		   req_cpus, req_memory_mb, req_gpus, req_time_seconds, req_cpu_mem_ratio,
		   actual_time_seconds, max_memory_mb, total_cpu_seconds, cpu_time_available,
		   cpu_efficiency, memory_efficiency, time_efficiency, effective_cpus, actual_cpu_mem_ratio,
		   partition, exit_code, queue_wait_seconds, execution_platform,
		   workload_type, bottleneck_type
	FROM job_history
	WHERE exit_code = 0
	  AND req_cpus BETWEEN ? AND ?
	  AND (req_cpu_mem_ratio BETWEEN ? AND ? OR req_cpu_mem_ratio IS NULL)
	ORDER BY submission_time DESC
	LIMIT 10
	`

	totalCPUs := pattern.Nodes * pattern.CPUsPerTask
	cpuMin, cpuMax := int(float64(totalCPUs)*0.5), int(float64(totalCPUs)*2.0)
	ratioMin, ratioMax := cpuMemRatio*0.5, cpuMemRatio*2.0

	rows, err := h.db.Query(query, cpuMin, cpuMax, ratioMin, ratioMax)
	if err != nil {
		return nil, errors.NewConfigError("getJobsBySimilarResources", "failed to query similar jobs", err)
	}
	defer rows.Close()

	return h.scanJobRows(rows)
}

// scanJobRows scans database rows into JobEfficiencyData structs.
func (h *JobHistoryDB) scanJobRows(rows *sql.Rows) ([]types.JobEfficiencyData, error) {
	var jobs []types.JobEfficiencyData

	for rows.Next() {
		var job types.JobEfficiencyData
		var submissionTime, actualTime, totalCPU, cpuAvailable, queueWait int64

		err := rows.Scan(
			&job.JobID, &job.JobName, &job.User, &job.ScriptPath, &job.ScriptHash, &submissionTime,
			&job.RequestedCPUs, &job.RequestedMemoryMB, &job.RequestedGPUs, &job.RequestedTime, &job.RequestedCPUMemRatio,
			&actualTime, &job.MaxMemoryUsedMB, &totalCPU, &cpuAvailable,
			&job.CPUEfficiency, &job.MemoryEfficiency, &job.TimeEfficiency, &job.EffectiveCPUs, &job.ActualCPUMemRatio,
			&job.Partition, &job.ExitCode, &queueWait, &job.ExecutionPlatform,
			&job.WorkloadType, &job.BottleneckType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job row: %w", err)
		}

		// Convert timestamps
		job.SubmissionTime = time.Unix(submissionTime, 0)
		job.ActualTime = time.Duration(actualTime) * time.Second
		job.TotalCPUTime = time.Duration(totalCPU) * time.Second
		job.CPUTimeAvailable = time.Duration(cpuAvailable) * time.Second
		job.QueueWaitTime = time.Duration(queueWait) * time.Second

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating job rows: %w", err)
	}

	return jobs, nil
}

// updateJobPattern updates the aggregated job pattern statistics.
func (h *JobHistoryDB) updateJobPattern(job types.JobEfficiencyData) error {
	if job.ScriptHash == "" {
		return nil // Can't update pattern without script hash
	}

	// Get current pattern or create new one
	pattern, err := h.getJobPattern(job.ScriptHash)
	if err != nil {
		// Create new pattern
		pattern = &types.JobPattern{
			ScriptHash: job.ScriptHash,
			ScriptName: filepath.Base(job.ScriptPath),
		}
	}

	// Update pattern with new job data
	pattern.RunCount++
	pattern.LastRun = job.SubmissionTime

	// Update CPU efficiency (running average)
	if pattern.RunCount == 1 {
		pattern.AvgCPUEfficiency = job.CPUEfficiency
		pattern.TypicalEffectiveCPUs = job.EffectiveCPUs
	} else {
		// Calculate running average
		weight := 1.0 / float64(pattern.RunCount)
		pattern.AvgCPUEfficiency = pattern.AvgCPUEfficiency*(1-weight) + job.CPUEfficiency*weight
		pattern.TypicalEffectiveCPUs = pattern.TypicalEffectiveCPUs*(1-weight) + job.EffectiveCPUs*weight
	}

	// Update memory efficiency
	if pattern.RunCount == 1 {
		pattern.AvgMemoryEfficiency = job.MemoryEfficiency
		pattern.TypicalMemoryUsageGB = float64(job.MaxMemoryUsedMB) / 1024.0
	} else {
		weight := 1.0 / float64(pattern.RunCount)
		pattern.AvgMemoryEfficiency = pattern.AvgMemoryEfficiency*(1-weight) + job.MemoryEfficiency*weight
		pattern.TypicalMemoryUsageGB = pattern.TypicalMemoryUsageGB*(1-weight) + float64(job.MaxMemoryUsedMB)/1024.0*weight
	}

	// Update CPU-memory ratios
	if pattern.RunCount == 1 {
		pattern.AvgRequestedRatio = job.RequestedCPUMemRatio
		pattern.AvgActualRatio = job.ActualCPUMemRatio
	} else {
		weight := 1.0 / float64(pattern.RunCount)
		pattern.AvgRequestedRatio = pattern.AvgRequestedRatio*(1-weight) + job.RequestedCPUMemRatio*weight
		pattern.AvgActualRatio = pattern.AvgActualRatio*(1-weight) + job.ActualCPUMemRatio*weight
	}

	// Update runtime patterns
	if pattern.RunCount == 1 {
		pattern.AvgRuntime = job.ActualTime
	} else {
		weight := 1.0 / float64(pattern.RunCount)
		avgSeconds := float64(pattern.AvgRuntime.Seconds())*(1-weight) + float64(job.ActualTime.Seconds())*weight
		pattern.AvgRuntime = time.Duration(avgSeconds) * time.Second
	}

	// Update success rate
	if job.IsSuccessful() {
		pattern.SuccessRate = (pattern.SuccessRate*float64(pattern.RunCount-1) + 1.0) / float64(pattern.RunCount)
	} else {
		pattern.SuccessRate = pattern.SuccessRate * float64(pattern.RunCount-1) / float64(pattern.RunCount)
	}

	// Update platform counts
	if job.ExecutionPlatform == "aws" {
		pattern.AWSExecutions++
	} else {
		pattern.LocalExecutions++
	}

	// Determine preferred platform
	if pattern.AWSExecutions > pattern.LocalExecutions {
		pattern.PreferredPlatform = "aws"
	} else {
		pattern.PreferredPlatform = "local"
	}

	// Update workload type based on latest classification
	pattern.WorkloadType = job.WorkloadType

	// Store updated pattern
	return h.storeJobPattern(pattern)
}

// getJobPattern retrieves an existing job pattern.
func (h *JobHistoryDB) getJobPattern(scriptHash string) (*types.JobPattern, error) {
	query := `
	SELECT script_hash, script_name, run_count, last_run,
		   avg_cpu_efficiency, cpu_variability, typical_effective_cpus,
		   avg_memory_efficiency, memory_variability, typical_memory_usage_gb,
		   avg_requested_ratio, avg_actual_ratio, workload_type,
		   avg_runtime_seconds, runtime_variability, success_rate,
		   local_executions, aws_executions, preferred_platform
	FROM job_patterns WHERE script_hash = ?
	`

	row := h.db.QueryRow(query, scriptHash)

	var pattern types.JobPattern
	var lastRun, avgRuntime int64

	err := row.Scan(
		&pattern.ScriptHash, &pattern.ScriptName, &pattern.RunCount, &lastRun,
		&pattern.AvgCPUEfficiency, &pattern.CPUVariability, &pattern.TypicalEffectiveCPUs,
		&pattern.AvgMemoryEfficiency, &pattern.MemoryVariability, &pattern.TypicalMemoryUsageGB,
		&pattern.AvgRequestedRatio, &pattern.AvgActualRatio, &pattern.WorkloadType,
		&avgRuntime, &pattern.RuntimeVariability, &pattern.SuccessRate,
		&pattern.LocalExecutions, &pattern.AWSExecutions, &pattern.PreferredPlatform,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no pattern found for script hash: %s", scriptHash)
	}
	if err != nil {
		return nil, errors.NewConfigError("getJobPattern", "failed to retrieve job pattern", err)
	}

	pattern.LastRun = time.Unix(lastRun, 0)
	pattern.AvgRuntime = time.Duration(avgRuntime) * time.Second

	return &pattern, nil
}

// storeJobPattern stores or updates a job pattern.
func (h *JobHistoryDB) storeJobPattern(pattern *types.JobPattern) error {
	query := `
	INSERT OR REPLACE INTO job_patterns (
		script_hash, script_name, run_count, last_run,
		avg_cpu_efficiency, cpu_variability, typical_effective_cpus,
		avg_memory_efficiency, memory_variability, typical_memory_usage_gb,
		avg_requested_ratio, avg_actual_ratio, workload_type,
		avg_runtime_seconds, runtime_variability, success_rate,
		local_executions, aws_executions, preferred_platform
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := h.db.Exec(query,
		pattern.ScriptHash, pattern.ScriptName, pattern.RunCount, pattern.LastRun.Unix(),
		pattern.AvgCPUEfficiency, pattern.CPUVariability, pattern.TypicalEffectiveCPUs,
		pattern.AvgMemoryEfficiency, pattern.MemoryVariability, pattern.TypicalMemoryUsageGB,
		pattern.AvgRequestedRatio, pattern.AvgActualRatio, pattern.WorkloadType,
		int64(pattern.AvgRuntime.Seconds()), pattern.RuntimeVariability, pattern.SuccessRate,
		pattern.LocalExecutions, pattern.AWSExecutions, pattern.PreferredPlatform,
	)

	if err != nil {
		return errors.NewConfigError("storeJobPattern", "failed to store job pattern", err)
	}

	return nil
}

// GetJobPatterns retrieves all job patterns for the user.
func (h *JobHistoryDB) GetJobPatterns() ([]types.JobPattern, error) {
	query := `
	SELECT script_hash, script_name, run_count, last_run,
		   avg_cpu_efficiency, cpu_variability, typical_effective_cpus,
		   avg_memory_efficiency, memory_variability, typical_memory_usage_gb,
		   avg_requested_ratio, avg_actual_ratio, workload_type,
		   avg_runtime_seconds, runtime_variability, success_rate,
		   local_executions, aws_executions, preferred_platform
	FROM job_patterns
	ORDER BY last_run DESC
	`

	rows, err := h.db.Query(query)
	if err != nil {
		return nil, errors.NewConfigError("GetJobPatterns", "failed to query job patterns", err)
	}
	defer rows.Close()

	var patterns []types.JobPattern
	for rows.Next() {
		var pattern types.JobPattern
		var lastRun, avgRuntime int64

		err := rows.Scan(
			&pattern.ScriptHash, &pattern.ScriptName, &pattern.RunCount, &lastRun,
			&pattern.AvgCPUEfficiency, &pattern.CPUVariability, &pattern.TypicalEffectiveCPUs,
			&pattern.AvgMemoryEfficiency, &pattern.MemoryVariability, &pattern.TypicalMemoryUsageGB,
			&pattern.AvgRequestedRatio, &pattern.AvgActualRatio, &pattern.WorkloadType,
			&avgRuntime, &pattern.RuntimeVariability, &pattern.SuccessRate,
			&pattern.LocalExecutions, &pattern.AWSExecutions, &pattern.PreferredPlatform,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pattern row: %w", err)
		}

		pattern.LastRun = time.Unix(lastRun, 0)
		pattern.AvgRuntime = time.Duration(avgRuntime) * time.Second

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// Close closes the database connection.
func (h *JobHistoryDB) Close() error {
	if h.db != nil {
		return h.db.Close()
	}
	return nil
}

// GetDatabasePath returns the path to the job history database.
func (h *JobHistoryDB) GetDatabasePath() string {
	return h.dbPath
}

// GetDatabaseSize returns the size of the database file in bytes.
func (h *JobHistoryDB) GetDatabaseSize() (int64, error) {
	info, err := os.Stat(h.dbPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetJobCount returns the total number of jobs stored in the database.
func (h *JobHistoryDB) GetJobCount() (int, error) {
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM job_history").Scan(&count)
	return count, err
}