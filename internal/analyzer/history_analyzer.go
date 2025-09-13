package analyzer

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/history"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// HistoryAwareAnalyzer enhances the base decision engine with historical job data.
type HistoryAwareAnalyzer struct {
	baseAnalyzer *DecisionEngine
	historyDB    *history.JobHistoryDB
}

// NewHistoryAwareAnalyzer creates a new analyzer with historical capabilities.
func NewHistoryAwareAnalyzer(weights types.DecisionWeights, historyDB *history.JobHistoryDB) *HistoryAwareAnalyzer {
	return &HistoryAwareAnalyzer{
		baseAnalyzer: NewDecisionEngine(weights),
		historyDB:    historyDB,
	}
}

// EnhancedAnalysis represents analysis results with historical insights and optimization suggestions.
type EnhancedAnalysis struct {
	// Current analysis (without optimization)
	Current *types.Analysis `json:"current"`

	// Optimized analysis (with resource right-sizing)
	Optimized *types.Analysis `json:"optimized,omitempty"`

	// Historical insights
	HistoryInsights *HistoryInsights `json:"history_insights,omitempty"`

	// Resource optimization suggestions
	ResourceOptimizations []ResourceOptimization `json:"resource_optimizations,omitempty"`

	// AWS instance recommendations
	InstanceRecommendations []InstanceRecommendation `json:"instance_recommendations,omitempty"`

	// Decision impact analysis
	DecisionImpact *DecisionImpact `json:"decision_impact,omitempty"`
}

// HistoryInsights provides insights from historical job executions.
type HistoryInsights struct {
	SimilarJobsFound   int                         `json:"similar_jobs_found"`
	JobPattern         *types.JobPattern           `json:"job_pattern,omitempty"`
	EfficiencyTrends   *EfficiencyTrends          `json:"efficiency_trends"`
	PerformanceHistory []types.JobEfficiencyData  `json:"performance_history,omitempty"`
	Confidence         float64                     `json:"confidence"`
}

// EfficiencyTrends shows how resource usage has trended over time.
type EfficiencyTrends struct {
	CPUTrend    string  `json:"cpu_trend"`    // "improving", "declining", "stable"
	MemoryTrend string  `json:"memory_trend"`
	TimeTrend   string  `json:"time_trend"`

	CPUEfficiencyAvg    float64 `json:"cpu_efficiency_avg"`
	MemoryEfficiencyAvg float64 `json:"memory_efficiency_avg"`
	TimeEfficiencyAvg   float64 `json:"time_efficiency_avg"`
}

// ResourceOptimization suggests improvements to resource requests.
type ResourceOptimization struct {
	ResourceType    string  `json:"resource_type"` // "cpu", "memory", "time"
	CurrentValue    string  `json:"current_value"`
	SuggestedValue  string  `json:"suggested_value"`
	Reasoning       string  `json:"reasoning"`
	ConfidenceLevel float64 `json:"confidence_level"`
	LocalSavings    float64 `json:"local_savings"`
	AWSSavings      float64 `json:"aws_savings"`
	RiskLevel       string  `json:"risk_level"` // "low", "medium", "high"
}

// InstanceRecommendation suggests better AWS instance types based on workload patterns.
type InstanceRecommendation struct {
	InstanceFamily   string  `json:"instance_family"`
	InstanceType     string  `json:"instance_type"`
	CurrentType      string  `json:"current_type"`
	Reasoning        string  `json:"reasoning"`
	CostImpact       string  `json:"cost_impact"`
	PerformanceImpact string `json:"performance_impact"`
	CPUMemoryRatio   float64 `json:"cpu_memory_ratio"`
	ConfidenceLevel  float64 `json:"confidence_level"`
}

// DecisionImpact shows how optimization changes the AWS vs local recommendation.
type DecisionImpact struct {
	OriginalRecommendation string  `json:"original_recommendation"`
	OptimizedRecommendation string `json:"optimized_recommendation"`
	DecisionChanged        bool    `json:"decision_changed"`
	ImpactDescription      string  `json:"impact_description"`
	CostDifferenceChange   float64 `json:"cost_difference_change"`
	TimeDifferenceChange   time.Duration `json:"time_difference_change"`
}

// AnalyzeWithHistory performs analysis enhanced with historical job data.
func (h *HistoryAwareAnalyzer) AnalyzeWithHistory(
	localPartition, awsPartition *types.PartitionAnalysis,
	job *types.JobRequest,
	scriptPath string,
) (*EnhancedAnalysis, error) {

	// 1. Run current analysis (baseline)
	currentRecommendation := h.baseAnalyzer.Compare(localPartition, awsPartition, job)
	currentAnalysis := &types.Analysis{
		TargetPartition: localPartition,
		BurstPartition:  awsPartition,
		Recommendation:  currentRecommendation,
		Timestamp:       time.Now(),
		JobRequest:      job,
	}

	enhanced := &EnhancedAnalysis{
		Current: currentAnalysis,
	}

	// 2. Look for historical insights if database is available
	if h.historyDB != nil {
		if err := h.addHistoricalInsights(enhanced, job, scriptPath); err != nil {
			// Log warning but continue with basic analysis
			fmt.Printf("Warning: failed to load historical insights: %v\n", err)
		}
	}

	return enhanced, nil
}

// addHistoricalInsights enriches the analysis with historical job data.
func (h *HistoryAwareAnalyzer) addHistoricalInsights(analysis *EnhancedAnalysis, job *types.JobRequest, scriptPath string) error {
	// Get script hash for similarity detection
	scriptHash := ""
	if scriptPath != "" {
		if hash, err := h.getScriptHash(scriptPath); err == nil {
			scriptHash = hash
		}
	}

	// Find similar jobs in user's history
	similarJobs, err := h.historyDB.FindSimilarJobs(scriptHash, *job)
	if err != nil {
		return fmt.Errorf("failed to find similar jobs: %w", err)
	}

	if len(similarJobs) == 0 {
		analysis.HistoryInsights = &HistoryInsights{
			SimilarJobsFound: 0,
			Confidence:       0.0,
		}
		return nil
	}

	// Generate insights from historical data
	insights := h.generateHistoryInsights(similarJobs)
	analysis.HistoryInsights = insights

	// Generate resource optimization suggestions
	optimizations := h.generateResourceOptimizations(job, similarJobs)
	analysis.ResourceOptimizations = optimizations

	// Generate AWS instance recommendations
	instanceRecs := h.generateInstanceRecommendations(job, similarJobs)
	analysis.InstanceRecommendations = instanceRecs

	// If we have optimizations, re-run analysis with optimized resources
	if len(optimizations) > 0 {
		optimizedJob := h.applyOptimizations(job, optimizations)
		optimizedRecommendation := h.baseAnalyzer.Compare(analysis.Current.TargetPartition, analysis.Current.BurstPartition, optimizedJob)

		analysis.Optimized = &types.Analysis{
			TargetPartition: analysis.Current.TargetPartition,
			BurstPartition:  analysis.Current.BurstPartition,
			Recommendation:  optimizedRecommendation,
			Timestamp:       time.Now(),
			JobRequest:      optimizedJob,
		}

		// Analyze decision impact
		analysis.DecisionImpact = h.analyzeDecisionImpact(analysis.Current.Recommendation, optimizedRecommendation)
	}

	return nil
}

// generateHistoryInsights creates insights from historical job execution data.
func (h *HistoryAwareAnalyzer) generateHistoryInsights(jobs []types.JobEfficiencyData) *HistoryInsights {
	if len(jobs) == 0 {
		return &HistoryInsights{SimilarJobsFound: 0}
	}

	// Calculate efficiency trends
	trends := h.calculateEfficiencyTrends(jobs)

	// Generate job pattern if we have enough data
	var pattern *types.JobPattern
	if len(jobs) >= 3 {
		pattern = h.generateJobPattern(jobs)
	}

	// Calculate confidence based on data consistency
	confidence := h.calculateInsightConfidence(jobs)

	return &HistoryInsights{
		SimilarJobsFound:   len(jobs),
		JobPattern:         pattern,
		EfficiencyTrends:   trends,
		PerformanceHistory: jobs,
		Confidence:         confidence,
	}
}

// calculateEfficiencyTrends analyzes trends in resource efficiency over time.
func (h *HistoryAwareAnalyzer) calculateEfficiencyTrends(jobs []types.JobEfficiencyData) *EfficiencyTrends {
	if len(jobs) == 0 {
		return &EfficiencyTrends{}
	}

	// Calculate averages
	var cpuSum, memSum, timeSum float64
	for _, job := range jobs {
		cpuSum += job.CPUEfficiency
		memSum += job.MemoryEfficiency
		timeSum += job.TimeEfficiency
	}

	count := float64(len(jobs))
	return &EfficiencyTrends{
		CPUTrend:            h.calculateTrend(jobs, "cpu"),
		MemoryTrend:         h.calculateTrend(jobs, "memory"),
		TimeTrend:           h.calculateTrend(jobs, "time"),
		CPUEfficiencyAvg:    cpuSum / count,
		MemoryEfficiencyAvg: memSum / count,
		TimeEfficiencyAvg:   timeSum / count,
	}
}

// calculateTrend determines if efficiency is improving, declining, or stable.
func (h *HistoryAwareAnalyzer) calculateTrend(jobs []types.JobEfficiencyData, metric string) string {
	if len(jobs) < 3 {
		return "insufficient-data"
	}

	// Compare first half vs second half of jobs (sorted by time)
	mid := len(jobs) / 2
	firstHalf := jobs[:mid]
	secondHalf := jobs[mid:]

	var firstAvg, secondAvg float64
	for _, job := range firstHalf {
		switch metric {
		case "cpu":
			firstAvg += job.CPUEfficiency
		case "memory":
			firstAvg += job.MemoryEfficiency
		case "time":
			firstAvg += job.TimeEfficiency
		}
	}
	firstAvg /= float64(len(firstHalf))

	for _, job := range secondHalf {
		switch metric {
		case "cpu":
			secondAvg += job.CPUEfficiency
		case "memory":
			secondAvg += job.MemoryEfficiency
		case "time":
			secondAvg += job.TimeEfficiency
		}
	}
	secondAvg /= float64(len(secondHalf))

	diff := secondAvg - firstAvg
	if diff > 5.0 {
		return "improving"
	} else if diff < -5.0 {
		return "declining"
	}
	return "stable"
}

// generateResourceOptimizations creates optimization suggestions based on historical patterns.
func (h *HistoryAwareAnalyzer) generateResourceOptimizations(job *types.JobRequest, history []types.JobEfficiencyData) []ResourceOptimization {
	if len(history) < 2 {
		return []ResourceOptimization{}
	}

	var optimizations []ResourceOptimization

	// Calculate average efficiencies
	var avgCPUEff, avgMemEff, avgTimeEff float64
	var avgMemUsageGB float64
	successfulJobs := 0

	for _, histJob := range history {
		if histJob.IsSuccessful() {
			avgCPUEff += histJob.CPUEfficiency
			avgMemEff += histJob.MemoryEfficiency
			avgTimeEff += histJob.TimeEfficiency
			avgMemUsageGB += float64(histJob.MaxMemoryUsedMB) / 1024.0
			successfulJobs++
		}
	}

	if successfulJobs == 0 {
		return optimizations
	}

	avgCPUEff /= float64(successfulJobs)
	avgMemEff /= float64(successfulJobs)
	avgTimeEff /= float64(successfulJobs)
	avgMemUsageGB /= float64(successfulJobs)

	// Memory optimization
	if avgMemEff < 70 { // Under 70% memory efficiency suggests over-allocation
		currentMemMB, _ := types.ParseMemoryString(job.Memory)
		suggestedMemGB := avgMemUsageGB * 1.25 // 25% buffer above typical usage
		suggestedMemMB := int64(suggestedMemGB * 1024)

		if suggestedMemMB < currentMemMB && suggestedMemMB > 0 {
			optimizations = append(optimizations, ResourceOptimization{
				ResourceType:   "memory",
				CurrentValue:   job.Memory,
				SuggestedValue: types.FormatMemoryMB(suggestedMemMB),
				Reasoning:      fmt.Sprintf("Your average usage: %.1fGB (%.0f%% efficiency)", avgMemUsageGB, avgMemEff),
				ConfidenceLevel: h.calculateOptimizationConfidence(avgMemEff, successfulJobs),
				RiskLevel:      h.assessOptimizationRisk(avgMemEff, successfulJobs),
			})
		}
	}

	// CPU optimization
	if avgCPUEff < 60 { // Under 60% CPU efficiency suggests over-allocation
		totalReqCPUs := job.Nodes * job.CPUsPerTask
		avgEffectiveCPUs := float64(totalReqCPUs) * avgCPUEff / 100.0
		suggestedCPUs := int(avgEffectiveCPUs * 1.2) // 20% buffer

		if suggestedCPUs < totalReqCPUs && suggestedCPUs > 0 {
			// Suggest reduction in CPUs per task (keep nodes same for now)
			newCPUsPerTask := suggestedCPUs / job.Nodes
			if newCPUsPerTask < job.CPUsPerTask && newCPUsPerTask > 0 {
				optimizations = append(optimizations, ResourceOptimization{
					ResourceType:   "cpu",
					CurrentValue:   fmt.Sprintf("%d", job.CPUsPerTask),
					SuggestedValue: fmt.Sprintf("%d", newCPUsPerTask),
					Reasoning:      fmt.Sprintf("Your average CPU efficiency: %.0f%% (%.1f effective cores)", avgCPUEff, avgEffectiveCPUs),
					ConfidenceLevel: h.calculateOptimizationConfidence(avgCPUEff, successfulJobs),
					RiskLevel:      h.assessOptimizationRisk(avgCPUEff, successfulJobs),
				})
			}
		}
	}

	// Time limit optimization
	if avgTimeEff < 60 { // Jobs finishing much earlier than time limit
		avgRuntimeHours := 0.0
		for _, histJob := range history {
			if histJob.IsSuccessful() {
				avgRuntimeHours += histJob.ActualTime.Hours()
			}
		}
		avgRuntimeHours /= float64(successfulJobs)

		suggestedTimeLimit := time.Duration(avgRuntimeHours*1.3) * time.Hour // 30% buffer
		if suggestedTimeLimit < job.TimeLimit {
			optimizations = append(optimizations, ResourceOptimization{
				ResourceType:   "time",
				CurrentValue:   job.TimeLimit.String(),
				SuggestedValue: suggestedTimeLimit.String(),
				Reasoning:      fmt.Sprintf("Your average runtime: %.1fh (%.0f%% of time limit)", avgRuntimeHours, avgTimeEff),
				ConfidenceLevel: h.calculateOptimizationConfidence(avgTimeEff, successfulJobs),
				RiskLevel:      "low", // Time limit reductions are generally low risk
			})
		}
	}

	return optimizations
}

// generateInstanceRecommendations suggests better AWS instance types based on workload patterns.
func (h *HistoryAwareAnalyzer) generateInstanceRecommendations(job *types.JobRequest, history []types.JobEfficiencyData) []InstanceRecommendation {
	if len(history) < 2 {
		return []InstanceRecommendation{}
	}

	// Analyze workload characteristics from history
	workloadAnalysis := h.analyzeWorkloadCharacteristics(history)

	// Generate instance family recommendations
	familyRec := h.recommendInstanceFamily(workloadAnalysis)

	if familyRec != nil {
		return []InstanceRecommendation{*familyRec}
	}

	return []InstanceRecommendation{}
}

// analyzeWorkloadCharacteristics determines workload type from historical patterns.
func (h *HistoryAwareAnalyzer) analyzeWorkloadCharacteristics(history []types.JobEfficiencyData) WorkloadCharacteristics {
	var cpuSum, memSum float64
	var actualRatioSum float64
	successfulJobs := 0

	for _, job := range history {
		if job.IsSuccessful() {
			cpuSum += job.CPUEfficiency
			memSum += job.MemoryEfficiency
			actualRatioSum += job.ActualCPUMemRatio
			successfulJobs++
		}
	}

	if successfulJobs == 0 {
		return WorkloadCharacteristics{}
	}

	return WorkloadCharacteristics{
		AvgCPUEfficiency:    cpuSum / float64(successfulJobs),
		AvgMemoryEfficiency: memSum / float64(successfulJobs),
		AvgCPUMemoryRatio:   actualRatioSum / float64(successfulJobs),
		SampleSize:          successfulJobs,
	}
}

// WorkloadCharacteristics represents analyzed workload patterns.
type WorkloadCharacteristics struct {
	AvgCPUEfficiency    float64
	AvgMemoryEfficiency float64
	AvgCPUMemoryRatio   float64 // GB per effective CPU
	SampleSize          int
}

// recommendInstanceFamily suggests optimal AWS instance family based on workload characteristics.
func (h *HistoryAwareAnalyzer) recommendInstanceFamily(chars WorkloadCharacteristics) *InstanceRecommendation {
	if chars.SampleSize < 2 {
		return nil
	}

	// Determine workload type and recommend instance family
	switch {
	case chars.AvgCPUEfficiency > 75 && chars.AvgCPUMemoryRatio < 4.0:
		// CPU-intensive workload
		return &InstanceRecommendation{
			InstanceFamily:    "c5",
			Reasoning:         fmt.Sprintf("CPU-bound workload (%.0f%% CPU eff, %.1fGB per effective core)", chars.AvgCPUEfficiency, chars.AvgCPUMemoryRatio),
			CostImpact:        "15-25% lower cost per vCPU compared to general-purpose instances",
			PerformanceImpact: "Higher clock speeds, optimized for CPU-intensive workloads",
			CPUMemoryRatio:    2.0,
			ConfidenceLevel:   h.calculateRecommendationConfidence(chars),
		}

	case chars.AvgMemoryEfficiency > 75 && chars.AvgCPUMemoryRatio > 6.0:
		// Memory-intensive workload
		return &InstanceRecommendation{
			InstanceFamily:    "r5",
			Reasoning:         fmt.Sprintf("Memory-bound workload (%.0f%% memory eff, %.1fGB per effective core)", chars.AvgMemoryEfficiency, chars.AvgCPUMemoryRatio),
			CostImpact:        "20-30% lower cost per GB memory compared to general-purpose instances",
			PerformanceImpact: "Higher memory bandwidth, optimized for memory-intensive workloads",
			CPUMemoryRatio:    8.0,
			ConfidenceLevel:   h.calculateRecommendationConfidence(chars),
		}

	case chars.AvgCPUEfficiency > 60 && chars.AvgMemoryEfficiency > 60:
		// Balanced workload
		return &InstanceRecommendation{
			InstanceFamily:    "m5",
			Reasoning:         fmt.Sprintf("Balanced workload (%.0f%% CPU eff, %.0f%% memory eff)", chars.AvgCPUEfficiency, chars.AvgMemoryEfficiency),
			CostImpact:        "Good general-purpose price/performance ratio",
			PerformanceImpact: "Balanced CPU and memory performance",
			CPUMemoryRatio:    4.0,
			ConfidenceLevel:   h.calculateRecommendationConfidence(chars),
		}

	default:
		// Over-allocated or variable workload
		return &InstanceRecommendation{
			InstanceFamily:    "m5",
			Reasoning:         fmt.Sprintf("Variable resource usage (%.0f%% CPU eff, %.0f%% memory eff)", chars.AvgCPUEfficiency, chars.AvgMemoryEfficiency),
			CostImpact:        "Consider right-sizing resources before choosing instance type",
			PerformanceImpact: "General-purpose instances until usage patterns stabilize",
			CPUMemoryRatio:    4.0,
			ConfidenceLevel:   0.3, // Low confidence for inconsistent patterns
		}
	}
}

// applyOptimizations creates an optimized job request based on suggestions.
func (h *HistoryAwareAnalyzer) applyOptimizations(original *types.JobRequest, optimizations []ResourceOptimization) *types.JobRequest {
	optimized := *original // Copy

	for _, opt := range optimizations {
		switch opt.ResourceType {
		case "memory":
			optimized.Memory = opt.SuggestedValue
		case "cpu":
			if newCPUs, err := fmt.Sscanf(opt.SuggestedValue, "%d", &optimized.CPUsPerTask); err == nil && newCPUs == 1 {
				// CPUs per task was updated
			}
		case "time":
			if newTime, err := time.ParseDuration(opt.SuggestedValue); err == nil {
				optimized.TimeLimit = newTime
			}
		}
	}

	return &optimized
}

// analyzeDecisionImpact compares original vs optimized recommendations.
func (h *HistoryAwareAnalyzer) analyzeDecisionImpact(original, optimized *types.Recommendation) *DecisionImpact {
	decisionChanged := original.Preferred != optimized.Preferred

	impact := &DecisionImpact{
		OriginalRecommendation:  string(original.Preferred),
		OptimizedRecommendation: string(optimized.Preferred),
		DecisionChanged:         decisionChanged,
		CostDifferenceChange:    optimized.CostDifference - original.CostDifference,
		TimeDifferenceChange:    optimized.TimeSavings - original.TimeSavings,
	}

	if decisionChanged {
		impact.ImpactDescription = fmt.Sprintf("Optimization changed recommendation from %s to %s",
			original.Preferred, optimized.Preferred)
	} else {
		impact.ImpactDescription = fmt.Sprintf("Optimization reinforces %s recommendation with better cost/time ratio",
			original.Preferred)
	}

	return impact
}

// Helper functions for confidence and risk assessment
func (h *HistoryAwareAnalyzer) calculateOptimizationConfidence(efficiency float64, sampleSize int) float64 {
	// Higher confidence for consistent low efficiency with more samples
	baseConfidence := 0.5
	if efficiency < 50 {
		baseConfidence = 0.8
	} else if efficiency < 70 {
		baseConfidence = 0.6
	}

	// Adjust for sample size
	sampleConfidence := float64(sampleSize) / 10.0 // Max confidence at 10+ samples
	if sampleConfidence > 1.0 {
		sampleConfidence = 1.0
	}

	return baseConfidence * sampleConfidence
}

func (h *HistoryAwareAnalyzer) assessOptimizationRisk(efficiency float64, sampleSize int) string {
	if sampleSize < 3 {
		return "medium" // Not enough data
	}
	if efficiency < 50 {
		return "low" // Very consistent over-allocation
	}
	if efficiency < 70 {
		return "low" // Moderate over-allocation
	}
	return "medium" // Less clear optimization opportunity
}

func (h *HistoryAwareAnalyzer) calculateInsightConfidence(jobs []types.JobEfficiencyData) float64 {
	if len(jobs) == 0 {
		return 0.0
	}

	// Base confidence on sample size and consistency
	sampleConfidence := float64(len(jobs)) / 10.0
	if sampleConfidence > 1.0 {
		sampleConfidence = 1.0
	}

	// Calculate variance in efficiency to assess consistency
	var cpuVar, memVar float64
	var cpuSum, memSum float64
	for _, job := range jobs {
		cpuSum += job.CPUEfficiency
		memSum += job.MemoryEfficiency
	}
	cpuAvg := cpuSum / float64(len(jobs))
	memAvg := memSum / float64(len(jobs))

	for _, job := range jobs {
		cpuVar += (job.CPUEfficiency - cpuAvg) * (job.CPUEfficiency - cpuAvg)
		memVar += (job.MemoryEfficiency - memAvg) * (job.MemoryEfficiency - memAvg)
	}
	cpuVar /= float64(len(jobs))
	memVar /= float64(len(jobs))

	// Lower variance = higher confidence
	consistencyFactor := 1.0 / (1.0 + (cpuVar+memVar)/1000.0)

	return sampleConfidence * consistencyFactor
}

func (h *HistoryAwareAnalyzer) calculateRecommendationConfidence(chars WorkloadCharacteristics) float64 {
	// Higher confidence for clear workload patterns with good sample size
	sampleFactor := float64(chars.SampleSize) / 5.0
	if sampleFactor > 1.0 {
		sampleFactor = 1.0
	}

	// Clear CPU or memory dominance increases confidence
	efficiencyDiff := chars.AvgCPUEfficiency - chars.AvgMemoryEfficiency
	if efficiencyDiff < 0 {
		efficiencyDiff = -efficiencyDiff
	}
	clarityFactor := efficiencyDiff / 50.0 // Max clarity when 50% difference
	if clarityFactor > 1.0 {
		clarityFactor = 1.0
	}

	return sampleFactor * (0.5 + 0.5*clarityFactor)
}

func (h *HistoryAwareAnalyzer) generateJobPattern(jobs []types.JobEfficiencyData) *types.JobPattern {
	if len(jobs) == 0 {
		return nil
	}

	pattern := &types.JobPattern{
		ScriptHash: jobs[0].ScriptHash,
		ScriptName: filepath.Base(jobs[0].ScriptPath),
		RunCount:   len(jobs),
	}

	// Calculate averages
	var cpuSum, memSum, timeSum float64
	var memUsageSum, reqRatioSum, actualRatioSum float64
	var runtimeSum float64
	successCount := 0

	for _, job := range jobs {
		cpuSum += job.CPUEfficiency
		memSum += job.MemoryEfficiency
		timeSum += job.TimeEfficiency
		memUsageSum += float64(job.MaxMemoryUsedMB) / 1024.0
		reqRatioSum += job.RequestedCPUMemRatio
		actualRatioSum += job.ActualCPUMemRatio
		runtimeSum += job.ActualTime.Hours()

		if job.IsSuccessful() {
			successCount++
		}

		if job.SubmissionTime.After(pattern.LastRun) {
			pattern.LastRun = job.SubmissionTime
		}
	}

	count := float64(len(jobs))
	pattern.AvgCPUEfficiency = cpuSum / count
	pattern.AvgMemoryEfficiency = memSum / count
	pattern.TypicalMemoryUsageGB = memUsageSum / count
	pattern.AvgRequestedRatio = reqRatioSum / count
	pattern.AvgActualRatio = actualRatioSum / count
	pattern.AvgRuntime = time.Duration(runtimeSum/count) * time.Hour
	pattern.SuccessRate = float64(successCount) / count

	// Classify workload type based on efficiency patterns
	if pattern.AvgCPUEfficiency > 75 && pattern.AvgMemoryEfficiency < 60 {
		pattern.WorkloadType = "cpu-bound"
	} else if pattern.AvgMemoryEfficiency > 75 && pattern.AvgCPUEfficiency < 60 {
		pattern.WorkloadType = "memory-bound"
	} else if pattern.AvgCPUEfficiency > 65 && pattern.AvgMemoryEfficiency > 65 {
		pattern.WorkloadType = "balanced"
	} else {
		pattern.WorkloadType = "over-allocated"
	}

	return pattern
}

func (h *HistoryAwareAnalyzer) getScriptHash(scriptPath string) (string, error) {
	// This would ideally use the SLURM client's GetJobScriptHash method
	// For now, return empty string to indicate no hash available
	return "", nil
}