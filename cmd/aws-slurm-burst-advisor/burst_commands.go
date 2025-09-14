package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/analyzer"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/aws"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/config"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/domain"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/history"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/slurm"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

var (
	outputExecutionPlan string
	dryRun             bool
	domainOverride     string
	burstNodes         string
)

// burstCmd implements the integrated analyze-and-execute workflow
var burstCmd = &cobra.Command{
	Use:   "burst <batch-file> <burst-partition> [node-list]",
	Short: "Analyze job and execute via aws-slurm-burst plugin",
	Long: `Integrated workflow that analyzes a job with ASBA intelligence and executes
it optimally via the aws-slurm-burst plugin. This provides a seamless analyze-and-execute
experience for academic researchers.

Examples:
  # Analyze and burst job to AWS
  asba burst training.sbatch gpu-aws aws-gpu-[001-004]

  # Dry run to see what would happen
  asba burst training.sbatch gpu-aws aws-gpu-[001-004] --dry-run

  # Force specific domain detection
  asba burst job.sbatch cpu-aws aws-cpu-[001-016] --domain=climate_modeling`,
	Args: cobra.RangeArgs(2, 3),
	Run:  runBurstCommand,
}

// executionPlanCmd generates execution plans for aws-slurm-burst
var executionPlanCmd = &cobra.Command{
	Use:   "execution-plan <batch-file> <burst-partition>",
	Short: "Generate execution plan for aws-slurm-burst plugin",
	Long: `Generate a JSON execution plan that can be used by the aws-slurm-burst plugin
for optimal job execution on AWS. This separates analysis from execution for
advanced workflows.

Examples:
  # Generate execution plan
  asba execution-plan training.sbatch gpu-aws --output=plan.json

  # Validate existing execution plan
  asba execution-plan --validate plan.json`,
	Args: cobra.ExactArgs(2),
	Run:  runExecutionPlanCommand,
}

// detectDomainCmd analyzes jobs to detect research domains
var detectDomainCmd = &cobra.Command{
	Use:   "detect-domain <batch-file>",
	Short: "Detect research domain from job characteristics",
	Long: `Analyze job script and resource requirements to classify the research domain.
This helps optimize MPI configurations and AWS instance selection.

Examples:
  # Detect domain from script
  asba detect-domain training.sbatch

  # Output as JSON for integration
  asba detect-domain simulation.sbatch --output=json`,
	Args: cobra.ExactArgs(1),
	Run:  runDetectDomainCommand,
}

func init() {
	// Configure burst command
	burstCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be executed without running")
	burstCmd.Flags().StringVar(&domainOverride, "domain", "", "override automatic domain detection")
	burstCmd.Flags().BoolVar(&optimize, "optimize", true, "apply resource optimizations")
	burstCmd.Flags().BoolVar(&trackHistory, "track-history", true, "collect job history for learning")

	// Configure execution-plan command
	executionPlanCmd.Flags().StringVarP(&outputExecutionPlan, "output", "o", "", "output file for execution plan")
	executionPlanCmd.Flags().StringVar(&domainOverride, "domain", "", "override automatic domain detection")
	executionPlanCmd.Flags().BoolVar(&optimize, "optimize", true, "apply resource optimizations")

	// Configure detect-domain command
	detectDomainCmd.Flags().StringVarP(&outputExecutionPlan, "output", "o", "", "output format: json or text")

	// Add commands to root
	rootCmd.AddCommand(burstCmd)
	rootCmd.AddCommand(executionPlanCmd)
	rootCmd.AddCommand(detectDomainCmd)
}

func runBurstCommand(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	batchFile = args[0]
	burstPartition = args[1]
	if len(args) > 2 {
		burstNodes = args[2]
	}

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		fmt.Printf("Error: failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Parse batch script
	batch, err := slurm.ParseBatchScript(batchFile)
	if err != nil {
		fmt.Printf("Error: failed to parse batch script: %v\n", err)
		os.Exit(1)
	}

	jobReq := batch.ToJobRequest()

	// Use partition from script if target not specified
	if targetPartition == "" {
		targetPartition = batch.Partition
	}

	fmt.Printf("🚀 ASBA Burst Analysis for %s\n", batch.JobName)
	fmt.Printf("=====================================\n\n")

	// Perform enhanced analysis with history
	analysis, err := performEnhancedAnalysis(ctx, cfg, jobReq, targetPartition, burstPartition, batchFile)
	if err != nil {
		fmt.Printf("Error: analysis failed: %v\n", err)
		os.Exit(1)
	}

	// Generate execution plan
	planGenerator := analyzer.NewExecutionPlanGenerator(nil, "v0.3.0-dev") // Would pass actual analyzer
	executionPlan, err := planGenerator.GenerateExecutionPlan(analysis, jobReq, batchFile)
	if err != nil {
		fmt.Printf("Error: failed to generate execution plan: %v\n", err)
		os.Exit(1)
	}

	// Display analysis results
	displayResults(analysis.Current)

	if executionPlan.ShouldBurst {
		fmt.Printf("\n🎯 EXECUTION PLAN\n")
		fmt.Printf("================\n")
		displayExecutionPlan(executionPlan)

		if !dryRun {
			// Execute via aws-slurm-burst plugin
			if err := executeViaBurstPlugin(executionPlan, burstNodes); err != nil {
				fmt.Printf("Error: failed to execute via aws-slurm-burst: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("\n[DRY RUN] Would execute: %s\n", executionPlan.GetRecommendedCommand(burstNodes))
		}
	} else {
		fmt.Printf("\n📍 LOCAL EXECUTION RECOMMENDED\n")
		fmt.Printf("==============================\n")
		fmt.Printf("Recommended command: sbatch %s\n", batchFile)
	}
}

func runExecutionPlanCommand(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	batchFile = args[0]
	burstPartition = args[1]

	// Similar to burst command but only generate plan
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		fmt.Printf("Error: failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	batch, err := slurm.ParseBatchScript(batchFile)
	if err != nil {
		fmt.Printf("Error: failed to parse batch script: %v\n", err)
		os.Exit(1)
	}

	jobReq := batch.ToJobRequest()
	if targetPartition == "" {
		targetPartition = batch.Partition
	}

	// Perform analysis
	analysis, err := performEnhancedAnalysis(ctx, cfg, jobReq, targetPartition, burstPartition, batchFile)
	if err != nil {
		fmt.Printf("Error: analysis failed: %v\n", err)
		os.Exit(1)
	}

	// Generate execution plan
	planGenerator := analyzer.NewExecutionPlanGenerator(nil, "v0.3.0-dev")
	executionPlan, err := planGenerator.GenerateExecutionPlan(analysis, jobReq, batchFile)
	if err != nil {
		fmt.Printf("Error: failed to generate execution plan: %v\n", err)
		os.Exit(1)
	}

	// Save or display execution plan
	if outputExecutionPlan != "" {
		if err := planGenerator.SaveExecutionPlan(executionPlan, outputExecutionPlan); err != nil {
			fmt.Printf("Error: failed to save execution plan: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Execution plan saved to: %s\n", outputExecutionPlan)
	} else {
		// Display execution plan
		displayExecutionPlan(executionPlan)
	}
}

func runDetectDomainCommand(cmd *cobra.Command, args []string) {
	batchFile = args[0]

	// Parse batch script
	batch, err := slurm.ParseBatchScript(batchFile)
	if err != nil {
		fmt.Printf("Error: failed to parse batch script: %v\n", err)
		os.Exit(1)
	}

	jobReq := batch.ToJobRequest()

	// Detect domain
	detector := domain.NewDomainDetector()
	classification := detector.DetectDomain(batchFile, jobReq)

	if outputExecutionPlan == "json" {
		// Output as JSON
		fmt.Printf("{\n")
		fmt.Printf("  \"domain\": \"%s\",\n", classification.Domain)
		fmt.Printf("  \"confidence\": %.2f,\n", classification.Confidence)
		fmt.Printf("  \"detection_methods\": %v\n", classification.DetectionMethods)
		fmt.Printf("}\n")
	} else {
		// Human-readable output
		fmt.Printf("🔍 DOMAIN DETECTION\n")
		fmt.Printf("==================\n\n")
		fmt.Printf("Detected domain: %s\n", classification.Domain)
		fmt.Printf("Confidence: %.0f%%\n", classification.Confidence*100)
		fmt.Printf("Detection methods: %v\n", classification.DetectionMethods)

		if classification.Domain != "unknown" {
			fmt.Printf("\nDomain characteristics:\n")
			fmt.Printf("  Communication pattern: %s\n", classification.DomainCharacteristics.TypicalCommunicationPattern)
			fmt.Printf("  Network sensitivity: %s\n", classification.DomainCharacteristics.NetworkSensitivity)
			fmt.Printf("  Scaling behavior: %s\n", classification.DomainCharacteristics.ScalingBehavior)
			fmt.Printf("  Optimal instance families: %v\n", classification.DomainCharacteristics.OptimalInstanceFamilies)
		}
	}
}

// performEnhancedAnalysis runs the full ASBA analysis with history
func performEnhancedAnalysis(ctx context.Context, cfg *config.Config, jobReq *types.JobRequest, target, burst, scriptPath string) (*analyzer.EnhancedAnalysis, error) {
	// Initialize clients
	slurmClient := slurm.NewClient(cfg.SlurmBinPath)
	awsClient, err := aws.NewPricingClient(ctx, cfg.AWS.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	// Get current user for history
	currentUser, err := slurm.GetCurrentUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Initialize history-aware analyzer
	historyDB, err := history.NewJobHistoryDB(currentUser)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize job history: %w", err)
	}
	defer historyDB.Close()

	historyAnalyzer := analyzer.NewHistoryAwareAnalyzer(cfg.Weights, historyDB)

	// Analyze partitions (reuse existing logic)
	targetAnalysis, err := analyzeLocalPartition(ctx, slurmClient, cfg, target, jobReq)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze target partition: %w", err)
	}

	burstAnalysis, err := analyzeBurstPartition(ctx, slurmClient, awsClient, cfg, burst, jobReq)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze burst partition: %w", err)
	}

	// Perform history-aware analysis
	enhanced, err := historyAnalyzer.AnalyzeWithHistory(targetAnalysis, burstAnalysis, jobReq, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to perform enhanced analysis: %w", err)
	}

	return enhanced, nil
}

// displayExecutionPlan shows the generated execution plan
func displayExecutionPlan(plan *types.ExecutionPlan) {
	fmt.Printf("Should burst: %t\n", plan.ShouldBurst)

	if !plan.ShouldBurst {
		fmt.Printf("Recommendation: Execute locally\n")
		return
	}

	fmt.Printf("Domain detected: %s\n", plan.JobMetadata.DetectedDomain)
	fmt.Printf("Workload type: %s\n", plan.JobMetadata.WorkloadType)

	fmt.Printf("\nInstance specification:\n")
	fmt.Printf("  Instance types: %v\n", plan.InstanceSpecification.InstanceTypes)
	fmt.Printf("  Instance count: %d\n", plan.InstanceSpecification.InstanceCount)
	fmt.Printf("  Purchasing: %s\n", plan.InstanceSpecification.PurchasingOption)

	if plan.MPIConfiguration.IsMPIJob {
		fmt.Printf("\nMPI configuration:\n")
		fmt.Printf("  Process count: %d\n", plan.MPIConfiguration.ProcessCount)
		fmt.Printf("  Processes per node: %d\n", plan.MPIConfiguration.ProcessesPerNode)
		fmt.Printf("  Communication pattern: %s\n", plan.MPIConfiguration.CommunicationPattern)
		fmt.Printf("  MPI library: %s\n", plan.MPIConfiguration.MPILibrary)
		fmt.Printf("  Requires EFA: %t\n", plan.MPIConfiguration.RequiresEFA)
	}

	fmt.Printf("\nNetwork configuration:\n")
	fmt.Printf("  Latency class: %s\n", plan.NetworkConfiguration.NetworkLatencyClass)
	fmt.Printf("  Bandwidth requirement: %s\n", plan.NetworkConfiguration.BandwidthRequirement)
	fmt.Printf("  Placement group: %s\n", plan.NetworkConfiguration.PlacementGroupType)

	fmt.Printf("\nCost constraints:\n")
	fmt.Printf("  Max total cost: $%.2f\n", plan.CostConstraints.MaxTotalCost)
	fmt.Printf("  Max duration: %.1fh\n", plan.CostConstraints.MaxDurationHours)
	fmt.Printf("  Prefer spot: %t\n", plan.CostConstraints.PreferSpot)

	if len(plan.OptimizationApplied) > 0 {
		fmt.Printf("\nOptimizations applied:\n")
		for _, opt := range plan.OptimizationApplied {
			fmt.Printf("  • %s\n", opt)
		}
	}

	fmt.Printf("\nConfidence: %.0f%%\n", plan.ConfidenceLevel*100)
}

// executeViaBurstPlugin executes the job via aws-slurm-burst plugin
func executeViaBurstPlugin(plan *types.ExecutionPlan, nodeList string) error {
	if nodeList == "" {
		return fmt.Errorf("node list is required for burst execution")
	}

	// Save execution plan to temporary file
	planFile := "/tmp/asba-execution-plan.json"
	planGenerator := analyzer.NewExecutionPlanGenerator(nil, "v0.3.0-dev")
	if err := planGenerator.SaveExecutionPlan(plan, planFile); err != nil {
		return fmt.Errorf("failed to save execution plan: %w", err)
	}

	// Check if aws-slurm-burst is available
	if _, err := exec.LookPath("aws-slurm-burst"); err != nil {
		return fmt.Errorf("aws-slurm-burst plugin not found in PATH: %w", err)
	}

	// Execute via aws-slurm-burst
	cmd := exec.Command("aws-slurm-burst", "resume", nodeList, "--execution-plan", planFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("🚀 Executing via aws-slurm-burst plugin...\n")
	fmt.Printf("Command: %s\n", cmd.String())

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws-slurm-burst execution failed: %w", err)
	}

	fmt.Printf("✅ Job successfully submitted via aws-slurm-burst\n")

	// Clean up temporary plan file
	os.Remove(planFile)

	return nil
}