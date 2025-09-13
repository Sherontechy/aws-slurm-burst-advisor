package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// Config represents the main application configuration with validation.
type Config struct {
	// Tool configuration
	SlurmBinPath string `yaml:"slurm_bin_path" json:"slurm_bin_path"`
	LogLevel     string `yaml:"log_level" json:"log_level"`

	// Configuration file paths
	PartitionsConfigPath string `yaml:"partitions_config_path" json:"partitions_config_path"`
	LocalCostsConfigPath string `yaml:"local_costs_config_path" json:"local_costs_config_path"`
	SlurmConfigPath      string `yaml:"slurm_config_path" json:"slurm_config_path"`

	// AWS configuration
	AWS AWSConfig `yaml:"aws" json:"aws"`

	// Local cluster costs
	LocalCosts LocalCostsConfig `yaml:"local_costs" json:"local_costs"`

	// Decision weights
	Weights types.DecisionWeights `yaml:"decision_weights" json:"decision_weights"`

	// Analysis parameters
	Analysis AnalysisConfig `yaml:"analysis" json:"analysis"`

	// Internal cached data
	awsPartitions map[string]*types.AWSPartitionConfig `yaml:"-" json:"-"`
}

// AWSConfig contains AWS-specific configuration with validation.
type AWSConfig struct {
	Region         string `yaml:"region" json:"region"`
	Profile        string `yaml:"profile" json:"profile"`
	PricingRegion  string `yaml:"pricing_region" json:"pricing_region"`

	PricingCacheMinutes int `yaml:"pricing_cache_minutes" json:"pricing_cache_minutes"`
}

// Validate checks if the AWS configuration is valid.
func (a *AWSConfig) Validate() error {
	if a.Region == "" {
		return fmt.Errorf("aws.region is required")
	}
	if a.PricingRegion == "" {
		a.PricingRegion = "us-east-1" // Default pricing region
	}
	if a.PricingCacheMinutes <= 0 {
		a.PricingCacheMinutes = 15 // Default to 15 minutes
	}
	return nil
}

// LocalCostsConfig contains local cluster cost modeling with validation.
type LocalCostsConfig struct {
	DepreciationYears   int     `yaml:"depreciation_years" json:"depreciation_years"`
	UtilizationTarget   float64 `yaml:"utilization_target" json:"utilization_target"`
	OverheadMultiplier  float64 `yaml:"overhead_multiplier" json:"overhead_multiplier"`
	DefaultCurrency     string  `yaml:"default_currency" json:"default_currency"`

	Partitions map[string]types.LocalPartitionCost `yaml:"partitions" json:"partitions"`
}

// Validate checks if the local costs configuration is valid.
func (l *LocalCostsConfig) Validate() error {
	if l.DepreciationYears <= 0 {
		l.DepreciationYears = 3 // Default
	}
	if l.UtilizationTarget <= 0 || l.UtilizationTarget > 1 {
		return fmt.Errorf("utilization_target must be between 0 and 1, got: %f", l.UtilizationTarget)
	}
	if l.OverheadMultiplier < 1 {
		return fmt.Errorf("overhead_multiplier must be >= 1, got: %f", l.OverheadMultiplier)
	}
	if l.DefaultCurrency == "" {
		l.DefaultCurrency = "USD"
	}

	// Validate each partition cost configuration
	for name, cost := range l.Partitions {
		if err := cost.Validate(); err != nil {
			return fmt.Errorf("invalid cost config for partition '%s': %w", name, err)
		}
	}

	return nil
}

// AnalysisConfig contains parameters for analysis algorithms with validation.
type AnalysisConfig struct {
	QueueSamplingMinutes     int     `yaml:"queue_sampling_minutes" json:"queue_sampling_minutes"`
	HistoricalDataDays       int     `yaml:"historical_data_days" json:"historical_data_days"`
	AWSStartupTimeMinutes    int     `yaml:"aws_startup_time_minutes" json:"aws_startup_time_minutes"`
	SpotInterruptionRisk     float64 `yaml:"spot_interruption_risk" json:"spot_interruption_risk"`
	MinConfidenceThreshold   float64 `yaml:"min_confidence_threshold" json:"min_confidence_threshold"`
	DefaultTimeValuePerHour  float64 `yaml:"default_time_value_per_hour" json:"default_time_value_per_hour"`
}

// Validate checks if the analysis configuration is valid.
func (a *AnalysisConfig) Validate() error {
	if a.QueueSamplingMinutes <= 0 {
		a.QueueSamplingMinutes = 5 // Default
	}
	if a.HistoricalDataDays <= 0 {
		a.HistoricalDataDays = 7 // Default
	}
	if a.AWSStartupTimeMinutes < 0 {
		return fmt.Errorf("aws_startup_time_minutes cannot be negative: %d", a.AWSStartupTimeMinutes)
	}
	if a.SpotInterruptionRisk < 0 || a.SpotInterruptionRisk > 1 {
		return fmt.Errorf("spot_interruption_risk must be between 0 and 1, got: %f", a.SpotInterruptionRisk)
	}
	if a.MinConfidenceThreshold < 0 || a.MinConfidenceThreshold > 1 {
		return fmt.Errorf("min_confidence_threshold must be between 0 and 1, got: %f", a.MinConfidenceThreshold)
	}
	if a.DefaultTimeValuePerHour < 0 {
		return fmt.Errorf("default_time_value_per_hour cannot be negative: %f", a.DefaultTimeValuePerHour)
	}
	return nil
}

// AWSPartitionInfo contains resolved AWS partition information.
type AWSPartitionInfo struct {
	PartitionName    string
	Region           string
	InstanceType     string
	PurchasingOption string
	MaxNodes         int
	SlurmSpecs       map[string]string
	NodeGroup        *types.AWSNodeGroup
}

// Validate checks if AWS partition info is valid.
func (a *AWSPartitionInfo) Validate() error {
	if a.PartitionName == "" {
		return fmt.Errorf("partition name cannot be empty")
	}
	if a.Region == "" {
		return fmt.Errorf("region cannot be empty")
	}
	if a.InstanceType == "" {
		return fmt.Errorf("instance type cannot be empty")
	}
	if a.MaxNodes <= 0 {
		return fmt.Errorf("max_nodes must be positive: %d", a.MaxNodes)
	}
	if a.NodeGroup != nil {
		if err := a.NodeGroup.Validate(); err != nil {
			return fmt.Errorf("invalid node group: %w", err)
		}
	}
	return nil
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		SlurmBinPath: "/usr/bin",
		LogLevel:     "info",

		PartitionsConfigPath: "/etc/slurm/partitions.json",
		LocalCostsConfigPath: "/etc/aws-slurm-burst-advisor/local-costs.yaml",
		SlurmConfigPath:      "/etc/slurm/slurm.conf",

		AWS: AWSConfig{
			Region:              "us-east-1",
			PricingRegion:       "us-east-1",
			PricingCacheMinutes: 15,
		},

		LocalCosts: LocalCostsConfig{
			DepreciationYears:  3,
			UtilizationTarget:  0.85,
			OverheadMultiplier: 1.6,
			DefaultCurrency:    "USD",
			Partitions:         make(map[string]types.LocalPartitionCost),
		},

		Weights: types.DecisionWeights{
			CostWeight:       0.3,
			TimeWeight:       0.7,
			RiskWeight:       0.1,
			TimeValuePerHour: 50.0,
		},

		Analysis: AnalysisConfig{
			QueueSamplingMinutes:    5,
			HistoricalDataDays:      7,
			AWSStartupTimeMinutes:   3,
			SpotInterruptionRisk:    0.05,
			MinConfidenceThreshold:  0.6,
			DefaultTimeValuePerHour: 50.0,
		},

		awsPartitions: make(map[string]*types.AWSPartitionConfig),
	}
}

// LoadConfig loads configuration from file or returns default config with validation.
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath != "" {
		if err := cfg.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
		}
	} else {
		// Try default locations
		defaultPaths := getDefaultConfigPaths()

		var lastErr error
		for _, path := range defaultPaths {
			expandedPath := os.ExpandEnv(path)
			if _, err := os.Stat(expandedPath); err == nil {
				if err := cfg.loadFromFile(expandedPath); err != nil {
					lastErr = err
					continue
				}
				break
			}
		}

		// If no config file was found, use defaults but warn
		if lastErr != nil {
			fmt.Printf("Warning: no configuration file found, using defaults. Last error: %v\n", lastErr)
		}
	}

	// Validate the loaded configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Load supplementary configurations
	if err := cfg.loadLocalCosts(); err != nil {
		return nil, fmt.Errorf("failed to load local costs config: %w", err)
	}

	if err := cfg.loadAWSPartitions(); err != nil {
		// AWS partitions config is optional, so just warn
		fmt.Printf("Warning: failed to load AWS partitions config: %v\n", err)
	}

	return cfg, nil
}

// getDefaultConfigPaths returns the standard configuration file locations.
func getDefaultConfigPaths() []string {
	return []string{
		"$HOME/.aws-slurm-burst-advisor.yaml",
		"/etc/aws-slurm-burst-advisor/config.yaml",
		"./config.yaml",
	}
}

// loadFromFile loads configuration from a YAML file with better error handling.
func (c *Config) loadFromFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", path)
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config file as YAML: %w", err)
	}

	return nil
}

// loadLocalCosts loads local cluster cost configuration with validation.
func (c *Config) loadLocalCosts() error {
	if c.LocalCostsConfigPath == "" {
		return nil // No local costs config specified
	}

	paths := []string{c.LocalCostsConfigPath}
	paths = append(paths, getAlternativeLocalCostsPaths()...)

	var lastErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}

		var localCosts LocalCostsConfig
		if err := yaml.Unmarshal(data, &localCosts); err != nil {
			lastErr = fmt.Errorf("failed to parse local costs config: %w", err)
			continue
		}

		// Validate loaded config
		if err := localCosts.Validate(); err != nil {
			lastErr = fmt.Errorf("invalid local costs config: %w", err)
			continue
		}

		// Merge with existing config
		c.mergeLocalCosts(&localCosts)
		return nil
	}

	// If we get here, all attempts failed
	if lastErr != nil {
		return lastErr
	}
	return nil // No local costs config found, which is OK
}

// getAlternativeLocalCostsPaths returns alternative locations for local costs config.
func getAlternativeLocalCostsPaths() []string {
	return []string{
		"/etc/aws-slurm-burst-advisor/local-costs.yaml",
		"./local-costs.yaml",
		"./configs/local-costs.yaml",
	}
}

// mergeLocalCosts merges loaded local costs configuration.
func (c *Config) mergeLocalCosts(loaded *LocalCostsConfig) {
	if loaded.Partitions != nil {
		if c.LocalCosts.Partitions == nil {
			c.LocalCosts.Partitions = make(map[string]types.LocalPartitionCost)
		}
		for name, cost := range loaded.Partitions {
			c.LocalCosts.Partitions[name] = cost
		}
	}
	if loaded.DepreciationYears > 0 {
		c.LocalCosts.DepreciationYears = loaded.DepreciationYears
	}
	if loaded.UtilizationTarget > 0 {
		c.LocalCosts.UtilizationTarget = loaded.UtilizationTarget
	}
	if loaded.OverheadMultiplier > 0 {
		c.LocalCosts.OverheadMultiplier = loaded.OverheadMultiplier
	}
	if loaded.DefaultCurrency != "" {
		c.LocalCosts.DefaultCurrency = loaded.DefaultCurrency
	}
}

// loadAWSPartitions loads AWS partition configuration with validation.
func (c *Config) loadAWSPartitions() error {
	if c.PartitionsConfigPath == "" {
		return fmt.Errorf("partitions config path not specified")
	}

	if _, err := os.Stat(c.PartitionsConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("AWS partitions config file does not exist: %s", c.PartitionsConfigPath)
	}

	data, err := os.ReadFile(c.PartitionsConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read partitions config: %w", err)
	}

	var partitionsConfig struct {
		Partitions []types.AWSPartitionConfig `json:"Partitions"`
	}

	if err := json.Unmarshal(data, &partitionsConfig); err != nil {
		return fmt.Errorf("failed to parse partitions config: %w", err)
	}

	// Validate and index partitions
	c.awsPartitions = make(map[string]*types.AWSPartitionConfig)
	for i, partition := range partitionsConfig.Partitions {
		if err := partition.Validate(); err != nil {
			return fmt.Errorf("invalid partition config '%s': %w", partition.PartitionName, err)
		}
		c.awsPartitions[partition.PartitionName] = &partitionsConfig.Partitions[i]
	}

	return nil
}

// Validate performs comprehensive validation of the configuration.
func (c *Config) Validate() error {
	// Validate required fields
	if c.SlurmBinPath == "" {
		return fmt.Errorf("slurm_bin_path is required")
	}

	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	valid := false
	for _, level := range validLogLevels {
		if strings.ToLower(c.LogLevel) == level {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid log_level: %s (must be debug, info, warn, or error)", c.LogLevel)
	}

	// Validate SLURM binaries exist
	if err := c.validateSlurmBinaries(); err != nil {
		return fmt.Errorf("SLURM validation failed: %w", err)
	}

	// Validate sub-configurations
	if err := c.AWS.Validate(); err != nil {
		return fmt.Errorf("AWS config validation failed: %w", err)
	}

	if err := c.LocalCosts.Validate(); err != nil {
		return fmt.Errorf("local costs validation failed: %w", err)
	}

	if err := c.Weights.Validate(); err != nil {
		return fmt.Errorf("decision weights validation failed: %w", err)
	}

	if err := c.Analysis.Validate(); err != nil {
		return fmt.Errorf("analysis config validation failed: %w", err)
	}

	return nil
}

// validateSlurmBinaries checks if required SLURM binaries are accessible.
func (c *Config) validateSlurmBinaries() error {
	requiredBinaries := []string{"squeue", "sinfo", "scontrol"}
	for _, binary := range requiredBinaries {
		path := filepath.Join(c.SlurmBinPath, binary)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("SLURM binary '%s' not found at %s", binary, path)
		}
	}
	return nil
}

// GetAWSPartitionConfig returns AWS configuration for a partition with validation.
func (c *Config) GetAWSPartitionConfig(partitionName string) (*AWSPartitionInfo, error) {
	if partitionName == "" {
		return nil, fmt.Errorf("partition name cannot be empty")
	}

	partitionConfig, exists := c.awsPartitions[partitionName]
	if !exists {
		return nil, fmt.Errorf("AWS partition '%s' not found in configuration", partitionName)
	}

	if len(partitionConfig.NodeGroups) == 0 {
		return nil, fmt.Errorf("no node groups configured for partition '%s'", partitionName)
	}

	// Use the first node group as primary
	nodeGroup := partitionConfig.NodeGroups[0]
	instanceType := nodeGroup.GetPrimaryInstanceType()
	if instanceType == "" {
		return nil, fmt.Errorf("no instance type configured for partition '%s'", partitionName)
	}

	info := &AWSPartitionInfo{
		PartitionName:    partitionName,
		Region:          nodeGroup.Region,
		InstanceType:    instanceType,
		PurchasingOption: nodeGroup.PurchasingOption,
		MaxNodes:        nodeGroup.MaxNodes,
		SlurmSpecs:      nodeGroup.SlurmSpecifications,
		NodeGroup:       &nodeGroup,
	}

	// Validate before returning
	if err := info.Validate(); err != nil {
		return nil, fmt.Errorf("invalid AWS partition info: %w", err)
	}

	return info, nil
}

// GetLocalPartitionCost returns cost configuration for a local partition.
func (c *Config) GetLocalPartitionCost(partitionName string) *types.LocalPartitionCost {
	if cost, exists := c.LocalCosts.Partitions[partitionName]; exists {
		return &cost
	}

	// Return default cost model with validation
	defaultCost := &types.LocalPartitionCost{
		CostPerCPUHour:    0.05,
		CostPerNodeHour:   0.10,
		CostPerGPUHour:    2.50,
		MaintenanceFactor: 1.3,
		PowerCostFactor:   1.2,
	}

	// Validate default costs
	if err := defaultCost.Validate(); err != nil {
		// This should never happen with hardcoded defaults, but be safe
		fmt.Printf("Warning: default cost validation failed: %v\n", err)
	}

	return defaultCost
}

// Save writes the current configuration to a file with validation.
func (c *Config) Save(path string) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("cannot save invalid configuration: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetAWSPartitionNames returns a list of available AWS partition names.
func (c *Config) GetAWSPartitionNames() []string {
	names := make([]string, 0, len(c.awsPartitions))
	for name := range c.awsPartitions {
		names = append(names, name)
	}
	return names
}

// GetLocalPartitionNames returns a list of configured local partition names.
func (c *Config) GetLocalPartitionNames() []string {
	names := make([]string, 0, len(c.LocalCosts.Partitions))
	for name := range c.LocalCosts.Partitions {
		names = append(names, name)
	}
	return names
}

// IsAWSPartition checks if a partition name corresponds to an AWS partition.
func (c *Config) IsAWSPartition(partitionName string) bool {
	_, exists := c.awsPartitions[partitionName]
	return exists
}