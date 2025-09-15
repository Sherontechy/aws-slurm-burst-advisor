package analyzer

import (
	"fmt"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/budget"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// BudgetAwareAnalyzer enhances analysis with budget constraints and grant timeline optimization.
type BudgetAwareAnalyzer struct {
	baseAnalyzer *HistoryAwareAnalyzer
	budgetClient *budget.ASBBClient
	enabled      bool
}

// NewBudgetAwareAnalyzer creates a new budget-aware analyzer.
func NewBudgetAwareAnalyzer(baseAnalyzer *HistoryAwareAnalyzer, budgetClient *budget.ASBBClient) *BudgetAwareAnalyzer {
	return &BudgetAwareAnalyzer{
		baseAnalyzer: baseAnalyzer,
		budgetClient: budgetClient,
		enabled:      budgetClient != nil,
	}
}

// BudgetAwareAnalysis extends EnhancedAnalysis with budget and timeline information.
type BudgetAwareAnalysis struct {
	*EnhancedAnalysis
	BudgetStatus       *budget.BudgetStatus       `json:"budget_status,omitempty"`
	AffordabilityCheck *budget.AffordabilityCheck `json:"affordability_check,omitempty"`
	GrantTimeline      *budget.GrantTimeline      `json:"grant_timeline,omitempty"`
	BudgetRecommendation *BudgetRecommendation    `json:"budget_recommendation,omitempty"`
	TimelineOptimization *TimelineOptimization    `json:"timeline_optimization,omitempty"`
}

// BudgetRecommendation provides budget-aware recommendations.
type BudgetRecommendation struct {
	FinalRecommendation   types.RecommendationType `json:"final_recommendation"`
	BudgetInfluence       string                   `json:"budget_influence"`
	CostOptimizationAdvice []string                `json:"cost_optimization_advice"`
	BudgetRisk            string                   `json:"budget_risk"`
	AlternativeStrategies []AlternativeStrategy    `json:"alternative_strategies"`
}

// TimelineOptimization provides grant timeline-aware optimization.
type TimelineOptimization struct {
	DeadlinePressure      string               `json:"deadline_pressure"`
	CriticalDeadlines     []budget.ResearchDeadline `json:"critical_deadlines"`
	TimelineRecommendation string              `json:"timeline_recommendation"`
	UrgencyFactor         float64              `json:"urgency_factor"`
	DeadlineJustification string               `json:"deadline_justification"`
}

// AlternativeStrategy suggests alternative resource allocation approaches.
type AlternativeStrategy struct {
	Strategy    string  `json:"strategy"`
	Cost        float64 `json:"cost"`
	Timeline    string  `json:"timeline"`
	BudgetRisk  string  `json:"budget_risk"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
}

// AnalyzeWithBudgetConstraints performs analysis enhanced with budget awareness.
func (b *BudgetAwareAnalyzer) AnalyzeWithBudgetConstraints(
	localPartition, awsPartition *types.PartitionAnalysis,
	job *types.JobRequest,
	scriptPath string,
	account string,
) (*BudgetAwareAnalysis, error) {

	// Start with base enhanced analysis
	enhanced, err := b.baseAnalyzer.AnalyzeWithHistory(localPartition, awsPartition, job, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("base analysis failed: %w", err)
	}

	budgetAnalysis := &BudgetAwareAnalysis{
		EnhancedAnalysis: enhanced,
	}

	// If budget integration is disabled, return base analysis
	if !b.enabled || account == "" {
		return budgetAnalysis, nil
	}

	// Check if ASBB is available
	if !b.budgetClient.IsAvailable() {
		fmt.Printf("Warning: ASBB service unavailable, proceeding without budget constraints\n")
		return budgetAnalysis, nil
	}

	// Get budget status
	budgetStatus, err := b.budgetClient.GetAccountStatus(account)
	if err != nil {
		fmt.Printf("Warning: failed to get budget status: %v\n", err)
		return budgetAnalysis, nil
	}
	budgetAnalysis.BudgetStatus = budgetStatus

	// Get affordability check for AWS option
	awsCost := enhanced.Current.BurstPartition.EstimatedCost.TotalCost
	affordabilityCheck, err := b.budgetClient.CheckAffordability(account, awsCost)
	if err != nil {
		fmt.Printf("Warning: failed to check affordability: %v\n", err)
	} else {
		budgetAnalysis.AffordabilityCheck = affordabilityCheck
	}

	// Get grant timeline information
	grantTimeline, err := b.budgetClient.GetGrantTimeline(account)
	if err != nil {
		fmt.Printf("Warning: failed to get grant timeline: %v\n", err)
	} else {
		budgetAnalysis.GrantTimeline = grantTimeline
	}

	// Generate budget-aware recommendation
	budgetRecommendation := b.generateBudgetRecommendation(enhanced, budgetStatus, affordabilityCheck)
	budgetAnalysis.BudgetRecommendation = budgetRecommendation

	// Generate timeline optimization
	if grantTimeline != nil {
		timelineOptimization := b.generateTimelineOptimization(enhanced, grantTimeline)
		budgetAnalysis.TimelineOptimization = timelineOptimization
	}

	return budgetAnalysis, nil
}

// generateBudgetRecommendation creates budget-aware recommendations.
func (b *BudgetAwareAnalyzer) generateBudgetRecommendation(
	analysis *EnhancedAnalysis,
	budgetStatus *budget.BudgetStatus,
	affordabilityCheck *budget.AffordabilityCheck,
) *BudgetRecommendation {

	recommendation := &BudgetRecommendation{
		FinalRecommendation: analysis.Current.Recommendation.Preferred,
		AlternativeStrategies: []AlternativeStrategy{},
	}

	// Analyze budget influence on decision
	if affordabilityCheck != nil {
		switch affordabilityCheck.RecommendedDecision {
		case "LOCAL":
			recommendation.FinalRecommendation = types.RecommendationLocal
			recommendation.BudgetInfluence = "Budget constraints favor local execution"
			recommendation.BudgetRisk = affordabilityCheck.RiskAssessment.BudgetRisk

		case "AWS":
			recommendation.FinalRecommendation = types.RecommendationAWS
			recommendation.BudgetInfluence = "Budget allows AWS execution for better performance"
			recommendation.BudgetRisk = affordabilityCheck.RiskAssessment.BudgetRisk

		case "EITHER":
			// Keep original ASBA recommendation
			recommendation.BudgetInfluence = "Budget neutral - technical factors determine decision"
			recommendation.BudgetRisk = "low"
		}

		// Add alternative strategies from ASBB
		for _, option := range affordabilityCheck.AlternativeOptions {
			recommendation.AlternativeStrategies = append(recommendation.AlternativeStrategies, AlternativeStrategy{
				Strategy:    option.Strategy,
				Cost:        option.Cost,
				Timeline:    option.Timeline,
				Description: option.Description,
				Score:       option.Score,
				BudgetRisk:  "medium", // Default risk level
			})
		}
	}

	// Generate cost optimization advice
	if budgetStatus != nil {
		if budgetStatus.HealthScore < 50 {
			recommendation.CostOptimizationAdvice = append(recommendation.CostOptimizationAdvice,
				"Budget health low - prioritize cost optimization")
		}
		if budgetStatus.BurnRate > budgetStatus.BudgetAvailable/30 {
			recommendation.CostOptimizationAdvice = append(recommendation.CostOptimizationAdvice,
				"High burn rate detected - consider local execution")
		}
		if budgetStatus.GrantDaysRemaining < 30 {
			recommendation.CostOptimizationAdvice = append(recommendation.CostOptimizationAdvice,
				"Grant ending soon - preserve budget for critical experiments")
		}
	}

	return recommendation
}

// generateTimelineOptimization creates grant timeline-aware optimization.
func (b *BudgetAwareAnalyzer) generateTimelineOptimization(
	analysis *EnhancedAnalysis,
	timeline *budget.GrantTimeline,
) *TimelineOptimization {

	optimization := &TimelineOptimization{
		CriticalDeadlines: []budget.ResearchDeadline{},
		UrgencyFactor:     0.5, // Default medium urgency
	}

	// Analyze upcoming deadlines
	criticalDeadlines := []budget.ResearchDeadline{}
	maxUrgency := 0.0

	for _, deadline := range timeline.UpcomingDeadlines {
		if deadline.Urgency == "high" || deadline.Urgency == "critical" {
			criticalDeadlines = append(criticalDeadlines, deadline)

			// Calculate urgency factor based on time until deadline
			daysUntil := float64(deadline.DaysUntil)
			urgency := 1.0 / (1.0 + daysUntil/7.0) // Higher urgency as deadline approaches

			if deadline.Urgency == "critical" {
				urgency *= 2.0 // Double urgency for critical deadlines
			}

			if urgency > maxUrgency {
				maxUrgency = urgency
			}
		}
	}

	optimization.CriticalDeadlines = criticalDeadlines
	optimization.UrgencyFactor = maxUrgency

	// Generate deadline pressure assessment
	switch {
	case maxUrgency > 0.8:
		optimization.DeadlinePressure = "critical"
		optimization.TimelineRecommendation = "Prioritize speed over cost - use AWS for faster results"
		optimization.DeadlineJustification = "Critical deadline within 1-2 weeks justifies premium cost"

	case maxUrgency > 0.5:
		optimization.DeadlinePressure = "high"
		optimization.TimelineRecommendation = "Consider AWS if budget allows - moderate deadline pressure"
		optimization.DeadlineJustification = "Upcoming deadline creates moderate time pressure"

	case maxUrgency > 0.2:
		optimization.DeadlinePressure = "medium"
		optimization.TimelineRecommendation = "Balanced approach - optimize cost-performance ratio"
		optimization.DeadlineJustification = "Sufficient time for cost optimization"

	default:
		optimization.DeadlinePressure = "low"
		optimization.TimelineRecommendation = "Prioritize cost efficiency - no urgent deadlines"
		optimization.DeadlineJustification = "No pressing deadlines, optimize for budget conservation"
	}

	return optimization
}

// IsEnabled returns true if budget integration is enabled.
func (b *BudgetAwareAnalyzer) IsEnabled() bool {
	return b.enabled && b.budgetClient != nil
}

// GetBudgetHealthSummary provides a summary of account budget health.
func (b *BudgetAwareAnalyzer) GetBudgetHealthSummary(account string) (string, error) {
	if !b.enabled {
		return "Budget integration disabled", nil
	}

	status, err := b.budgetClient.GetAccountStatus(account)
	if err != nil {
		return "", err
	}

	switch status.RiskLevel {
	case "low":
		return fmt.Sprintf("Budget healthy: $%.2f available (%.0f%% remaining)",
			status.BudgetAvailable, (status.BudgetAvailable/status.BudgetLimit)*100), nil
	case "medium":
		return fmt.Sprintf("Budget moderate: $%.2f available, burn rate: $%.2f/day",
			status.BudgetAvailable, status.BurnRate), nil
	case "high":
		return fmt.Sprintf("Budget critical: Only $%.2f available, %d days until depletion",
			status.BudgetAvailable, status.GrantDaysRemaining), nil
	default:
		return fmt.Sprintf("Budget status: $%.2f available", status.BudgetAvailable), nil
	}
}