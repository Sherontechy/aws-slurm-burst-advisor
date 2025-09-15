package budget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/errors"
)

// ASBBClient provides integration with ASBB (aws-slurm-burst-budget) APIs.
type ASBBClient struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
	timeout    time.Duration
}

// NewASBBClient creates a new ASBB client.
func NewASBBClient(baseURL string, apiKey string) *ASBBClient {
	return &ASBBClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		timeout: 30 * time.Second,
	}
}

// BudgetStatus represents current budget status from ASBB.
type BudgetStatus struct {
	Account           string    `json:"account"`
	BudgetLimit       float64   `json:"budget_limit"`
	BudgetUsed        float64   `json:"budget_used"`
	BudgetHeld        float64   `json:"budget_held"`
	BudgetAvailable   float64   `json:"budget_available"`
	BurnRate          float64   `json:"burn_rate"`
	BurnRateVariance  float64   `json:"burn_rate_variance"`
	HealthScore       int       `json:"health_score"`
	RiskLevel         string    `json:"risk_level"`
	GrantDaysRemaining int      `json:"grant_days_remaining"`
	Decision          string    `json:"decision"` // PREFER_LOCAL, PREFER_AWS, EITHER, EMERGENCY_ONLY
	CanAffordAWS      bool      `json:"can_afford_aws"`
	LastUpdated       time.Time `json:"last_updated"`
}

// AffordabilityCheck represents affordability analysis from ASBB.
type AffordabilityCheck struct {
	Affordable             bool                   `json:"affordable"`
	RecommendedDecision    string                 `json:"recommended_decision"`
	BudgetImpact          BudgetImpact           `json:"budget_impact"`
	RiskAssessment        RiskAssessment         `json:"risk_assessment"`
	AlternativeOptions    []AlternativeOption    `json:"alternative_options"`
	ConfidenceLevel       float64                `json:"confidence_level"`
}

// BudgetImpact describes the impact of a cost on the budget.
type BudgetImpact struct {
	CostAsPercentOfBudget    float64 `json:"cost_as_percent_of_budget"`
	CostAsPercentOfRemaining float64 `json:"cost_as_percent_of_remaining"`
	OpportunityCost          float64 `json:"opportunity_cost"`
	BudgetAfterCost          float64 `json:"budget_after_cost"`
}

// RiskAssessment describes financial and timeline risks.
type RiskAssessment struct {
	BudgetRisk    string `json:"budget_risk"`    // "low", "medium", "high"
	DeadlineRisk  string `json:"deadline_risk"`  // "low", "medium", "high"
	OverallRisk   string `json:"overall_risk"`   // "low", "medium", "high"
	RiskFactors   []string `json:"risk_factors"`
}

// AlternativeOption suggests alternative resource allocation strategies.
type AlternativeOption struct {
	Strategy    string  `json:"strategy"`
	Cost        float64 `json:"cost"`
	Timeline    string  `json:"timeline"`
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

// GrantTimeline represents grant timeline information from ASBB.
type GrantTimeline struct {
	Account               string              `json:"account"`
	GrantStartDate        time.Time           `json:"grant_start_date"`
	GrantEndDate          time.Time           `json:"grant_end_date"`
	DaysRemaining         int                 `json:"days_remaining"`
	CurrentPeriod         string              `json:"current_period"`
	NextAllocation        NextAllocation      `json:"next_allocation"`
	UpcomingDeadlines     []ResearchDeadline  `json:"upcoming_deadlines"`
	BudgetGuidance        BudgetGuidance      `json:"budget_guidance"`
	EmergencyBurstAdvice  EmergencyAdvice     `json:"emergency_burst_advice"`
}

// NextAllocation describes upcoming budget allocation.
type NextAllocation struct {
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`
	Source string    `json:"source"`
}

// ResearchDeadline represents critical research timeline events.
type ResearchDeadline struct {
	Type        string    `json:"type"`        // "conference", "grant_report", "renewal"
	Name        string    `json:"name"`        // "ICLR 2026", "NSF Quarterly Report"
	Date        time.Time `json:"date"`
	DaysUntil   int       `json:"days_until"`
	Urgency     string    `json:"urgency"`     // "low", "medium", "high", "critical"
	Impact      string    `json:"impact"`      // Description of deadline impact
}

// BudgetGuidance provides spending recommendations.
type BudgetGuidance struct {
	RecommendedStrategy   string  `json:"recommended_strategy"`
	MaxRecommendedSpend   float64 `json:"max_recommended_spend"`
	ConservationAdvice    string  `json:"conservation_advice"`
	OptimizationSuggestions []string `json:"optimization_suggestions"`
}

// EmergencyAdvice provides guidance for critical deadline scenarios.
type EmergencyAdvice struct {
	EmergencyFundsAvailable bool    `json:"emergency_funds_available"`
	EmergencyThreshold      float64 `json:"emergency_threshold"`
	EmergencyProcedure      string  `json:"emergency_procedure"`
	ContactInfo             string  `json:"contact_info"`
}

// GetAccountStatus retrieves current budget status for an account.
func (c *ASBBClient) GetAccountStatus(account string) (*BudgetStatus, error) {
	if account == "" {
		return nil, errors.NewValidationError("GetAccountStatus", "account cannot be empty", nil)
	}

	url := fmt.Sprintf("%s/api/v1/asba/budget-status", c.baseURL)

	requestData := map[string]string{
		"account": account,
	}

	resp, err := c.makeRequest("POST", url, requestData)
	if err != nil {
		return nil, errors.NewNetworkError("GetAccountStatus", "failed to query ASBB budget status", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewNetworkError("GetAccountStatus",
			fmt.Sprintf("ASBB API returned status %d", resp.StatusCode), nil)
	}

	var status BudgetStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, errors.NewValidationError("GetAccountStatus", "failed to parse ASBB response", err)
	}

	return &status, nil
}

// CheckAffordability checks if an account can afford a specific cost.
func (c *ASBBClient) CheckAffordability(account string, estimatedCost float64) (*AffordabilityCheck, error) {
	if account == "" {
		return nil, errors.NewValidationError("CheckAffordability", "account cannot be empty", nil)
	}
	if estimatedCost < 0 {
		return nil, errors.NewValidationError("CheckAffordability", "estimated cost cannot be negative", nil)
	}

	url := fmt.Sprintf("%s/api/v1/asba/affordability-check", c.baseURL)

	requestData := map[string]interface{}{
		"account":        account,
		"estimated_cost": estimatedCost,
	}

	resp, err := c.makeRequest("POST", url, requestData)
	if err != nil {
		return nil, errors.NewNetworkError("CheckAffordability", "failed to query ASBB affordability", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewNetworkError("CheckAffordability",
			fmt.Sprintf("ASBB API returned status %d", resp.StatusCode), nil)
	}

	var check AffordabilityCheck
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return nil, errors.NewValidationError("CheckAffordability", "failed to parse ASBB response", err)
	}

	return &check, nil
}

// GetGrantTimeline retrieves grant timeline and deadline information.
func (c *ASBBClient) GetGrantTimeline(account string) (*GrantTimeline, error) {
	if account == "" {
		return nil, errors.NewValidationError("GetGrantTimeline", "account cannot be empty", nil)
	}

	url := fmt.Sprintf("%s/api/v1/asba/grant-timeline", c.baseURL)

	requestData := map[string]interface{}{
		"account":         account,
		"look_ahead_days": 90,
		"include_alerts":  true,
	}

	resp, err := c.makeRequest("POST", url, requestData)
	if err != nil {
		return nil, errors.NewNetworkError("GetGrantTimeline", "failed to query ASBB grant timeline", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewNetworkError("GetGrantTimeline",
			fmt.Sprintf("ASBB API returned status %d", resp.StatusCode), nil)
	}

	var timeline GrantTimeline
	if err := json.NewDecoder(resp.Body).Decode(&timeline); err != nil {
		return nil, errors.NewValidationError("GetGrantTimeline", "failed to parse ASBB response", err)
	}

	return &timeline, nil
}

// makeRequest makes an HTTP request to the ASBB API.
func (c *ASBBClient) makeRequest(method, url string, data interface{}) (*http.Response, error) {
	var body bytes.Buffer
	if data != nil {
		if err := json.NewEncoder(&body).Encode(data); err != nil {
			return nil, fmt.Errorf("failed to encode request data: %w", err)
		}
	}

	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	return c.httpClient.Do(req)
}

// IsAvailable checks if ASBB service is available.
func (c *ASBBClient) IsAvailable() bool {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/health", c.baseURL))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// SetTimeout configures the HTTP client timeout.
func (c *ASBBClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}