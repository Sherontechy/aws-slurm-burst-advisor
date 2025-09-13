# CPU-Memory Optimization Strategy

## SLURM CPU Efficiency Data Available

From `sacct`, we can collect:
```bash
sacct --format=JobID,JobName,ReqCPUs,ReqMem,TotalCPU,CPUTime,AveCPU,MaxRSS,Elapsed,CPUEff
```

**Key Metrics:**
- **ReqCPUs**: Number of cores requested
- **ReqMem**: Memory requested
- **TotalCPU**: Actual CPU time used (sum of all cores)
- **CPUTime**: Total CPU time available (ReqCPUs × Elapsed)
- **CPUEff**: CPU efficiency = TotalCPU / CPUTime × 100%
- **MaxRSS**: Peak memory usage (Resident Set Size)
- **AveCPU**: Average CPU usage across tasks

## CPU-Memory Pattern Analysis

### **Pattern Detection**
```go
type ResourcePattern struct {
    // What user requested
    RequestedCPUs    int     `json:"requested_cpus"`
    RequestedMemoryGB float64 `json:"requested_memory_gb"`
    CPUMemoryRatio   float64 `json:"cpu_memory_ratio"`    // GB per CPU

    // What user actually used
    CPUEfficiency    float64 `json:"cpu_efficiency"`      // 0-100%
    MemoryEfficiency float64 `json:"memory_efficiency"`   // 0-100%
    ActualCPUMemRatio float64 `json:"actual_cpu_mem_ratio"` // Effective GB per CPU used

    // Workload classification
    WorkloadType     string  `json:"workload_type"`       // cpu-bound, memory-bound, balanced
    BottleneckType   string  `json:"bottleneck"`          // cpu, memory, io, network
}

func ClassifyWorkload(cpuEff, memEff float64) string {
    if cpuEff > 80 && memEff < 60 {
        return "cpu-bound"      // High CPU usage, low memory → c5/c6i instances
    }
    if memEff > 80 && cpuEff < 60 {
        return "memory-bound"   // High memory usage, low CPU → r5/r6i instances
    }
    if cpuEff > 70 && memEff > 70 {
        return "balanced"       // Both high → m5/m6i instances
    }
    if cpuEff < 50 && memEff < 50 {
        return "over-allocated" // Both low → downsize significantly
    }
    return "mixed"
}
```

### **AWS Instance Family Mapping**
```go
type InstanceRecommendation struct {
    Family          string   `json:"family"`           // c5, r5, m5, etc.
    ReasoningCPU    string   `json:"reasoning_cpu"`
    ReasoningMemory string   `json:"reasoning_memory"`
    CPUMemoryRatio  float64  `json:"cpu_memory_ratio"` // GB per vCPU
    CostEfficiency  string   `json:"cost_efficiency"`
}

var InstanceFamilyOptimization = map[string][]InstanceRecommendation{
    "cpu-bound": {
        {
            Family: "c5",
            CPUMemoryRatio: 2.0,  // 2GB per vCPU
            ReasoningCPU: "High-performance Intel CPUs, optimized for compute",
            ReasoningMemory: "Lower memory cost, perfect for CPU-intensive jobs",
            CostEfficiency: "Best price/performance for CPU workloads",
        },
        {
            Family: "c6i",
            CPUMemoryRatio: 2.0,
            ReasoningCPU: "Latest generation, 15% better price/performance",
            ReasoningMemory: "Efficient memory allocation for compute workloads",
            CostEfficiency: "Newest generation with best efficiency",
        },
    },
    "memory-bound": {
        {
            Family: "r5",
            CPUMemoryRatio: 8.0,  // 8GB per vCPU
            ReasoningCPU: "Moderate CPU performance",
            ReasoningMemory: "High memory-to-CPU ratio, perfect for memory-intensive jobs",
            CostEfficiency: "Best price per GB of memory",
        },
        {
            Family: "r6i",
            CPUMemoryRatio: 8.0,
            ReasoningCPU: "Latest Intel CPUs with better performance",
            ReasoningMemory: "Same memory ratio with faster processors",
            CostEfficiency: "Better performance per dollar for memory workloads",
        },
    },
    "balanced": {
        {
            Family: "m5",
            CPUMemoryRatio: 4.0,  // 4GB per vCPU
            ReasoningCPU: "Balanced CPU performance",
            ReasoningMemory: "Balanced memory allocation",
            CostEfficiency: "Good general-purpose price/performance",
        },
    },
}
```

### **Enhanced Analysis Output**
```bash
asba training.sbatch gpu-aws --optimize-resources

# RESOURCE EFFICIENCY ANALYSIS
# =============================
# Based on YOUR 6 previous runs of training.sbatch:
#
# CPU ANALYSIS:
#   Requested: 32 cores (always request same amount)
#   Efficiency: 45% average (14.4 cores effectively used)
#   Pattern: CPU-underutilized, suggest fewer cores
#
# MEMORY ANALYSIS:
#   Requested: 256GB (8GB per core)
#   Peak usage: 180GB average (70% efficiency)
#   Pattern: Memory-intensive workload
#
# WORKLOAD CLASSIFICATION: Memory-bound
#   CPU efficiency: 45% (low)
#   Memory efficiency: 70% (high)
#   Bottleneck: Memory bandwidth, not CPU cycles
#
# OPTIMIZATION RECOMMENDATIONS:
#
# LOCAL CLUSTER:
#   Current: --nodes=4 --cpus-per-task=8 --mem=64G  (32 cores, 256GB)
#   Optimized: --nodes=2 --cpus-per-task=8 --mem=96G (16 cores, 192GB)
#   Reasoning: Your workload doesn't scale beyond 16 cores
#   Local savings: $18/run (40% reduction)
#
# AWS INSTANCE OPTIMIZATION:
#   Current choice: c5.8xlarge (32 vCPUs, 64GB) + extra memory
#   Better choice: r5.4xlarge (16 vCPUs, 128GB)
#   Reasoning: Memory-bound workload needs high memory-to-CPU ratio
#   AWS savings: $2.10/hr → $1.60/hr (24% reduction)
#   Performance: Same or better (memory is your bottleneck)
#
# UPDATED RECOMMENDATION:
#   With optimization: AWS r5.4xlarge wins by larger margin
#   Cost difference: AWS $32 vs Local $28 (only $4 premium)
#   Time difference: AWS 3.2h vs Local 6h (saves 2h48m)
#   New choice: Burst to AWS for better time/cost ratio
```

## **Implementation Details**

### **Extended sacct Data Collection**
```go
// Collect comprehensive efficiency data
type JobEfficiencyData struct {
    JobID           string
    RequestedCPUs   int
    RequestedMemoryMB int64
    CPUEfficiency   float64  // TotalCPU/CPUTime * 100
    MemoryEfficiency float64 // MaxRSS/ReqMem * 100
    CPUMemoryRatio  float64  // RequestedMemoryMB / RequestedCPUs
    ActualCPUMemRatio float64 // MaxRSS / EffectiveCPUs
}

func (c *Client) GetDetailedJobHistory(ctx context.Context, user string, days int) ([]JobEfficiencyData, error) {
    cmd := exec.CommandContext(ctx, c.binPath+"/sacct",
        "--user", user,
        "--starttime", startTime.Format("2006-01-02"),
        "--format=JobID,ReqCPUs,ReqMem,TotalCPU,CPUTime,MaxRSS,Elapsed",
        "--units=M",  // Memory in MB
        "--noheader",
        "--parsable2",
    )

    // Parse and calculate efficiency metrics
    return c.parseEfficiencyData(output)
}
```

### **Instance Family Recommendation Engine**
```go
type InstanceOptimizer struct {
    awsPricing *aws.PricingClient
}

func (i *InstanceOptimizer) RecommendInstanceFamily(pattern ResourcePattern) []InstanceRecommendation {
    recommendations := []InstanceRecommendation{}

    // Analyze CPU-to-memory ratio needs
    if pattern.WorkloadType == "memory-bound" {
        // Recommend r-family (high memory ratio)
        recommendations = append(recommendations, InstanceRecommendation{
            Family: "r5",
            Reasoning: fmt.Sprintf("Your workload uses %.1fGB per effective CPU, r5 provides 8GB/vCPU",
                      pattern.ActualCPUMemRatio),
            CostImpact: "20-30% better price per GB memory",
            PerformanceImpact: "Same or better performance with right-sized memory",
        })
    } else if pattern.WorkloadType == "cpu-bound" {
        // Recommend c-family (high CPU performance)
        recommendations = append(recommendations, InstanceRecommendation{
            Family: "c5",
            Reasoning: fmt.Sprintf("Your workload has %.0f%% CPU efficiency, c5 optimized for CPU performance",
                      pattern.CPUEfficiency),
            CostImpact: "15-25% better price per vCPU",
            PerformanceImpact: "Higher clock speeds, better CPU performance per core",
        })
    }

    return recommendations
}
```

## **Academic Research Example**

### **Typical Over-Allocation Scenario**
```bash
# Researcher's typical request (following local cluster defaults)
#SBATCH --nodes=4
#SBATCH --cpus-per-task=8      # 32 total cores
#SBATCH --mem=8G               # 8GB per core = 256GB total
#SBATCH --gres=gpu:4

# What sacct reveals after job completion:
# CPUEff: 35% (only using ~11 effective cores)
# MaxRSS: 180GB (70% memory efficiency)
# Pattern: Memory-bound, CPU-underutilized
```

### **Optimized Recommendation**
```bash
asba training.sbatch gpu-aws --analyze-efficiency

# EFFICIENCY ANALYSIS
# ===================
# Your CPU-to-Memory usage pattern:
#   Requested: 32 cores, 256GB (8GB per core)
#   Actually used: ~11 effective cores, 180GB (16GB per effective core)
#   Classification: Memory-intensive, CPU-light
#
# AWS INSTANCE OPTIMIZATION:
#   Current approach: c5.8xlarge + extra memory (expensive)
#   Better approach: r5.4xlarge (16 vCPUs, 128GB)
#   Perfect fit: Your 11 effective cores + 180GB usage
#
# COST COMPARISON:
#   c5.8xlarge: $1.36/hr + $0.50/hr extra memory = $1.86/hr
#   r5.4xlarge: $1.01/hr (native high memory ratio)
#   Savings: $0.85/hr (46% reduction)
#
# PERFORMANCE IMPACT:
#   Same or better: Memory bandwidth is your bottleneck, not CPU count
#   Risk: Very low (you never use >16 effective cores)
```

This approach gives researchers **actionable insights** about their actual resource usage patterns and how to optimize for both local and AWS execution. The key insight is that **right-sizing changes the AWS vs local recommendation** because it affects the cost calculations significantly!

Perfect strategy for academic researchers who need to maximize their research output within grant budget constraints.