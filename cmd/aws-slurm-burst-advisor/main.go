package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/analyzer"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/aws"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/config"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/history"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/slurm"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

var (
	cfgFile         string
	batchFile       string
	targetPartition string
	burstPartition  string
	nodes           int
	cpusPerTask     int
	timeLimit       string
	memory          string
	gres            string
	verbose         bool

	// Phase 1: History tracking flags
	trackHistory    bool
	withHistory     bool
	optimize        bool
	recommendInstance bool
	historyDays     int

	// Phase 3B: Budget integration flags
	account         string
	checkBudget     bool
	asbbEndpoint    string
	budgetAware     bool
)

var rootCmd = &cobra.Command{
	Use:   "aws-slurm-burst-advisor",
	Short: "Analyze whether to submit jobs to local cluster or burst to AWS",
	Long: `AWS SLURM Burst Advisor - A tool to help HPC users decide whether to submit
their jobs to the local cluster or burst to AWS EC2 based on queue conditions, costs,
and time-to-completion analysis. Built for SLURM clusters with AWS EC2 bursting capability.

Examples:
  # Analyze using batch script
  aws-slurm-burst-advisor --batch-file=job.sbatch --burst-partition=gpu-aws

  # Manual job specification
  aws-slurm-burst-advisor --target-partition=cpu --burst-partition=cpu-aws \
                          --nodes=4 --cpus-per-task=8 --time=2:00:00

  # Quick positional syntax
  aws-slurm-burst-advisor job.sbatch gpu-aws`,
	Run: runAnalysis,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Configuration
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.aws-slurm-burst-advisor.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Input methods
	rootCmd.Flags().StringVarP(&batchFile, "batch-file", "f", "", "SLURM batch script to analyze")
	rootCmd.Flags().StringVarP(&targetPartition, "target-partition", "t", "", "target (local) partition")
	rootCmd.Flags().StringVarP(&burstPartition, "burst-partition", "b", "", "burst (AWS) partition")

	// Manual job specification
	rootCmd.Flags().IntVarP(&nodes, "nodes", "N", 0, "number of nodes")
	rootCmd.Flags().IntVarP(&cpusPerTask, "cpus-per-task", "c", 1, "CPUs per task")
	rootCmd.Flags().StringVar(&timeLimit, "time", "", "time limit (e.g., 2:00:00)")
	rootCmd.Flags().StringVar(&memory, "mem", "", "memory per node")
	rootCmd.Flags().StringVar(&gres, "gres", "", "generic resources (e.g., gpu:2)")

	// Phase 1: History tracking flags
	rootCmd.Flags().BoolVar(&trackHistory, "track-history", false, "enable job history collection")
	rootCmd.Flags().BoolVar(&withHistory, "with-history", false, "show historical insights")
	rootCmd.Flags().BoolVar(&optimize, "optimize", false, "suggest resource optimizations based on history")
	rootCmd.Flags().BoolVar(&recommendInstance, "recommend-instance", false, "suggest better AWS instance types")
	rootCmd.Flags().IntVar(&historyDays, "history-days", 90, "days of history to analyze (1-365)")

	// Phase 3B: Budget integration flags
	rootCmd.Flags().StringVarP(&account, "account", "A", "", "budget account for grant management (e.g., NSF-ABC123)")
	rootCmd.Flags().BoolVar(&checkBudget, "check-budget", false, "check budget availability before AWS recommendations")
	rootCmd.Flags().StringVar(&asbbEndpoint, "asbb-endpoint", "http://localhost:8080", "ASBB service endpoint")
	rootCmd.Flags().BoolVar(&budgetAware, "budget-aware", false, "enable comprehensive budget-aware analysis")

	// Mark required flags
	rootCmd.MarkFlagRequired("burst-partition")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Error getting home directory: %v", err)
		}

		// Search for config in multiple locations
		viper.AddConfigPath(home)
		viper.AddConfigPath("/etc/aws-slurm-burst-advisor/")
		viper.AddConfigPath(".")
		viper.SetConfigName(".aws-slurm-burst-advisor")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if verbose {
			fmt.Printf("Config file not found, using defaults: %v\n", err)
		}
	} else if verbose {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func runAnalysis(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Handle positional arguments: slurm-burst-advisor job.sbatch burst-partition
	if len(args) >= 1 && batchFile == "" {
		batchFile = args[0]
	}
	if len(args) >= 2 && burstPartition == "" {
		burstPartition = args[1]
	}

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Parse job requirements
	var jobReq *types.JobRequest
	if batchFile != "" {
		batch, err := slurm.ParseBatchScript(batchFile)
		if err != nil {
			log.Fatalf("Failed to parse batch script %s: %v", batchFile, err)
		}

		jobReq = batchScriptToJobRequest(batch)

		// Use partition from script if target not specified
		if targetPartition == "" {
			targetPartition = batch.Partition
		}

		fmt.Printf("Analyzing job from %s:\n", filepath.Base(batchFile))
		displayJobSummary(batch)
	} else {
		// Manual specification
		if targetPartition == "" || nodes == 0 {
			log.Fatal("When not using --batch-file, --target-partition and --nodes are required")
		}

		duration, err := parseSlurmTime(timeLimit)
		if err != nil {
			log.Fatalf("Invalid time format: %v", err)
		}

		jobReq = &types.JobRequest{
			Nodes:       nodes,
			CPUsPerTask: cpusPerTask,
			TimeLimit:   duration,
			Memory:      memory,
		}

		if gres != "" {
			jobReq.TRES = parseGRES(gres)
		}
	}

	if targetPartition == "" {
		log.Fatal("Target partition must be specified")
	}

	// Perform analysis
	analysis, err := performAnalysis(ctx, cfg, jobReq, targetPartition, burstPartition)
	if err != nil {
		log.Fatalf("Analysis failed: %v", err)
	}

	// Display results
	displayResults(analysis)
}

func performAnalysis(ctx context.Context, cfg *config.Config, jobReq *types.JobRequest, target, burst string) (*types.Analysis, error) {
	// Initialize clients
	slurmClient := slurm.NewClient(cfg.SlurmBinPath)
	awsClient, err := aws.NewPricingClient(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// Initialize analyzer (with or without history)
	var baseAnalyzer *analyzer.DecisionEngine
	var historyAnalyzer *analyzer.HistoryAwareAnalyzer

	if withHistory || optimize || recommendInstance {
		// Initialize history database
		currentUser, err := slurm.GetCurrentUser()
		if err != nil {
			fmt.Printf("Warning: failed to get current user for history: %v\n", err)
			baseAnalyzer = analyzer.NewDecisionEngine(cfg.Weights)
		} else {
			historyDB, err := history.NewJobHistoryDB(currentUser)
			if err != nil {
				fmt.Printf("Warning: failed to initialize job history: %v\n", err)
				baseAnalyzer = analyzer.NewDecisionEngine(cfg.Weights)
			} else {
				defer historyDB.Close()
				historyAnalyzer = analyzer.NewHistoryAwareAnalyzer(cfg.Weights, historyDB)

				// Collect recent job history if tracking is enabled
				if trackHistory {
					if err := collectJobHistory(ctx, slurmClient, historyDB, currentUser, historyDays); err != nil {
						fmt.Printf("Warning: failed to collect job history: %v\n", err)
					}
				}
			}
		}
	} else {
		baseAnalyzer = analyzer.NewDecisionEngine(cfg.Weights)
	}

	// Analyze both partitions concurrently
	var wg sync.WaitGroup
	results := make(chan *types.PartitionAnalysis, 2)
	errors := make(chan error, 2)

	// Analyze target (local) partition
	wg.Add(1)
	go func() {
		defer wg.Done()
		analysis, err := analyzeLocalPartition(ctx, slurmClient, cfg, target, jobReq)
		if err != nil {
			errors <- fmt.Errorf("target partition analysis failed: %w", err)
			return
		}
		results <- analysis
	}()

	// Analyze burst (AWS) partition
	wg.Add(1)
	go func() {
		defer wg.Done()
		analysis, err := analyzeBurstPartition(ctx, slurmClient, awsClient, cfg, burst, jobReq)
		if err != nil {
			errors <- fmt.Errorf("burst partition analysis failed: %w", err)
			return
		}
		results <- analysis
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results or errors
	var targetAnalysis, burstAnalysis *types.PartitionAnalysis
	for i := 0; i < 2; i++ {
		select {
		case err := <-errors:
			if err != nil {
				return nil, err
			}
		case result := <-results:
			if result.Type == "local" {
				targetAnalysis = result
			} else {
				burstAnalysis = result
			}
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("analysis timeout")
		}
	}

	// Generate recommendation using appropriate analyzer
	var analysis *types.Analysis

	if historyAnalyzer != nil && (withHistory || optimize || recommendInstance) {
		// Use history-aware analysis
		enhanced, err := historyAnalyzer.AnalyzeWithHistory(targetAnalysis, burstAnalysis, jobReq, batchFile)
		if err != nil {
			// Fall back to basic analysis
			fmt.Printf("Warning: history analysis failed, using basic analysis: %v\n", err)
			recommendation := baseAnalyzer.Compare(targetAnalysis, burstAnalysis, jobReq)
			analysis = &types.Analysis{
				TargetPartition: targetAnalysis,
				BurstPartition:  burstAnalysis,
				Recommendation:  recommendation,
				Timestamp:       time.Now(),
				JobRequest:      jobReq,
			}
		} else {
			// Use enhanced analysis result
			if enhanced.Optimized != nil && optimize {
				analysis = enhanced.Optimized
				// Store enhanced data for display
				analysis.Metadata.DataSources = append(analysis.Metadata.DataSources, "job_history")
			} else {
				analysis = enhanced.Current
			}

			// Display historical insights if requested
			if withHistory && enhanced.HistoryInsights != nil {
				displayHistoricalInsights(enhanced.HistoryInsights)
			}

			// Display optimizations if requested
			if optimize && len(enhanced.ResourceOptimizations) > 0 {
				displayResourceOptimizations(enhanced.ResourceOptimizations)
			}

			// Display instance recommendations if requested
			if recommendInstance && len(enhanced.InstanceRecommendations) > 0 {
				displayInstanceRecommendations(enhanced.InstanceRecommendations)
			}

			// Display decision impact if optimization changed the recommendation
			if enhanced.DecisionImpact != nil && enhanced.DecisionImpact.DecisionChanged {
				displayDecisionImpact(enhanced.DecisionImpact)
			}
		}
	} else {
		// Use basic analysis
		if baseAnalyzer == nil {
			baseAnalyzer = analyzer.NewDecisionEngine(cfg.Weights)
		}
		recommendation := baseAnalyzer.Compare(targetAnalysis, burstAnalysis, jobReq)
		analysis = &types.Analysis{
			TargetPartition: targetAnalysis,
			BurstPartition:  burstAnalysis,
			Recommendation:  recommendation,
			Timestamp:       time.Now(),
			JobRequest:      jobReq,
		}
	}

	return analysis, nil
}

func analyzeLocalPartition(ctx context.Context, client *slurm.Client, cfg *config.Config, partition string, job *types.JobRequest) (*types.PartitionAnalysis, error) {
	// Get partition info and queue state
	partitionInfo, err := client.GetPartitionInfo(ctx, partition)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition info: %w", err)
	}

	queueInfo, err := client.GetQueueInfo(ctx, partition)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue info: %w", err)
	}

	// Calculate costs using local cost model
	costCalc := analyzer.NewLocalCostCalculator(cfg.LocalCosts)
	cost := costCalc.Calculate(partition, job, partitionInfo)

	return &types.PartitionAnalysis{
		Name:              partition,
		Type:              "local",
		QueueDepth:        queueInfo.JobsPending,
		EstimatedWaitTime: queueInfo.EstimatedWaitTime,
		StartupTime:       0, // Local resources start immediately
		AvailableNodes:    partitionInfo.IdleNodes,
		TotalNodes:        partitionInfo.TotalNodes,
		EstimatedCost:     cost,
		ResourcesPerNode:  make(map[string]string), // Convert NodeResources to map[string]string
	}, nil
}

func analyzeBurstPartition(ctx context.Context, slurmClient *slurm.Client, awsClient *aws.PricingClient, cfg *config.Config, partition string, job *types.JobRequest) (*types.PartitionAnalysis, error) {
	// Get AWS partition configuration
	partitionConfig, err := cfg.GetAWSPartitionConfig(partition)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS partition config: %w", err)
	}

	// Get current pricing
	pricing, err := awsClient.GetInstancePricing(ctx, partitionConfig.InstanceType, partitionConfig.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS pricing: %w", err)
	}

	// Calculate AWS costs
	costCalc := analyzer.NewAWSCostCalculator()
	cost := costCalc.Calculate(job, partitionConfig, pricing)

	return &types.PartitionAnalysis{
		Name:              partition,
		Type:              "aws",
		QueueDepth:        0, // AWS has unlimited capacity
		EstimatedWaitTime: 0,
		StartupTime:       3 * time.Minute, // Typical AWS startup time
		AvailableNodes:    1000,            // Effectively unlimited
		TotalNodes:        1000,
		EstimatedCost:     cost,
		InstanceType:      partitionConfig.InstanceType,
		CurrentPrice:      pricing.SpotPrice,
	}, nil
}

func batchScriptToJobRequest(batch *types.BatchScript) *types.JobRequest {
	return &types.JobRequest{
		JobName:     batch.JobName,
		Nodes:       batch.Nodes,
		CPUsPerTask: batch.CPUsPerTask,
		TimeLimit:   batch.TimeLimit,
		Memory:      batch.Memory,
		TRES:        batch.GRES,
		Account:     batch.Account,
		QOS:         batch.QOS,
		Features:    batch.Features,
		Constraints: batch.Constraints,
	}
}

func displayJobSummary(batch *types.BatchScript) {
	fmt.Printf("  Job: %s\n", batch.JobName)
	fmt.Printf("  Resources: %d nodes, %d CPUs/task", batch.Nodes, batch.CPUsPerTask)
	if batch.TimeLimit > 0 {
		fmt.Printf(", %v", batch.TimeLimit)
	}
	fmt.Println()

	if len(batch.GRES) > 0 {
		fmt.Printf("  GRES: %v\n", batch.GRES)
	}
	if batch.Memory != "" {
		fmt.Printf("  Memory: %s\n", batch.Memory)
	}
	fmt.Printf("  Original partition: %s\n\n", batch.Partition)
}

func displayResults(analysis *types.Analysis) {
	fmt.Println("ANALYSIS RESULTS")
	fmt.Println("================")
	fmt.Println()

	// Display target partition analysis
	displayPartitionAnalysis("TARGET", analysis.TargetPartition)
	fmt.Println()

	// Display burst partition analysis
	displayPartitionAnalysis("BURST", analysis.BurstPartition)
	fmt.Println()

	// Display recommendation
	displayRecommendation(analysis.Recommendation)
}

func displayPartitionAnalysis(label string, analysis *types.PartitionAnalysis) {
	fmt.Printf("%s (%s", label, analysis.Name)
	if analysis.Type == "local" {
		fmt.Printf(" - Local Cluster")
	} else {
		fmt.Printf(" - AWS")
		if analysis.InstanceType != "" {
			fmt.Printf(", %s", analysis.InstanceType)
		}
	}
	fmt.Printf("):\n")

	if analysis.QueueDepth > 0 {
		fmt.Printf("  Queue depth: %d jobs ahead\n", analysis.QueueDepth)
		fmt.Printf("  Est. wait time: %v\n", analysis.EstimatedWaitTime)
	} else {
		fmt.Printf("  Queue depth: None (immediate start)\n")
		if analysis.StartupTime > 0 {
			fmt.Printf("  Startup time: %v\n", analysis.StartupTime)
		}
	}

	if analysis.Type == "local" {
		fmt.Printf("  Available capacity: %d/%d nodes idle\n",
			analysis.AvailableNodes, analysis.TotalNodes)
	}

	cost := analysis.EstimatedCost
	fmt.Printf("  Cost breakdown:\n")
	if cost.ComputeCost > 0 {
		fmt.Printf("    Compute cost:     $%.2f\n", cost.ComputeCost)
	}
	if cost.NodeCost > 0 {
		fmt.Printf("    Node cost:        $%.2f\n", cost.NodeCost)
	}
	if cost.OverheadCost > 0 {
		fmt.Printf("    Overhead cost:    $%.2f\n", cost.OverheadCost)
	}
	if cost.DataTransferCost > 0 {
		fmt.Printf("    Data transfer:    $%.2f\n", cost.DataTransferCost)
	}
	fmt.Printf("    Total cost:       $%.2f\n", cost.TotalCost)

	if analysis.CurrentPrice > 0 {
		fmt.Printf("  Current spot price: $%.3f/hour\n", analysis.CurrentPrice)
	}
}

func displayRecommendation(rec *types.Recommendation) {
	fmt.Printf("🎯 ASBA RECOMMENDATION\n")
	fmt.Printf("=====================\n")

	// Clear recommendation with advisory language
	fmt.Printf("Advisory: ")
	if rec.Preferred == types.RecommendationLocal {
		fmt.Printf("💻 Use Local Cluster (Recommended)\n")
	} else {
		fmt.Printf("☁️  Burst to AWS (Recommended)\n")
	}

	fmt.Printf("├─ Time difference: ")
	if rec.TimeSavings > 0 {
		fmt.Printf("+%v (AWS saves time)\n", rec.TimeSavings)
	} else if rec.TimeSavings < 0 {
		fmt.Printf("%v (local is faster)\n", -rec.TimeSavings)
	} else {
		fmt.Printf("~0 (similar timing)\n")
	}

	fmt.Printf("├─ Cost difference: ")
	if rec.CostDifference > 0 {
		fmt.Printf("+$%.2f (AWS costs more)\n", rec.CostDifference)
	} else if rec.CostDifference < 0 {
		fmt.Printf("-$%.2f (AWS costs less)\n", -rec.CostDifference)
	} else {
		fmt.Printf("~$0 (similar cost)\n")
	}

	if rec.BreakevenTime > 0 {
		fmt.Printf("├─ Break-even point: %v (if wait time exceeds this, AWS is worth it)\n",
			rec.BreakevenTime)
	} else {
		fmt.Printf("├─ Break-even point: N/A\n")
	}

	fmt.Printf("└─ Confidence: %.0f%% (based on analysis)\n", rec.Confidence*100)

	if len(rec.Reasoning) > 0 {
		fmt.Printf("\nReasoning:\n")
		for _, reason := range rec.Reasoning {
			fmt.Printf("• %s\n", reason)
		}
	}

	// Clear advisory conclusion with user choice
	fmt.Printf("\n📋 SUGGESTED COMMANDS\n")
	fmt.Printf("====================\n")
	if rec.Preferred == types.RecommendationLocal {
		fmt.Printf("Recommended: sbatch %s\n", getBatchFileName())
		fmt.Printf("Alternative: asba burst %s %s [node-list]  # Override to use AWS\n",
			getBatchFileName(), getBurstPartition())
	} else {
		fmt.Printf("Recommended: asba burst %s %s [node-list]\n",
			getBatchFileName(), getBurstPartition())
		fmt.Printf("Alternative: sbatch %s  # Override to use local cluster\n", getBatchFileName())
	}

	fmt.Printf("\n💡 This is advisory guidance - you make the final decision!\n")
}

func parseSlurmTime(timeStr string) (time.Duration, error) {
	if timeStr == "" {
		return 0, nil
	}

	// Handle common SLURM time formats
	// "HH:MM:SS"
	if strings.Count(timeStr, ":") == 2 {
		parts := strings.Split(timeStr, ":")
		if len(parts) == 3 {
			hours, _ := strconv.Atoi(parts[0])
			minutes, _ := strconv.Atoi(parts[1])
			seconds, _ := strconv.Atoi(parts[2])
			return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second, nil
		}
	}

	// Try Go duration format as fallback
	return time.ParseDuration(timeStr)
}

func parseGRES(gresStr string) map[string]int {
	gres := make(map[string]int)

	// Parse "gpu:2" format
	if strings.Contains(gresStr, ":") {
		parts := strings.Split(gresStr, ":")
		if len(parts) >= 2 {
			resource := parts[0]
			if count, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				gres[resource] = count
			}
		}
	}

	return gres
}

// collectJobHistory collects recent job history for the user
func collectJobHistory(ctx context.Context, client *slurm.Client, historyDB *history.JobHistoryDB, user string, days int) error {
	// Get detailed job efficiency data from SLURM
	jobs, err := client.GetUserJobEfficiency(ctx, user, days)
	if err != nil {
		return fmt.Errorf("failed to get user job efficiency: %w", err)
	}

	// Store jobs in history database
	stored := 0
	for _, job := range jobs {
		// Calculate script hash if script path is available
		if job.ScriptPath != "" {
			if hash, err := client.GetJobScriptHash(job.ScriptPath); err == nil {
				job.ScriptHash = hash
			}
		}

		if err := historyDB.StoreJobExecution(job); err != nil {
			fmt.Printf("Warning: failed to store job %s: %v\n", job.JobID, err)
			continue
		}
		stored++
	}

	if verbose {
		fmt.Printf("Collected %d job records (%d stored) from last %d days\n", len(jobs), stored, days)
	}

	return nil
}

// displayHistoricalInsights shows insights from job history
func displayHistoricalInsights(insights *analyzer.HistoryInsights) {
	fmt.Println("\nHISTORICAL INSIGHTS")
	fmt.Println("===================")

	if insights.SimilarJobsFound == 0 {
		fmt.Println("No similar jobs found in your history")
		return
	}

	fmt.Printf("Similar jobs found: %d\n", insights.SimilarJobsFound)
	fmt.Printf("Confidence: %.0f%%\n", insights.Confidence*100)

	if insights.EfficiencyTrends != nil {
		fmt.Printf("\nEfficiency patterns:\n")
		fmt.Printf("  CPU efficiency: %.1f%% average (%s trend)\n",
			insights.EfficiencyTrends.CPUEfficiencyAvg, insights.EfficiencyTrends.CPUTrend)
		fmt.Printf("  Memory efficiency: %.1f%% average (%s trend)\n",
			insights.EfficiencyTrends.MemoryEfficiencyAvg, insights.EfficiencyTrends.MemoryTrend)
		fmt.Printf("  Time efficiency: %.1f%% average (%s trend)\n",
			insights.EfficiencyTrends.TimeEfficiencyAvg, insights.EfficiencyTrends.TimeTrend)
	}

	if insights.JobPattern != nil {
		fmt.Printf("\nJob pattern detected:\n")
		fmt.Printf("  Workload type: %s\n", insights.JobPattern.WorkloadType)
		fmt.Printf("  Typical effective CPUs: %.1f\n", insights.JobPattern.TypicalEffectiveCPUs)
		fmt.Printf("  Typical memory usage: %.1fGB\n", insights.JobPattern.TypicalMemoryUsageGB)
		fmt.Printf("  Success rate: %.0f%%\n", insights.JobPattern.SuccessRate*100)
	}
}

// displayResourceOptimizations shows resource optimization suggestions
func displayResourceOptimizations(optimizations []analyzer.ResourceOptimization) {
	fmt.Println("\nRESOURCE OPTIMIZATIONS")
	fmt.Println("======================")

	for _, opt := range optimizations {
		fmt.Printf("\n%s OPTIMIZATION:\n", strings.ToUpper(opt.ResourceType))
		fmt.Printf("  Current: %s\n", opt.CurrentValue)
		fmt.Printf("  Suggested: %s\n", opt.SuggestedValue)
		fmt.Printf("  Reasoning: %s\n", opt.Reasoning)
		fmt.Printf("  Confidence: %.0f%% (%s risk)\n", opt.ConfidenceLevel*100, opt.RiskLevel)

		if opt.LocalSavings > 0 {
			fmt.Printf("  Local savings: $%.2f per run\n", opt.LocalSavings)
		}
		if opt.AWSSavings > 0 {
			fmt.Printf("  AWS savings: $%.2f per run\n", opt.AWSSavings)
		}
	}
}

// displayInstanceRecommendations shows AWS instance type recommendations
func displayInstanceRecommendations(recommendations []analyzer.InstanceRecommendation) {
	fmt.Println("\nAWS INSTANCE RECOMMENDATIONS")
	fmt.Println("============================")

	for _, rec := range recommendations {
		fmt.Printf("\nRecommended family: %s\n", rec.InstanceFamily)
		if rec.InstanceType != "" {
			fmt.Printf("Specific instance: %s\n", rec.InstanceType)
		}
		fmt.Printf("CPU:Memory ratio: %.1fGB per vCPU\n", rec.CPUMemoryRatio)
		fmt.Printf("Reasoning: %s\n", rec.Reasoning)
		fmt.Printf("Cost impact: %s\n", rec.CostImpact)
		fmt.Printf("Performance impact: %s\n", rec.PerformanceImpact)
		fmt.Printf("Confidence: %.0f%%\n", rec.ConfidenceLevel*100)
	}
}

// displayDecisionImpact shows how optimization changes the AWS vs local decision
func displayDecisionImpact(impact *analyzer.DecisionImpact) {
	fmt.Println("\nDECISION IMPACT")
	fmt.Println("===============")

	fmt.Printf("Original recommendation: %s\n", impact.OriginalRecommendation)
	fmt.Printf("Optimized recommendation: %s\n", impact.OptimizedRecommendation)

	if impact.DecisionChanged {
		fmt.Printf("✨ OPTIMIZATION CHANGED THE DECISION! ✨\n")
	}

	fmt.Printf("Impact: %s\n", impact.ImpactDescription)

	if impact.CostDifferenceChange != 0 {
		fmt.Printf("Cost difference change: $%.2f\n", impact.CostDifferenceChange)
	}
	if impact.TimeDifferenceChange != 0 {
		fmt.Printf("Time difference change: %v\n", impact.TimeDifferenceChange)
	}
}

// Helper functions for display
func getBatchFileName() string {
	if batchFile != "" {
		return batchFile
	}
	return "job.sbatch"
}

func getBurstPartition() string {
	if burstPartition != "" {
		return burstPartition
	}
	return "aws-partition"
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}