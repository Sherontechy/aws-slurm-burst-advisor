package types

import (
	"fmt"
	"time"
)

// AWSInstancePricing contains comprehensive pricing information for AWS instances.
type AWSInstancePricing struct {
	InstanceType    string    `json:"instance_type" yaml:"instance_type"`
	Region          string    `json:"region" yaml:"region"`
	OnDemandPrice   float64   `json:"on_demand_price" yaml:"on_demand_price"`
	SpotPrice       float64   `json:"spot_price" yaml:"spot_price"`
	Currency        string    `json:"currency" yaml:"currency"`
	LastUpdated     time.Time `json:"last_updated" yaml:"last_updated"`
	VCPUs           int       `json:"vcpus" yaml:"vcpus"`
	Memory          float64   `json:"memory" yaml:"memory"`
	GPUs            int       `json:"gpus" yaml:"gpus"`
	GPUType         string    `json:"gpu_type" yaml:"gpu_type"`
	NetworkPerf     string    `json:"network_perf" yaml:"network_perf"`
	StorageType     string    `json:"storage_type" yaml:"storage_type"`
}

// IsSpotAvailable returns true if spot pricing is available and current.
func (p *AWSInstancePricing) IsSpotAvailable() bool {
	return p.SpotPrice > 0 && time.Since(p.LastUpdated) < 30*time.Minute
}

// EffectivePrice returns the lowest available price (spot if available, otherwise on-demand).
func (p *AWSInstancePricing) EffectivePrice() float64 {
	if p.IsSpotAvailable() {
		return p.SpotPrice
	}
	return p.OnDemandPrice
}

// SpotSavings returns the percentage savings when using spot vs on-demand.
func (p *AWSInstancePricing) SpotSavings() float64 {
	if p.OnDemandPrice == 0 || p.SpotPrice == 0 {
		return 0.0
	}
	return (p.OnDemandPrice - p.SpotPrice) / p.OnDemandPrice * 100.0
}

// Validate checks if the pricing data is complete and valid.
func (p *AWSInstancePricing) Validate() error {
	if p.InstanceType == "" {
		return fmt.Errorf("instance_type cannot be empty")
	}
	if p.Region == "" {
		return fmt.Errorf("region cannot be empty")
	}
	if p.OnDemandPrice < 0 {
		return fmt.Errorf("on_demand_price cannot be negative: %f", p.OnDemandPrice)
	}
	if p.SpotPrice < 0 {
		return fmt.Errorf("spot_price cannot be negative: %f", p.SpotPrice)
	}
	if p.VCPUs <= 0 {
		return fmt.Errorf("vcpus must be positive: %d", p.VCPUs)
	}
	if p.Memory <= 0 {
		return fmt.Errorf("memory must be positive: %f", p.Memory)
	}
	return nil
}

// AWSPartitionConfig contains AWS-specific partition configuration.
type AWSPartitionConfig struct {
	PartitionName string         `json:"partition_name" yaml:"partition_name"`
	InstanceType  string         `json:"instance_type" yaml:"instance_type"`
	Region        string         `json:"region" yaml:"region"`
	SubnetID      string         `json:"subnet_id" yaml:"subnet_id"`
	SecurityGroup string         `json:"security_group" yaml:"security_group"`
	ImageID       string         `json:"image_id" yaml:"image_id"`
	NodeGroups    []AWSNodeGroup `json:"node_groups" yaml:"node_groups"`

	// Resource specifications
	CPUs         int    `json:"cpus" yaml:"cpus"`
	Memory       int    `json:"memory" yaml:"memory"`
	GPUs         int    `json:"gpus" yaml:"gpus"`
	GPUType      string `json:"gpu_type" yaml:"gpu_type"`

	// Scaling parameters
	MaxNodes     int `json:"max_nodes" yaml:"max_nodes"`
	MinNodes     int `json:"min_nodes" yaml:"min_nodes"`

	// Instance preferences
	SpotFleetRequest bool   `json:"spot_fleet_request" yaml:"spot_fleet_request"`
	InstanceFamily   string `json:"instance_family" yaml:"instance_family"`
}

// Validate checks if the AWS partition configuration is valid.
func (c *AWSPartitionConfig) Validate() error {
	if c.PartitionName == "" {
		return fmt.Errorf("partition_name cannot be empty")
	}
	if c.Region == "" {
		return fmt.Errorf("region cannot be empty")
	}
	if c.MaxNodes < c.MinNodes {
		return fmt.Errorf("max_nodes (%d) cannot be less than min_nodes (%d)", c.MaxNodes, c.MinNodes)
	}
	if len(c.NodeGroups) == 0 {
		return fmt.Errorf("at least one node group must be specified")
	}
	return nil
}

// AWSNodeGroup represents AWS partition node group configuration.
type AWSNodeGroup struct {
	Name                string            `json:"Name" yaml:"name"`
	Region              string            `json:"Region" yaml:"region"`
	MaxNodes            int               `json:"MaxNodesCount" yaml:"max_nodes"`
	InstanceTypes       []string          `json:"InstanceTypes" yaml:"instance_types"`
	PurchasingOption    string            `json:"PurchasingOption" yaml:"purchasing_option"`
	SlurmSpecifications map[string]string `json:"SlurmSpecifications" yaml:"slurm_specifications"`
}

// GetPrimaryInstanceType returns the primary instance type for the node group.
func (ng *AWSNodeGroup) GetPrimaryInstanceType() string {
	if len(ng.InstanceTypes) > 0 {
		return ng.InstanceTypes[0]
	}
	return ""
}

// IsSpotEnabled returns true if the node group uses spot instances.
func (ng *AWSNodeGroup) IsSpotEnabled() bool {
	return ng.PurchasingOption == "spot"
}

// Validate checks if the node group configuration is valid.
func (ng *AWSNodeGroup) Validate() error {
	if ng.Name == "" {
		return fmt.Errorf("node group name cannot be empty")
	}
	if ng.Region == "" {
		return fmt.Errorf("node group region cannot be empty")
	}
	if ng.MaxNodes <= 0 {
		return fmt.Errorf("max_nodes must be positive: %d", ng.MaxNodes)
	}
	if len(ng.InstanceTypes) == 0 {
		return fmt.Errorf("at least one instance type must be specified")
	}
	validPurchasingOptions := []string{"spot", "on-demand"}
	valid := false
	for _, option := range validPurchasingOptions {
		if ng.PurchasingOption == option {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid purchasing option: %s (must be 'spot' or 'on-demand')", ng.PurchasingOption)
	}
	return nil
}

// LocalPartitionCost contains cost parameters for local cluster partitions.
type LocalPartitionCost struct {
	CostPerCPUHour    float64 `yaml:"cost_per_cpu_hour" json:"cost_per_cpu_hour"`
	CostPerNodeHour   float64 `yaml:"cost_per_node_hour" json:"cost_per_node_hour"`
	CostPerGPUHour    float64 `yaml:"cost_per_gpu_hour" json:"cost_per_gpu_hour"`
	MaintenanceFactor float64 `yaml:"maintenance_factor" json:"maintenance_factor"`
	PowerCostFactor   float64 `yaml:"power_cost_factor" json:"power_cost_factor"`
}

// Validate checks if the local partition cost configuration is valid.
func (c *LocalPartitionCost) Validate() error {
	if c.CostPerCPUHour < 0 {
		return fmt.Errorf("cost_per_cpu_hour cannot be negative: %f", c.CostPerCPUHour)
	}
	if c.CostPerNodeHour < 0 {
		return fmt.Errorf("cost_per_node_hour cannot be negative: %f", c.CostPerNodeHour)
	}
	if c.CostPerGPUHour < 0 {
		return fmt.Errorf("cost_per_gpu_hour cannot be negative: %f", c.CostPerGPUHour)
	}
	if c.MaintenanceFactor < 1.0 {
		return fmt.Errorf("maintenance_factor must be >= 1.0: %f", c.MaintenanceFactor)
	}
	if c.PowerCostFactor < 1.0 {
		return fmt.Errorf("power_cost_factor must be >= 1.0: %f", c.PowerCostFactor)
	}
	return nil
}