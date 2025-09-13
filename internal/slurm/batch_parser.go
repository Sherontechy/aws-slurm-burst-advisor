package slurm

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

var (
	sbatchDirectiveRegex = regexp.MustCompile(`^#SBATCH\s+(.+)`)
	arrayJobRegex        = regexp.MustCompile(`^(\d+)-(\d+)(?::(\d+))?$`)
)

// ParseBatchScript parses a SLURM batch script and extracts job requirements
func ParseBatchScript(filename string) (*types.BatchScript, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open batch script: %w", err)
	}
	defer file.Close()

	script := &types.BatchScript{
		Filename:      filename,
		GRES:          make(map[string]int),
		RawDirectives: make(map[string]string),
		Features:      make([]string, 0),
		Constraints:   make([]string, 0),
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Stop processing directives after first non-comment line
		if !strings.HasPrefix(line, "#") && line != "" {
			break
		}

		// Skip non-SBATCH comments
		if !sbatchDirectiveRegex.MatchString(line) {
			continue
		}

		// Extract directive
		matches := sbatchDirectiveRegex.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}

		directive := strings.TrimSpace(matches[1])
		if err := parseDirective(script, directive, lineNum); err != nil {
			// Log warning but continue parsing
			fmt.Printf("Warning line %d: failed to parse directive '%s': %v\n", lineNum, directive, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading batch script: %w", err)
	}

	// Validate required fields
	if err := validateBatchScript(script); err != nil {
		return nil, fmt.Errorf("invalid batch script: %w", err)
	}

	return script, nil
}

// parseDirective parses a single SBATCH directive
func parseDirective(s *types.BatchScript, directive string, lineNum int) error {
	// Handle quoted arguments and split properly
	args, err := parseDirectiveArgs(directive)
	if err != nil {
		return fmt.Errorf("failed to parse arguments: %w", err)
	}

	if len(args) == 0 {
		return nil
	}

	arg := args[0]

	// Store raw directive for reference
	s.RawDirectives[arg] = strings.Join(args[1:], " ")

	// Parse specific directives
	switch {
	case strings.HasPrefix(arg, "--job-name=") || arg == "-J":
		s.JobName = extractValue(arg, args)
	case strings.HasPrefix(arg, "--partition=") || arg == "-p":
		s.Partition = extractValue(arg, args)
	case strings.HasPrefix(arg, "--nodes=") || arg == "-N":
		val := extractValue(arg, args)
		if nodes, err := parseNodeSpec(val); err == nil {
			s.Nodes = nodes
		}
	case strings.HasPrefix(arg, "--ntasks=") || arg == "-n":
		val := extractValue(arg, args)
		if ntasks, err := strconv.Atoi(val); err == nil {
			// Calculate nodes if not specified separately
			if s.Nodes == 0 && s.NTasksPerNode > 0 {
				s.Nodes = (ntasks + s.NTasksPerNode - 1) / s.NTasksPerNode
			}
		}
	case strings.HasPrefix(arg, "--ntasks-per-node="):
		val := extractValue(arg, args)
		if ntasks, err := strconv.Atoi(val); err == nil {
			s.NTasksPerNode = ntasks
		}
	case strings.HasPrefix(arg, "--cpus-per-task=") || arg == "-c":
		val := extractValue(arg, args)
		if cpus, err := strconv.Atoi(val); err == nil {
			s.CPUsPerTask = cpus
		}
	case strings.HasPrefix(arg, "--time=") || arg == "-t":
		val := extractValue(arg, args)
		s.TimeLimit = parseSlurmTimeFormat(val)
	case strings.HasPrefix(arg, "--mem="):
		s.Memory = extractValue(arg, args)
	case strings.HasPrefix(arg, "--mem-per-cpu="):
		// Store as memory spec for analysis
		val := extractValue(arg, args)
		s.Memory = fmt.Sprintf("%s-per-cpu", val)
	case strings.HasPrefix(arg, "--mem-per-gpu="):
		val := extractValue(arg, args)
		s.Memory = fmt.Sprintf("%s-per-gpu", val)
	case strings.HasPrefix(arg, "--gres="):
		val := extractValue(arg, args)
		parseGRES(s, val)
	case strings.HasPrefix(arg, "--account=") || arg == "-A":
		s.Account = extractValue(arg, args)
	case strings.HasPrefix(arg, "--qos=") || arg == "-q":
		s.QOS = extractValue(arg, args)
	case strings.HasPrefix(arg, "--constraint=") || arg == "-C":
		val := extractValue(arg, args)
		s.Constraints = append(s.Constraints, strings.Split(val, "&")...)
	case strings.HasPrefix(arg, "--dependency=") || arg == "-d":
		// Store dependency info in raw directives (complex parsing)
		val := extractValue(arg, args)
		s.RawDirectives["dependency"] = val
	case strings.HasPrefix(arg, "--array="):
		// Handle job arrays
		val := extractValue(arg, args)
		s.RawDirectives["array"] = val
	case strings.HasPrefix(arg, "--exclusive"):
		s.RawDirectives["exclusive"] = "true"
	case strings.HasPrefix(arg, "--mail-type="):
		s.RawDirectives["mail-type"] = extractValue(arg, args)
	case strings.HasPrefix(arg, "--mail-user="):
		s.RawDirectives["mail-user"] = extractValue(arg, args)
	case strings.HasPrefix(arg, "--output=") || arg == "-o":
		s.RawDirectives["output"] = extractValue(arg, args)
	case strings.HasPrefix(arg, "--error=") || arg == "-e":
		s.RawDirectives["error"] = extractValue(arg, args)
	}

	return nil
}

// parseDirectiveArgs properly parses directive arguments, handling quotes
func parseDirectiveArgs(directive string) ([]string, error) {
	var args []string
	var current strings.Builder
	var inQuotes bool
	var quoteChar rune

	runes := []rune(directive)
	for i, r := range runes {
		switch {
		case !inQuotes && (r == '"' || r == '\''):
			inQuotes = true
			quoteChar = r
		case inQuotes && r == quoteChar:
			inQuotes = false
			quoteChar = 0
		case !inQuotes && r == ' ':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		case inQuotes && r == '\\' && i+1 < len(runes):
			// Handle escape sequences
			next := runes[i+1]
			switch next {
			case 'n':
				current.WriteRune('\n')
			case 't':
				current.WriteRune('\t')
			case 'r':
				current.WriteRune('\r')
			case '\\':
				current.WriteRune('\\')
			case '"':
				current.WriteRune('"')
			case '\'':
				current.WriteRune('\'')
			default:
				current.WriteRune(r)
				current.WriteRune(next)
			}
			// Skip next character - need to update loop index properly
			i++
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if inQuotes {
		return nil, fmt.Errorf("unclosed quote in directive")
	}

	return args, nil
}

// extractValue extracts value from argument (either --arg=value or --arg value)
func extractValue(arg string, args []string) string {
	if strings.Contains(arg, "=") {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	if len(args) > 1 {
		return args[1]
	}

	return ""
}

// parseNodeSpec parses node specification (could be number or range)
func parseNodeSpec(spec string) (int, error) {
	// Handle simple number
	if nodes, err := strconv.Atoi(spec); err == nil {
		return nodes, nil
	}

	// Handle range specification like "1-4"
	if strings.Contains(spec, "-") {
		parts := strings.Split(spec, "-")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && end >= start {
				return end - start + 1, nil
			}
		}
	}

	return 0, fmt.Errorf("invalid node specification: %s", spec)
}

// parseSlurmTimeFormat parses SLURM time format into time.Duration
func parseSlurmTimeFormat(timeStr string) time.Duration {
	if timeStr == "" || timeStr == "UNLIMITED" {
		return 0
	}

	// Handle different time formats:
	// - "minutes"
	// - "minutes:seconds"
	// - "hours:minutes:seconds"
	// - "days-hours:minutes:seconds"

	// Check for days
	if strings.Contains(timeStr, "-") {
		parts := strings.SplitN(timeStr, "-", 2)
		if len(parts) == 2 {
			days, err := strconv.Atoi(parts[0])
			if err == nil {
				daysDuration := time.Duration(days) * 24 * time.Hour
				timePart := parseSlurmTimeFormat(parts[1])
				return daysDuration + timePart
			}
		}
	}

	// Split by colons
	parts := strings.Split(timeStr, ":")

	switch len(parts) {
	case 1:
		// Just minutes
		if minutes, err := strconv.Atoi(parts[0]); err == nil {
			return time.Duration(minutes) * time.Minute
		}
	case 2:
		// minutes:seconds
		minutes, err1 := strconv.Atoi(parts[0])
		seconds, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			return time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
		}
	case 3:
		// hours:minutes:seconds
		hours, err1 := strconv.Atoi(parts[0])
		minutes, err2 := strconv.Atoi(parts[1])
		seconds, err3 := strconv.Atoi(parts[2])
		if err1 == nil && err2 == nil && err3 == nil {
			return time.Duration(hours)*time.Hour +
				time.Duration(minutes)*time.Minute +
				time.Duration(seconds)*time.Second
		}
	}

	return 0
}

// parseGRES parses Generic RESource specification
func parseGRES(s *types.BatchScript, gresStr string) {
	// Handle multiple GRES specifications separated by commas
	specs := strings.Split(gresStr, ",")

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		// Parse format: resource:type:count or resource:count
		parts := strings.Split(spec, ":")
		if len(parts) < 2 {
			continue
		}

		resource := parts[0]
		var count int
		var err error

		if len(parts) == 2 {
			// resource:count format
			count, err = strconv.Atoi(parts[1])
		} else if len(parts) >= 3 {
			// resource:type:count format
			count, err = strconv.Atoi(parts[len(parts)-1])
		}

		if err == nil && count > 0 {
			s.GRES[resource] = count
		}
	}
}

// validateBatchScript performs basic validation on the parsed batch script
func validateBatchScript(s *types.BatchScript) error {
	if s.JobName == "" {
		s.JobName = "batch_job" // Default name
	}

	if s.Nodes <= 0 {
		s.Nodes = 1 // Default to 1 node
	}

	if s.CPUsPerTask <= 0 {
		s.CPUsPerTask = 1 // Default to 1 CPU per task
	}

	if s.NTasksPerNode <= 0 {
		s.NTasksPerNode = 1 // Default to 1 task per node
	}

	return nil
}

// EstimateResourceRequirements calculates total resource requirements
func EstimateResourceRequirements(s *types.BatchScript) map[string]interface{} {
	reqs := make(map[string]interface{})

	reqs["total_nodes"] = s.Nodes
	reqs["total_tasks"] = s.Nodes * s.NTasksPerNode
	reqs["total_cpus"] = s.Nodes * s.NTasksPerNode * s.CPUsPerTask
	reqs["runtime"] = s.TimeLimit

	if len(s.GRES) > 0 {
		totalGPUs := 0
		for resource, count := range s.GRES {
			totalCount := s.Nodes * count
			reqs["total_"+resource] = totalCount
			if resource == "gpu" {
				totalGPUs = totalCount
			}
		}
		reqs["total_gpus"] = totalGPUs
	}

	if s.Memory != "" {
		reqs["memory_spec"] = s.Memory
	}

	return reqs
}

// IsArrayJob returns true if this is a job array
func IsArrayJob(s *types.BatchScript) bool {
	_, exists := s.RawDirectives["array"]
	return exists
}

// GetArrayJobCount returns the number of jobs in an array job
func GetArrayJobCount(s *types.BatchScript) int {
	arraySpec, exists := s.RawDirectives["array"]
	if !exists {
		return 1
	}

	// Parse array specification like "1-100" or "1-100:2"
	matches := arrayJobRegex.FindStringSubmatch(arraySpec)
	if len(matches) >= 3 {
		start, _ := strconv.Atoi(matches[1])
		end, _ := strconv.Atoi(matches[2])
		step := 1

		if len(matches) >= 4 && matches[3] != "" {
			step, _ = strconv.Atoi(matches[3])
		}

		if step > 0 && end >= start {
			return ((end - start) / step) + 1
		}
	}

	return 1
}

// HasDependencies returns true if the job has dependencies
func HasDependencies(s *types.BatchScript) bool {
	_, exists := s.RawDirectives["dependency"]
	return exists
}

// IsGPUJob returns true if the job requires GPUs
func IsGPUJob(s *types.BatchScript) bool {
	return s.GRES["gpu"] > 0
}

// IsExclusive returns true if the job requires exclusive node access
func IsExclusive(s *types.BatchScript) bool {
	exclusive, exists := s.RawDirectives["exclusive"]
	return exists && exclusive == "true"
}