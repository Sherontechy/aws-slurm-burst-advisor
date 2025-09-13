# AWS SLURM Burst Advisor Roadmap

## Current State (v0.1.0)
- Basic queue analysis and cost comparison
- Static resource request analysis from batch scripts
- Simple AWS EC2 vs local cluster recommendations
- Academic researcher-focused documentation

## Vision: Intelligent AWS vs Local Decisions

Transform from a static comparison tool to an **intelligent advisor** that learns from your job execution history to provide better AWS vs local recommendations and optimize resource requests for either platform.

**Core Focus**:
1. **Better AWS vs local decisions** based on your actual job patterns
2. **Smarter resource requests** that improve performance on either platform
3. **Personal optimization** that prevents over/under-allocation mistakes

**Philosophy**: Use simple, interpretable heuristics based on your own job history. Every recommendation improves your AWS vs local decision-making and resource efficiency.

---

## Phase 1: Historical Job Tracking (v0.2.0)

### **1.1 Job Execution History with CPU-Memory Efficiency**
```go
// Enhanced job execution tracking focusing on resource efficiency
type JobExecution struct {
    JobID           string    `json:"job_id"`
    User            string    `json:"user"`
    ScriptPath      string    `json:"script_path"`
    ScriptHash      string    `json:"script_hash"`      // Detect identical scripts
    Partition       string    `json:"partition"`

    // Resource requests
    RequestedCPUs   int       `json:"requested_cpus"`
    RequestedMemoryMB int64   `json:"requested_memory_mb"`
    RequestedGPUs   int       `json:"requested_gpus"`
    RequestedTime   time.Duration `json:"requested_time"`
    CPUMemoryRatio  float64   `json:"cpu_memory_ratio"`    // GB per CPU requested

    // Actual usage (from sacct detailed fields)
    ActualTime      time.Duration `json:"actual_time"`
    MaxMemoryUsedMB int64     `json:"max_memory_used_mb"`  // MaxRSS
    CPUEfficiency   float64   `json:"cpu_efficiency"`     // TotalCPU/CPUTime * 100
    MemoryEfficiency float64  `json:"memory_efficiency"`  // MaxRSS/ReqMem * 100
    EffectiveCPUs   float64   `json:"effective_cpus"`     // ReqCPUs * CPUEff/100
    ActualCPUMemRatio float64 `json:"actual_cpu_mem_ratio"` // MaxRSS/EffectiveCPUs

    // Execution details
    ExitCode        int       `json:"exit_code"`
    StartTime       time.Time `json:"start_time"`
    EndTime         time.Time `json:"end_time"`
    QueueWaitTime   time.Duration `json:"queue_wait_time"`
    ExecutionPlatform string  `json:"execution_platform"` // "local" or "aws"
}
```

### **1.2 Simple History Storage**
```bash
# Store in user's home directory
~/.asba/
├── history.json        # Job execution history
├── patterns.json       # Detected usage patterns
└── cache/             # Cached analyses
```

### **1.3 CPU-Memory Pattern Recognition**
```go
// Enhanced pattern recognition focusing on CPU-memory efficiency
type JobPattern struct {
    ScriptName      string    `json:"script_name"`
    RunCount        int       `json:"run_count"`

    // CPU analysis
    TypicalCPUEff   float64   `json:"typical_cpu_efficiency"`    // Average CPU utilization
    EffectiveCores  float64   `json:"effective_cores"`          // Actual cores used on average
    CPUVariability  float64   `json:"cpu_variability"`          // How consistent CPU usage is

    // Memory analysis
    TypicalMemUsageGB float64 `json:"typical_memory_usage_gb"`   // Average peak memory
    MemoryEfficiency  float64 `json:"memory_efficiency"`        // How much of requested memory used
    MemoryVariability float64 `json:"memory_variability"`       // How consistent memory usage is

    // CPU-Memory relationship
    RequestedRatio    float64 `json:"requested_ratio"`          // GB per requested CPU
    ActualRatio       float64 `json:"actual_ratio"`             // GB per effective CPU
    WorkloadType      string  `json:"workload_type"`            // cpu-bound, memory-bound, balanced

    // Success patterns
    SuccessRate       float64 `json:"success_rate"`
    AvgRuntime        time.Duration `json:"avg_runtime"`
}

// Detection logic
func DetectPatterns(executions []JobExecution) []JobPattern {
    patterns := make(map[string][]JobExecution)

    // Group by script name/hash
    for _, exec := range executions {
        key := filepath.Base(exec.ScriptPath)
        patterns[key] = append(patterns[key], exec)
    }

    // Calculate simple statistics
    var result []JobPattern
    for scriptName, jobs := range patterns {
        if len(jobs) >= 2 { // Need at least 2 runs for patterns
            pattern := calculateStats(jobs)
            result = append(result, pattern)
        }
    }
    return result
}
```

---

## Phase 2: Practical Optimization (v0.3.0)

### **2.1 Resource Right-Sizing for Better AWS vs Local Decisions**
```bash
# Example analysis output
asba training.sbatch gpu-aws --optimize

# HISTORICAL ANALYSIS (based on 5 similar runs)
# ============================================
#
# SCRIPT: training.sbatch (run 5 times in last 90 days)
# Your typical efficiency:
#   CPU: 72% (you request 64 cores, actually use ~46)
#   Memory: 58% (you request 256GB, peak usage 148GB)
#   Runtime: 3.2h average (you request 6h limit)
#
# IMPACT ON AWS vs LOCAL DECISION:
#
# CURRENT REQUEST (over-allocated):
#   Local cost: $45, Queue wait: 4h → Total: 7.2h, $45
#   AWS cost: $120, Startup: 3m → Total: 3.3h, $120
#   Current recommendation: Use local (save $75)
#
# OPTIMIZED REQUEST (right-sized):
#   Local cost: $28, Queue wait: 2h → Total: 5.2h, $28
#   AWS cost: $65, Startup: 3m → Total: 3.3h, $65
#   Better recommendation: Use AWS (save 1.9h for $37)
#
# KEY INSIGHT: Right-sizing changes the AWS vs local decision!
# Over-allocation made local look cheaper, but optimized resources favor AWS
```

### **2.2 Instance Type Matching**
```go
// Simple workload classification based on observed patterns
type WorkloadClassifier struct{}

func (w *WorkloadClassifier) ClassifyFromHistory(patterns []JobPattern) WorkloadType {
    // Simple heuristics:
    if avgCPUEff > 0.8 && avgMemEff < 0.6 {
        return WorkloadTypeCPUBound
    }
    if avgMemEff > 0.8 && avgCPUEff < 0.6 {
        return WorkloadTypeMemoryBound
    }
    // etc.
}

// Instance family recommendations
var InstanceFamilyMap = map[WorkloadType][]InstanceRecommendation{
    WorkloadTypeCPUBound: {
        {Family: "c5", Reasoning: "High CPU performance, lower memory cost"},
        {Family: "c6i", Reasoning: "Latest generation, better price/performance"},
    },
    WorkloadTypeMemoryBound: {
        {Family: "r5", Reasoning: "High memory-to-CPU ratio"},
        {Family: "x1e", Reasoning: "Ultra-high memory for large datasets"},
    },
}
```

### **2.3 Smart Defaults**
```go
// Learn user's typical patterns
type UserDefaults struct {
    User                string
    TypicalCPURequest   int     // What they usually ask for
    TypicalCPUUsage     int     // What they actually use
    PreferredInstances  []string // Instances they've had success with
    AvoidedInstances    []string // Instances that failed/performed poorly
    BudgetSensitivity   float64  // 0-1, derived from past decisions
}

// Auto-suggest based on simple patterns
func SuggestResourceAdjustments(job JobRequest, userDefaults UserDefaults) []Suggestion
```

---

## Phase 3: Advanced Heuristics (v0.4.0)

### **3.1 Failure Pattern Recognition**
```go
// Track failure patterns without ML
type FailurePattern struct {
    Pattern         string   // "OOM on r5.large with >100GB datasets"
    Occurrences     int      // How often this happens
    Prevention      string   // "Use r5.xlarge or reduce batch size"
    ConfidenceLevel float64  // Based on observation frequency
}

// Simple rule matching
func CheckFailureRisk(job JobRequest, targetInstance string, history []JobExecution) []RiskWarning
```

### **3.2 Queue-Aware Optimization**
```go
// Consider both resource efficiency AND queue conditions
type QueueAwareOptimizer struct {}

func (q *QueueAwareOptimizer) OptimizeWithContext(
    job JobRequest,
    queueState QueueInfo,
    userHistory []JobExecution,
    budgetRemaining float64,
) OptimizationRecommendation

type OptimizationRecommendation struct {
    ResourceAdjustments []ResourceSuggestion
    InstanceTypeRecs    []InstanceRecommendation
    TimingAdvice        string // "Submit now" vs "wait 2h for better queue"
    BudgetImpact        string // "This will use 15% of monthly budget"
}
```

### **3.3 Research Pipeline Awareness**
```bash
# Detect related jobs in pipelines
asba step1.sbatch cpu-aws --pipeline-mode

# "Detected: Part of multi-step workflow
#  Previous steps: preprocessing.sbatch (completed 2h ago)
#  Next likely step: analysis.sbatch (based on past patterns)
#
#  Pipeline recommendation:
#  - Run step1 on local (queue is short, save budget for step2)
#  - Plan step2 for AWS (CPU-intensive, benefits from scaling)
#  - Reserve budget: $120 for full pipeline vs $200 all-AWS"
```

---

## Phase 4: Research Intelligence (v1.0.0)

### **4.1 Research Domain Optimization**
```go
// Domain-specific optimization rules (not ML)
type ResearchDomainOptimizer struct {
    domain string // "machine-learning", "bioinformatics", "climate"
}

// Hand-crafted rules based on research computing knowledge
var DomainOptimizations = map[string][]OptimizationRule{
    "machine-learning": {
        {
            Pattern: "transformer training",
            Suggestion: "Use memory-optimized instances (r5/r6i) for large models",
            Reasoning: "Transformer attention scales quadratically with memory",
        },
        {
            Pattern: "distributed training",
            Suggestion: "Prefer fewer, larger instances over many small ones",
            Reasoning: "Reduces inter-node communication overhead",
        },
    },
    "bioinformatics": {
        {
            Pattern: "blast search",
            Suggestion: "Use compute-optimized instances (c5/c6i)",
            Reasoning: "CPU-intensive with linear scaling",
        },
    },
}
```

### **4.2 Collaborative Learning**
```go
// Share anonymized patterns across research groups
type CollaborativeInsights struct {
    AnonymizedPatterns []WorkloadPattern
    BestPractices     []BestPractice
    CommonMistakes    []CommonMistake
}

type BestPractice struct {
    Workload     string
    Optimization string
    SavingsPercent float64
    AdoptionRate   float64
}
```

---

## Why This Approach Works Better

### **🎯 Focused on Practical Gains**
- **Simple heuristics** that work with limited data
- **Rule-based optimization** using HPC domain knowledge
- **Statistical analysis** rather than complex ML models
- **Interpretable recommendations** researchers can understand

### **📊 Realistic Data Expectations**
- **Individual patterns**: 50-200 jobs/year per researcher
- **Script similarity**: Researchers often run variations of same experiments
- **Resource patterns**: Clear efficiency trends emerge quickly
- **Failure patterns**: Common mistakes repeated across users

### **🔄 Continuous Improvement**
- **User feedback loop**: Track which recommendations were accepted
- **Pattern refinement**: Improve heuristics based on observed outcomes
- **Domain knowledge**: Incorporate research computing best practices
- **Community learning**: Share insights across research institutions

---

## Implementation Strategy

### **Phase 1 Focus**: Personal job history and basic optimization
- Track YOUR job execution history with `sacct` integration
- Detect when you run identical/similar scripts
- Calculate YOUR efficiency statistics (CPU/memory utilization)
- Provide personalized right-sizing suggestions that improve AWS vs local decisions

### **Phase 2 Focus**: Smarter AWS vs local recommendations
- Use YOUR resource usage patterns to make better burst decisions
- Recommend optimal AWS instance types based on YOUR workload characteristics
- Show how resource optimization changes the AWS vs local cost/time trade-off
- Help you avoid common over/under-allocation mistakes on both platforms

### **Phase 3 Focus**: Advanced decision intelligence
- Predict job success/failure risk on different platforms
- Consider YOUR research timeline patterns and budget constraints
- Provide workflow-level optimization for YOUR typical research patterns
- Track which recommendations you accept to improve future advice

### **Core Value**: Every feature improves your ability to make smart AWS vs local decisions and optimize resources for better performance on either platform.

This approach provides immediate, personal value while building toward more sophisticated optimization based on your own job execution patterns.

---

## Phase 1 Implementation Plan (v0.2.0)

### **Week 1-2: History Collection Foundation**

#### **New CLI Options**
```bash
# Enable history tracking (opt-in initially)
asba job.sbatch gpu-aws --track-history

# View your job history
asba history --days 30
asba history --script training.sbatch

# Show efficiency insights
asba insights --job-id 12345
```

#### **Database Schema**
```sql
-- Simple SQLite in ~/.asba/jobs.db
CREATE TABLE job_history (
    job_id TEXT PRIMARY KEY,
    user TEXT,
    script_path TEXT,
    script_hash TEXT,
    submission_time INTEGER,

    -- Resource requests
    req_nodes INTEGER,
    req_cpus INTEGER,
    req_memory_mb INTEGER,
    req_gpus INTEGER,
    req_time_seconds INTEGER,

    -- Actual usage from sacct
    actual_time_seconds INTEGER,
    max_memory_mb INTEGER,
    cpu_efficiency REAL,        -- 0-100%

    -- Execution context
    partition TEXT,
    exit_code INTEGER,
    queue_wait_seconds INTEGER,

    -- For AWS vs local analysis
    execution_platform TEXT     -- 'local' or 'aws'
);

CREATE INDEX idx_script_hash ON job_history(script_hash);
CREATE INDEX idx_submission_time ON job_history(submission_time);
```

### **Week 3-4: Basic Pattern Recognition**

#### **Enhanced Analysis Output**
```bash
asba training.sbatch gpu-aws --with-history

# CURRENT ANALYSIS
# ================
# Queue depth: 8 jobs, Est. wait: 2h 15m
# Local cost: $32, AWS cost: $58
# Current recommendation: Use local cluster
#
# PERSONAL HISTORY INSIGHTS
# ==========================
# You've run training.sbatch 4 times before:
#   Run 1 (local): 2.1h runtime, 87GB memory, 68% CPU efficiency
#   Run 2 (AWS):   1.9h runtime, 82GB memory, 74% CPU efficiency
#   Run 3 (local): 2.3h runtime, 91GB memory, 71% CPU efficiency
#   Run 4 (AWS):   2.0h runtime, 85GB memory, 72% CPU efficiency
#
# PATTERNS DETECTED:
#   ✓ Consistent runtime: ~2h (you request 6h)
#   ✓ Memory over-allocation: You use ~85GB, request 256GB
#   ✓ CPU efficiency: 70% average (room for optimization)
#   ✓ Platform performance: Similar results on AWS vs local
#
# OPTIMIZATION OPPORTUNITY:
#   Suggested: --nodes=2 --mem=128G --time=3:00:00
#   Impact: Changes recommendation to AWS (better cost/time with right-sizing)
```

### **Week 5-6: Intelligent Resource Suggestions**

#### **Smart Defaults Based on History**
```go
type PersonalOptimizer struct {
    userHistory []JobExecution
    patterns    map[string]JobPattern
}

func (p *PersonalOptimizer) OptimizeJobRequest(job *types.JobRequest, scriptPath string) OptimizationSuggestion {
    // Find similar jobs in user's history
    similar := p.findSimilarJobs(scriptPath, job)

    if len(similar) < 2 {
        return OptimizationSuggestion{Message: "No history available yet"}
    }

    // Calculate efficiency-based suggestions
    suggestions := []ResourceSuggestion{}

    // Memory optimization
    if avgMemUsage := calculateAvgMemory(similar); avgMemUsage < parseMemory(job.Memory)*0.7 {
        suggestions = append(suggestions, ResourceSuggestion{
            Type: "memory",
            Current: job.Memory,
            Suggested: formatMemory(avgMemUsage * 1.2), // 20% buffer
            Reasoning: fmt.Sprintf("Your average usage: %s (%.0f%% of request)",
                      formatMemory(avgMemUsage), avgMemUsage/parseMemory(job.Memory)*100),
            ImpactLocal: calculateLocalSavings(job, "memory", avgMemUsage*1.2),
            ImpactAWS: calculateAWSSavings(job, "memory", avgMemUsage*1.2),
        })
    }

    return OptimizationSuggestion{
        Suggestions: suggestions,
        ConfidenceLevel: calculateConfidence(similar),
        DecisionImpact: "Right-sizing may change AWS vs local recommendation",
    }
}
```

### **Week 7-8: AWS Instance Type Intelligence**

#### **Personal Instance Recommendations**
```bash
asba training.sbatch gpu-aws --recommend-instance

# INSTANCE TYPE ANALYSIS
# ======================
# Current choice: p3.8xlarge (32 vCPUs, 244GB RAM, 4x V100)
#
# Based on YOUR job patterns:
#   Workload type: Memory-bound (high memory efficiency, moderate CPU)
#   GPU utilization: ~60% (based on your typical batch sizes)
#   Network usage: Low (single-node jobs)
#
# BETTER INSTANCE OPTIONS:
#   g4dn.4xlarge: $1.20/hr vs $12.24/hr (90% cost savings)
#     Trade-off: 25% slower training, but 1/10th the cost
#     Best for: Prototype training, hyperparameter tuning
#
#   p3.2xlarge: $3.06/hr vs $12.24/hr (75% cost savings)
#     Trade-off: 15% slower, but fits your memory patterns
#     Best for: Production training with budget constraints
#
# RECOMMENDATION: Try g4dn.4xlarge for this job
# Reasoning: Your memory usage (85GB) fits comfortably
# Risk: Low (similar memory-bound jobs perform well on g4dn)
```

### **Success Metrics for v0.2.0**
- [ ] 90% of users see immediate value after 3+ job runs
- [ ] 15% average resource optimization (CPU-memory right-sizing)
- [ ] 25% improvement in AWS vs local decision accuracy
- [ ] 80% of recommendations correctly identify CPU vs memory bottlenecks
- [ ] Zero setup overhead - works out of the box

---

## Next Steps: Start Phase 1 Development

### **Immediate Development Plan**

#### **Step 1: Extend SLURM Client for Efficiency Data**
```go
// Add to internal/slurm/client.go
func (c *Client) GetUserJobEfficiency(ctx context.Context, user string, days int) ([]JobEfficiencyData, error) {
    cmd := exec.CommandContext(ctx, c.binPath+"/sacct",
        "--user", user,
        "--starttime", startTime.Format("2006-01-02"),
        "--format=JobID,JobName,ReqCPUs,ReqMem,TotalCPU,CPUTime,MaxRSS,Elapsed,ExitCode,Partition",
        "--units=M",
        "--noheader",
        "--parsable2",
    )

    output, err := cmd.Output()
    if err != nil {
        return nil, errors.NewSLURMError("GetUserJobEfficiency", "sacct command failed", err)
    }

    return c.parseJobEfficiencyData(string(output))
}
```

#### **Step 2: Create Job History Database**
```go
// Add internal/history package
type JobHistoryDB struct {
    db   *sql.DB
    user string
}

func NewJobHistoryDB(user string) (*JobHistoryDB, error) {
    dbPath := filepath.Join(os.Getenv("HOME"), ".asba", "jobs.db")
    // Create database with schema for CPU-memory efficiency tracking
}
```

#### **Step 3: Add History-Aware Analysis**
```go
// Extend internal/analyzer/decision_engine.go
type HistoryAwareAnalyzer struct {
    baseAnalyzer *DecisionEngine
    historyDB    *history.JobHistoryDB
}

func (h *HistoryAwareAnalyzer) AnalyzeWithHistory(job *types.JobRequest, scriptPath string) (*EnhancedAnalysis, error) {
    // 1. Run current analysis (queue + cost)
    baseAnalysis := h.baseAnalyzer.Compare(local, aws, job)

    // 2. Look for similar jobs in user history
    similar := h.historyDB.FindSimilarJobs(scriptPath, job)

    // 3. Generate resource optimization suggestions
    optimizations := h.generateOptimizations(job, similar)

    // 4. Re-run analysis with optimized resources
    if len(optimizations) > 0 {
        optimizedJob := h.applyOptimizations(job, optimizations)
        optimizedAnalysis := h.baseAnalyzer.Compare(local, aws, optimizedJob)

        return &EnhancedAnalysis{
            Current:      baseAnalysis,
            Optimized:    optimizedAnalysis,
            Optimizations: optimizations,
            HistoryInsights: h.generateInsights(similar),
        }
    }

    return &EnhancedAnalysis{Current: baseAnalysis}
}
```

#### **Step 4: CLI Integration**
```bash
# Add new flags to main.go
--track-history     # Enable job history collection
--with-history      # Show historical insights
--optimize         # Suggest resource optimizations
--recommend-instance # Suggest better AWS instance types

# Usage examples:
asba job.sbatch gpu-aws --with-history --optimize
asba history --script training.sbatch --days 90
asba insights --efficiency --user researcher
```

This approach focuses on the core value: **better AWS vs local decisions through intelligent resource optimization based on your actual job execution patterns**.