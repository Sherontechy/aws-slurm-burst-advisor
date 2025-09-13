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
)

var rootCmd = &cobra.Command{
	Use:   "aws-slurm-burst-advisor",
	Short: "Analyze whether to submit jobs to local cluster or burst to AWS",
	Long: `AWS SLURM Burst Advisor - A tool to help HPC users decide whether to submit
their jobs to the local cluster or burst to AWS based on queue conditions, costs,
and time-to-completion analysis. Built specifically for AWS ParallelCluster environments.

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

	analyzer := analyzer.NewDecisionEngine(cfg.Weights)

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

	// Generate recommendation
	recommendation := analyzer.Compare(targetAnalysis, burstAnalysis, jobReq)

	return &types.Analysis{
		TargetPartition: targetAnalysis,
		BurstPartition:  burstAnalysis,
		Recommendation:  recommendation,
		Timestamp:       time.Now(),
		JobRequest:      jobReq,
	}, nil
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
	fmt.Printf("RECOMMENDATION: ")
	if rec.Preferred == "target" {
		fmt.Printf("Use Local Cluster\n")
	} else {
		fmt.Printf("Burst to AWS\n")
	}

	fmt.Printf("├─ Time difference: ")
	if rec.TimeSavings > 0 {
		fmt.Printf("+%v (burst saves time)\n", rec.TimeSavings)
	} else if rec.TimeSavings < 0 {
		fmt.Printf("%v (local is faster)\n", -rec.TimeSavings)
	} else {
		fmt.Printf("~0 (similar timing)\n")
	}

	fmt.Printf("├─ Cost difference: ")
	if rec.CostDifference > 0 {
		fmt.Printf("+$%.2f (burst costs more)\n", rec.CostDifference)
	} else if rec.CostDifference < 0 {
		fmt.Printf("-$%.2f (burst costs less)\n", -rec.CostDifference)
	} else {
		fmt.Printf("~$0 (similar cost)\n")
	}

	if rec.BreakevenTime > 0 {
		fmt.Printf("├─ Break-even point: %v (if wait time exceeds this, burst is worth it)\n",
			rec.BreakevenTime)
	} else {
		fmt.Printf("├─ Break-even point: N/A\n")
	}

	fmt.Printf("└─ Confidence: %.0f%% (based on current queue state)\n", rec.Confidence*100)

	if len(rec.Reasoning) > 0 {
		fmt.Printf("\nReasoning:\n")
		for _, reason := range rec.Reasoning {
			fmt.Printf("• %s\n", reason)
		}
	}
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}