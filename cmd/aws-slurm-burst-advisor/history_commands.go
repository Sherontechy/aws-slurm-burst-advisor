package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/history"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/slurm"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

var (
	historyUser   string
	historyScript string
	showPatterns  bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View your job execution history and patterns",
	Long: `View your personal job execution history, efficiency patterns, and optimization insights.

Examples:
  # View last 30 days of job history
  asba history --days 30

  # View history for a specific script
  asba history --script training.sbatch

  # View aggregated patterns
  asba history --patterns`,
	Run: runHistoryCommand,
}

var insightsCmd = &cobra.Command{
	Use:   "insights",
	Short: "Show efficiency insights and optimization opportunities",
	Long: `Analyze your job execution patterns to identify optimization opportunities
and provide insights for better resource allocation.

Examples:
  # Show general efficiency insights
  asba insights

  # Show insights for specific workload types
  asba insights --workload memory-bound`,
	Run: runInsightsCommand,
}

func init() {
	// Add history command
	historyCmd.Flags().IntVar(&historyDays, "days", 30, "days of history to show")
	historyCmd.Flags().StringVar(&historyUser, "user", "", "user to show history for (defaults to current user)")
	historyCmd.Flags().StringVar(&historyScript, "script", "", "show history for specific script")
	historyCmd.Flags().BoolVar(&showPatterns, "patterns", false, "show aggregated job patterns")

	// Add insights command
	insightsCmd.Flags().IntVar(&historyDays, "days", 90, "days of history to analyze")
	insightsCmd.Flags().StringVar(&historyUser, "user", "", "user to analyze (defaults to current user)")

	// Add commands to root
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(insightsCmd)
}

func runHistoryCommand(cmd *cobra.Command, args []string) {
	// Get current user if not specified
	username := historyUser
	if username == "" {
		currentUser, err := slurm.GetCurrentUser()
		if err != nil {
			fmt.Printf("Error: failed to get current user: %v\n", err)
			os.Exit(1)
		}
		username = currentUser
	}

	// Initialize history database
	historyDB, err := history.NewJobHistoryDB(username)
	if err != nil {
		fmt.Printf("Error: failed to initialize job history: %v\n", err)
		os.Exit(1)
	}
	defer historyDB.Close()

	if showPatterns {
		// Show aggregated patterns
		patterns, err := historyDB.GetJobPatterns()
		if err != nil {
			fmt.Printf("Error: failed to get job patterns: %v\n", err)
			os.Exit(1)
		}

		displayJobPatterns(patterns)
	} else {
		// Show job history (this would need a new method in historyDB)
		fmt.Printf("Job history for user: %s (last %d days)\n", username, historyDays)

		// Get basic statistics
		count, err := historyDB.GetJobCount()
		if err != nil {
			fmt.Printf("Error: failed to get job count: %v\n", err)
			os.Exit(1)
		}

		dbSize, err := historyDB.GetDatabaseSize()
		if err != nil {
			fmt.Printf("Error: failed to get database size: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Total jobs tracked: %d\n", count)
		fmt.Printf("Database size: %.2f MB\n", float64(dbSize)/1024/1024)
		fmt.Printf("Database location: %s\n", historyDB.GetDatabasePath())

		if count == 0 {
			fmt.Println("\nNo job history found. Run 'asba --track-history' to start collecting data.")
		}
	}
}

func runInsightsCommand(cmd *cobra.Command, args []string) {
	// Get current user if not specified
	username := historyUser
	if username == "" {
		currentUser, err := slurm.GetCurrentUser()
		if err != nil {
			fmt.Printf("Error: failed to get current user: %v\n", err)
			os.Exit(1)
		}
		username = currentUser
	}

	// Initialize history database
	historyDB, err := history.NewJobHistoryDB(username)
	if err != nil {
		fmt.Printf("Error: failed to initialize job history: %v\n", err)
		os.Exit(1)
	}
	defer historyDB.Close()

	// Get job patterns for insights
	patterns, err := historyDB.GetJobPatterns()
	if err != nil {
		fmt.Printf("Error: failed to get job patterns: %v\n", err)
		os.Exit(1)
	}

	if len(patterns) == 0 {
		fmt.Println("No job patterns found. Run more jobs with --track-history to generate insights.")
		return
	}

	displayInsights(patterns)
}

func displayJobPatterns(patterns []types.JobPattern) {
	fmt.Println("\nJOB PATTERNS")
	fmt.Println("============")

	if len(patterns) == 0 {
		fmt.Println("No patterns detected yet. Run more jobs to build patterns.")
		return
	}

	for _, pattern := range patterns {
		fmt.Printf("\nScript: %s\n", pattern.ScriptName)
		fmt.Printf("  Runs: %d (last: %s)\n", pattern.RunCount, pattern.LastRun.Format("2006-01-02"))
		fmt.Printf("  Workload type: %s\n", pattern.WorkloadType)
		fmt.Printf("  Avg runtime: %v\n", pattern.AvgRuntime)
		fmt.Printf("  Success rate: %.0f%%\n", pattern.SuccessRate*100)
		fmt.Printf("  CPU efficiency: %.1f%%\n", pattern.AvgCPUEfficiency)
		fmt.Printf("  Memory efficiency: %.1f%%\n", pattern.AvgMemoryEfficiency)
		fmt.Printf("  Typical memory usage: %.1fGB\n", pattern.TypicalMemoryUsageGB)
		fmt.Printf("  CPU:Memory ratio: %.1f GB/core (requested) → %.1f GB/effective-core (actual)\n",
			pattern.AvgRequestedRatio, pattern.AvgActualRatio)

		if pattern.PreferredPlatform != "" {
			fmt.Printf("  Platform preference: %s (%d local, %d AWS executions)\n",
				pattern.PreferredPlatform, pattern.LocalExecutions, pattern.AWSExecutions)
		}
	}
}

func displayInsights(patterns []types.JobPattern) {
	fmt.Println("\nEFFICIENCY INSIGHTS")
	fmt.Println("===================")

	// Overall statistics
	totalRuns := 0
	cpuEffSum := 0.0
	memEffSum := 0.0
	workloadTypes := make(map[string]int)

	for _, pattern := range patterns {
		totalRuns += pattern.RunCount
		cpuEffSum += pattern.AvgCPUEfficiency * float64(pattern.RunCount)
		memEffSum += pattern.AvgMemoryEfficiency * float64(pattern.RunCount)
		workloadTypes[pattern.WorkloadType]++
	}

	if totalRuns > 0 {
		fmt.Printf("Overall statistics (%d total job runs):\n", totalRuns)
		fmt.Printf("  Average CPU efficiency: %.1f%%\n", cpuEffSum/float64(totalRuns))
		fmt.Printf("  Average memory efficiency: %.1f%%\n", memEffSum/float64(totalRuns))
		fmt.Printf("  Job patterns tracked: %d\n", len(patterns))
	}

	fmt.Printf("\nWorkload distribution:\n")
	for workloadType, count := range workloadTypes {
		fmt.Printf("  %s: %d patterns\n", workloadType, count)
	}

	// Optimization opportunities
	fmt.Printf("\nOptimization opportunities:\n")
	cpuOverAlloc := 0
	memOverAlloc := 0
	for _, pattern := range patterns {
		if pattern.AvgCPUEfficiency < 60 {
			cpuOverAlloc++
		}
		if pattern.AvgMemoryEfficiency < 70 {
			memOverAlloc++
		}
	}

	if cpuOverAlloc > 0 {
		fmt.Printf("  CPU over-allocation: %d patterns could reduce CPU requests\n", cpuOverAlloc)
	}
	if memOverAlloc > 0 {
		fmt.Printf("  Memory over-allocation: %d patterns could reduce memory requests\n", memOverAlloc)
	}

	if cpuOverAlloc == 0 && memOverAlloc == 0 {
		fmt.Println("  ✓ No major over-allocation detected - your resource requests look good!")
	}
}