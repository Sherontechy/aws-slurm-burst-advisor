package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/budget"
)

var (
	budgetAccount   string
	budgetEndpoint  string
	showTimeline    bool
	estimatedCost   float64
)

// budgetCmd provides budget-related commands
var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Budget management and analysis commands",
	Long: `Budget management commands for academic grant integration.
Requires ASBB (aws-slurm-burst-budget) service to be running.

Examples:
  # Check budget status
  asba budget status --account=NSF-ABC123

  # Check if you can afford a job
  asba budget check --account=NSF-ABC123 --cost=45.67

  # View grant timeline
  asba budget timeline --account=NSF-ABC123`,
}

var budgetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current budget status",
	Long: `Display current budget status including available funds, burn rate,
and budget health score for the specified grant account.`,
	Run: runBudgetStatusCommand,
}

var budgetCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check affordability of estimated cost",
	Long: `Check if the specified account can afford an estimated cost,
with recommendations for cost optimization if needed.`,
	Run: runBudgetCheckCommand,
}

var budgetTimelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show grant timeline and deadlines",
	Long: `Display grant timeline information including deadlines,
budget allocation schedule, and research deadline optimization advice.`,
	Run: runBudgetTimelineCommand,
}

func init() {
	// Configure budget status command
	budgetStatusCmd.Flags().StringVarP(&budgetAccount, "account", "A", "", "budget account (required)")
	budgetStatusCmd.Flags().StringVar(&budgetEndpoint, "endpoint", "http://localhost:8080", "ASBB service endpoint")
	budgetStatusCmd.MarkFlagRequired("account")

	// Configure budget check command
	budgetCheckCmd.Flags().StringVarP(&budgetAccount, "account", "A", "", "budget account (required)")
	budgetCheckCmd.Flags().Float64Var(&estimatedCost, "cost", 0, "estimated cost to check (required)")
	budgetCheckCmd.Flags().StringVar(&budgetEndpoint, "endpoint", "http://localhost:8080", "ASBB service endpoint")
	budgetCheckCmd.MarkFlagRequired("account")
	budgetCheckCmd.MarkFlagRequired("cost")

	// Configure budget timeline command
	budgetTimelineCmd.Flags().StringVarP(&budgetAccount, "account", "A", "", "budget account (required)")
	budgetTimelineCmd.Flags().StringVar(&budgetEndpoint, "endpoint", "http://localhost:8080", "ASBB service endpoint")
	budgetTimelineCmd.Flags().BoolVar(&showTimeline, "show-timeline", true, "show full grant timeline")
	budgetTimelineCmd.MarkFlagRequired("account")

	// Add subcommands
	budgetCmd.AddCommand(budgetStatusCmd)
	budgetCmd.AddCommand(budgetCheckCmd)
	budgetCmd.AddCommand(budgetTimelineCmd)

	// Add to root command
	rootCmd.AddCommand(budgetCmd)
}

func runBudgetStatusCommand(cmd *cobra.Command, args []string) {
	// Initialize ASBB client
	client := budget.NewASBBClient(budgetEndpoint, "")

	// Check if ASBB is available
	if !client.IsAvailable() {
		fmt.Printf("Error: ASBB service not available at %s\n", budgetEndpoint)
		fmt.Printf("Please ensure ASBB (aws-slurm-burst-budget) is running\n")
		os.Exit(1)
	}

	// Get budget status
	status, err := client.GetAccountStatus(budgetAccount)
	if err != nil {
		fmt.Printf("Error: failed to get budget status: %v\n", err)
		os.Exit(1)
	}

	// Display budget status
	displayBudgetStatus(status)
}

func runBudgetCheckCommand(cmd *cobra.Command, args []string) {
	// Initialize ASBB client
	client := budget.NewASBBClient(budgetEndpoint, "")

	// Check if ASBB is available
	if !client.IsAvailable() {
		fmt.Printf("Error: ASBB service not available at %s\n", budgetEndpoint)
		os.Exit(1)
	}

	// Check affordability
	check, err := client.CheckAffordability(budgetAccount, estimatedCost)
	if err != nil {
		fmt.Printf("Error: failed to check affordability: %v\n", err)
		os.Exit(1)
	}

	// Display affordability check
	displayAffordabilityCheck(check, estimatedCost)
}

func runBudgetTimelineCommand(cmd *cobra.Command, args []string) {
	// Initialize ASBB client
	client := budget.NewASBBClient(budgetEndpoint, "")

	// Check if ASBB is available
	if !client.IsAvailable() {
		fmt.Printf("Error: ASBB service not available at %s\n", budgetEndpoint)
		os.Exit(1)
	}

	// Get grant timeline
	timeline, err := client.GetGrantTimeline(budgetAccount)
	if err != nil {
		fmt.Printf("Error: failed to get grant timeline: %v\n", err)
		os.Exit(1)
	}

	// Display grant timeline
	displayGrantTimeline(timeline)
}

func displayBudgetStatus(status *budget.BudgetStatus) {
	fmt.Println("💰 BUDGET STATUS")
	fmt.Println("================")
	fmt.Printf("Account: %s\n", status.Account)
	fmt.Printf("Budget limit: $%.2f\n", status.BudgetLimit)
	fmt.Printf("Budget used: $%.2f\n", status.BudgetUsed)
	fmt.Printf("Budget available: $%.2f\n", status.BudgetAvailable)
	fmt.Printf("Budget held: $%.2f\n", status.BudgetHeld)

	fmt.Printf("\nBurn rate: $%.2f/day (±%.2f)\n", status.BurnRate, status.BurnRateVariance)
	fmt.Printf("Health score: %d/100\n", status.HealthScore)
	fmt.Printf("Risk level: %s\n", status.RiskLevel)

	if status.GrantDaysRemaining > 0 {
		fmt.Printf("Grant days remaining: %d\n", status.GrantDaysRemaining)
	}

	fmt.Printf("\nASBB Recommendation: %s\n", status.Decision)
	if status.CanAffordAWS {
		fmt.Printf("✅ Can afford AWS bursting\n")
	} else {
		fmt.Printf("❌ AWS bursting not recommended (budget constraints)\n")
	}
}

func displayAffordabilityCheck(check *budget.AffordabilityCheck, cost float64) {
	fmt.Println("🔍 AFFORDABILITY CHECK")
	fmt.Println("======================")
	fmt.Printf("Estimated cost: $%.2f\n", cost)
	fmt.Printf("Affordable: %t\n", check.Affordable)
	fmt.Printf("Recommended decision: %s\n", check.RecommendedDecision)
	fmt.Printf("Confidence: %.0f%%\n", check.ConfidenceLevel*100)

	fmt.Printf("\nBudget impact:\n")
	fmt.Printf("  Cost as %% of budget: %.1f%%\n", check.BudgetImpact.CostAsPercentOfBudget)
	fmt.Printf("  Cost as %% of remaining: %.1f%%\n", check.BudgetImpact.CostAsPercentOfRemaining)
	fmt.Printf("  Budget after cost: $%.2f\n", check.BudgetImpact.BudgetAfterCost)

	fmt.Printf("\nRisk assessment:\n")
	fmt.Printf("  Budget risk: %s\n", check.RiskAssessment.BudgetRisk)
	fmt.Printf("  Deadline risk: %s\n", check.RiskAssessment.DeadlineRisk)
	fmt.Printf("  Overall risk: %s\n", check.RiskAssessment.OverallRisk)

	if len(check.RiskAssessment.RiskFactors) > 0 {
		fmt.Printf("  Risk factors: %v\n", check.RiskAssessment.RiskFactors)
	}

	if len(check.AlternativeOptions) > 0 {
		fmt.Printf("\nAlternative strategies:\n")
		for _, option := range check.AlternativeOptions {
			fmt.Printf("  • %s: $%.2f (%s) - %s\n",
				option.Strategy, option.Cost, option.Timeline, option.Description)
		}
	}
}

func displayGrantTimeline(timeline *budget.GrantTimeline) {
	fmt.Println("📅 GRANT TIMELINE")
	fmt.Println("=================")
	fmt.Printf("Account: %s\n", timeline.Account)
	fmt.Printf("Grant period: %s to %s\n",
		timeline.GrantStartDate.Format("2006-01-02"),
		timeline.GrantEndDate.Format("2006-01-02"))
	fmt.Printf("Days remaining: %d\n", timeline.DaysRemaining)
	fmt.Printf("Current period: %s\n", timeline.CurrentPeriod)

	if !timeline.NextAllocation.Date.IsZero() {
		fmt.Printf("\nNext allocation:\n")
		fmt.Printf("  Date: %s\n", timeline.NextAllocation.Date.Format("2006-01-02"))
		fmt.Printf("  Amount: $%.2f\n", timeline.NextAllocation.Amount)
		fmt.Printf("  Source: %s\n", timeline.NextAllocation.Source)
	}

	if len(timeline.UpcomingDeadlines) > 0 {
		fmt.Printf("\nUpcoming deadlines:\n")
		for _, deadline := range timeline.UpcomingDeadlines {
			urgencyIcon := "📅"
			if deadline.Urgency == "critical" {
				urgencyIcon = "🚨"
			} else if deadline.Urgency == "high" {
				urgencyIcon = "⚠️"
			}

			fmt.Printf("  %s %s (%s) - %d days until %s\n",
				urgencyIcon, deadline.Name, deadline.Type,
				deadline.DaysUntil, deadline.Date.Format("2006-01-02"))
			if deadline.Impact != "" {
				fmt.Printf("     Impact: %s\n", deadline.Impact)
			}
		}
	}

	fmt.Printf("\nBudget guidance:\n")
	fmt.Printf("  Strategy: %s\n", timeline.BudgetGuidance.RecommendedStrategy)
	fmt.Printf("  Max recommended spend: $%.2f\n", timeline.BudgetGuidance.MaxRecommendedSpend)
	if timeline.BudgetGuidance.ConservationAdvice != "" {
		fmt.Printf("  Conservation advice: %s\n", timeline.BudgetGuidance.ConservationAdvice)
	}

	if len(timeline.BudgetGuidance.OptimizationSuggestions) > 0 {
		fmt.Printf("  Optimization suggestions:\n")
		for _, suggestion := range timeline.BudgetGuidance.OptimizationSuggestions {
			fmt.Printf("    • %s\n", suggestion)
		}
	}

	if timeline.EmergencyBurstAdvice.EmergencyFundsAvailable {
		fmt.Printf("\n🚨 Emergency burst options available\n")
		fmt.Printf("  Threshold: $%.2f\n", timeline.EmergencyBurstAdvice.EmergencyThreshold)
		fmt.Printf("  Procedure: %s\n", timeline.EmergencyBurstAdvice.EmergencyProcedure)
	}
}