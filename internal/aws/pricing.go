package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

// PricingClient provides AWS pricing information
type PricingClient struct {
	ec2Client     *ec2.Client
	pricingClient *pricing.Client
	region        string
	pricingRegion string

	// Cache to avoid repeated API calls
	cache      map[string]*cachedPricing
	cacheMutex sync.RWMutex
	cacheTTL   time.Duration
}

type cachedPricing struct {
	pricing     *types.AWSInstancePricing
	lastUpdated time.Time
}

type instanceSpecs struct {
	VCPUs       int
	Memory      float64
	GPUs        int
	GPUType     string
	NetworkPerf string
	StorageType string
}

// NewPricingClient creates a new AWS pricing client
func NewPricingClient(ctx context.Context, region string) (*PricingClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Pricing API is only available in us-east-1
	pricingCfg := cfg.Copy()
	pricingCfg.Region = "us-east-1"

	return &PricingClient{
		ec2Client:     ec2.NewFromConfig(cfg),
		pricingClient: pricing.NewFromConfig(pricingCfg),
		region:        region,
		pricingRegion: "us-east-1",
		cache:         make(map[string]*cachedPricing),
		cacheTTL:      15 * time.Minute, // Cache pricing for 15 minutes
	}, nil
}

// GetInstancePricing retrieves current pricing for an instance type
func (c *PricingClient) GetInstancePricing(ctx context.Context, instanceType, region string) (*types.AWSInstancePricing, error) {
	cacheKey := fmt.Sprintf("%s-%s", instanceType, region)

	// Check cache first
	c.cacheMutex.RLock()
	if cached, exists := c.cache[cacheKey]; exists {
		if time.Since(cached.lastUpdated) < c.cacheTTL {
			c.cacheMutex.RUnlock()
			return cached.pricing, nil
		}
	}
	c.cacheMutex.RUnlock()

	// Fetch current pricing
	pricing, err := c.fetchInstancePricing(ctx, instanceType, region)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pricing for %s: %w", instanceType, err)
	}

	// Update cache
	c.cacheMutex.Lock()
	c.cache[cacheKey] = &cachedPricing{
		pricing:     pricing,
		lastUpdated: time.Now(),
	}
	c.cacheMutex.Unlock()

	return pricing, nil
}

// fetchInstancePricing fetches pricing from AWS APIs
func (c *PricingClient) fetchInstancePricing(ctx context.Context, instanceType, region string) (*types.AWSInstancePricing, error) {
	pricing := &types.AWSInstancePricing{
		InstanceType: instanceType,
		Region:       region,
		Currency:     "USD",
		LastUpdated:  time.Now(),
	}

	// Get instance specifications
	specs, err := c.getInstanceSpecs(ctx, instanceType)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance specs: %w", err)
	}

	pricing.VCPUs = specs.VCPUs
	pricing.Memory = specs.Memory
	pricing.GPUs = specs.GPUs
	pricing.GPUType = specs.GPUType
	pricing.NetworkPerf = specs.NetworkPerf
	pricing.StorageType = specs.StorageType

	// Get spot pricing
	spotPrice, err := c.getSpotPrice(ctx, instanceType, region)
	if err != nil {
		// Log warning but continue - spot pricing might not be available
		fmt.Printf("Warning: failed to get spot price for %s: %v\n", instanceType, err)
	} else {
		pricing.SpotPrice = spotPrice
	}

	// Get on-demand pricing
	onDemandPrice, err := c.getOnDemandPrice(ctx, instanceType, region)
	if err != nil {
		// Try to estimate from spot price if available
		if pricing.SpotPrice > 0 {
			pricing.OnDemandPrice = pricing.SpotPrice * 3.5 // Rough estimate
			fmt.Printf("Warning: using estimated on-demand price for %s\n", instanceType)
		} else {
			return nil, fmt.Errorf("failed to get on-demand price: %w", err)
		}
	} else {
		pricing.OnDemandPrice = onDemandPrice
	}

	return pricing, nil
}

// getSpotPrice retrieves current spot price for instance type
func (c *PricingClient) getSpotPrice(ctx context.Context, instanceType, region string) (float64, error) {
	// Get spot price history for the most recent price
	input := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
		ProductDescriptions: []string{
			"Linux/UNIX",
			"Linux/UNIX (Amazon VPC)",
		},
		MaxResults: aws.Int32(1),
		StartTime:  aws.Time(time.Now().Add(-1 * time.Hour)),
	}

	result, err := c.ec2Client.DescribeSpotPriceHistory(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to describe spot price history: %w", err)
	}

	if len(result.SpotPriceHistory) == 0 {
		return 0, fmt.Errorf("no spot price data available for %s in %s", instanceType, region)
	}

	// Parse the price string
	priceStr := aws.ToString(result.SpotPriceHistory[0].SpotPrice)
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse spot price '%s': %w", priceStr, err)
	}

	return price, nil
}

// getOnDemandPrice retrieves on-demand price using pricing API
func (c *PricingClient) getOnDemandPrice(ctx context.Context, instanceType, region string) (float64, error) {
	// Build pricing service filters
	filters := []pricingtypes.Filter{
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("ServiceCode"),
			Value: aws.String("AmazonEC2"),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("instanceType"),
			Value: aws.String(instanceType),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("location"),
			Value: aws.String(c.regionToLocationName(region)),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("tenancy"),
			Value: aws.String("Shared"),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("operatingSystem"),
			Value: aws.String("Linux"),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("preInstalledSw"),
			Value: aws.String("NA"),
		},
		{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String("capacitystatus"),
			Value: aws.String("Used"),
		},
	}

	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		Filters:     filters,
	}

	result, err := c.pricingClient.GetProducts(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("failed to get products from pricing API: %w", err)
	}

	if len(result.PriceList) == 0 {
		return 0, fmt.Errorf("no pricing data found for %s in %s", instanceType, region)
	}

	// Parse the complex pricing JSON structure
	// This is a simplified parsing - the actual AWS pricing structure is quite complex
	priceStr := result.PriceList[0]
	price, err := c.parsePricingJSON(priceStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse pricing data: %w", err)
	}

	return price, nil
}

// getInstanceSpecs retrieves instance specifications
func (c *PricingClient) getInstanceSpecs(ctx context.Context, instanceType string) (*instanceSpecs, error) {
	// This is a simplified approach - in practice, you might want to maintain
	// a database of instance specifications or use additional AWS APIs

	specs := &instanceSpecs{}

	// Parse instance type for basic info (e.g., "c5.2xlarge")
	parts := strings.Split(instanceType, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid instance type format: %s", instanceType)
	}

	family := parts[0]
	size := parts[1]

	// Basic mapping - this could be replaced with a comprehensive lookup table
	specs.VCPUs = c.getVCPUsForSize(family, size)
	specs.Memory = c.getMemoryForSize(family, size)
	specs.NetworkPerf = c.getNetworkPerf(family, size)
	specs.StorageType = "EBS-only"

	// Check for GPU instances
	if c.isGPUInstance(family) {
		specs.GPUs = c.getGPUCount(family, size)
		specs.GPUType = c.getGPUType(family)
	}

	return specs, nil
}

// Helper methods for instance specifications
func (c *PricingClient) getVCPUsForSize(family, size string) int {
	// Simplified mapping - should be replaced with comprehensive data
	sizeMap := map[string]int{
		"nano":     1,
		"micro":    1,
		"small":    1,
		"medium":   1,
		"large":    2,
		"xlarge":   4,
		"2xlarge":  8,
		"4xlarge":  16,
		"8xlarge":  32,
		"12xlarge": 48,
		"16xlarge": 64,
		"24xlarge": 96,
	}

	if vcpus, exists := sizeMap[size]; exists {
		return vcpus
	}

	// Try to extract number from size (e.g., "9xlarge" -> 36)
	if strings.HasSuffix(size, "xlarge") {
		numStr := strings.TrimSuffix(size, "xlarge")
		if num, err := strconv.Atoi(numStr); err == nil {
			return num * 4 // Assume 4 vCPUs per "x"
		}
	}

	return 2 // Default fallback
}

func (c *PricingClient) getMemoryForSize(family, size string) float64 {
	// Simplified memory calculation based on family type
	vcpus := float64(c.getVCPUsForSize(family, size))

	switch {
	case strings.HasPrefix(family, "r"): // Memory optimized
		return vcpus * 8.0
	case strings.HasPrefix(family, "c"): // Compute optimized
		return vcpus * 2.0
	case strings.HasPrefix(family, "m"): // General purpose
		return vcpus * 4.0
	case strings.HasPrefix(family, "t"): // Burstable
		return vcpus * 4.0
	default:
		return vcpus * 4.0 // Default ratio
	}
}

func (c *PricingClient) isGPUInstance(family string) bool {
	gpuFamilies := []string{"p2", "p3", "p4", "p5", "g3", "g4", "g5", "g6"}
	for _, gpuFamily := range gpuFamilies {
		if family == gpuFamily {
			return true
		}
	}
	return false
}

func (c *PricingClient) getGPUCount(family, size string) int {
	// Simplified GPU count mapping
	if family == "p3" {
		switch size {
		case "2xlarge":
			return 1
		case "8xlarge":
			return 4
		case "16xlarge":
			return 8
		}
	}
	// Add more mappings as needed
	return 1
}

func (c *PricingClient) getGPUType(family string) string {
	gpuTypes := map[string]string{
		"p2": "Tesla K80",
		"p3": "Tesla V100",
		"p4": "Tesla A100",
		"p5": "Tesla H100",
		"g3": "Tesla M60",
		"g4": "Tesla T4",
		"g5": "Tesla A10G",
		"g6": "Tesla L4",
	}

	if gpuType, exists := gpuTypes[family]; exists {
		return gpuType
	}
	return "Unknown"
}

func (c *PricingClient) getNetworkPerf(family, size string) string {
	vcpus := c.getVCPUsForSize(family, size)

	switch {
	case vcpus >= 32:
		return "25 Gigabit"
	case vcpus >= 16:
		return "10 Gigabit"
	case vcpus >= 8:
		return "High"
	case vcpus >= 4:
		return "Moderate"
	default:
		return "Low to Moderate"
	}
}

func (c *PricingClient) regionToLocationName(region string) string {
	// Map AWS region codes to pricing API location names
	locationMap := map[string]string{
		"us-east-1":      "US East (N. Virginia)",
		"us-east-2":      "US East (Ohio)",
		"us-west-1":      "US West (N. California)",
		"us-west-2":      "US West (Oregon)",
		"eu-west-1":      "Europe (Ireland)",
		"eu-central-1":   "Europe (Frankfurt)",
		"ap-southeast-1": "Asia Pacific (Singapore)",
		"ap-northeast-1": "Asia Pacific (Tokyo)",
	}

	if location, exists := locationMap[region]; exists {
		return location
	}

	return region // Fallback to region code
}

func (c *PricingClient) parsePricingJSON(priceJSON string) (float64, error) {
	// This is a simplified parser for AWS pricing JSON
	// The actual structure is very complex and nested

	// Look for hourly price in the JSON string
	// This is a very basic implementation - should use proper JSON parsing
	if strings.Contains(priceJSON, "USD") && strings.Contains(priceJSON, "Hrs") {
		// Extract price using regex or JSON parsing
		// For now, return a placeholder
		return 0.10, nil // TODO: Implement proper JSON parsing
	}

	return 0, fmt.Errorf("unable to parse pricing JSON")
}

// GetRegionSpotPrices retrieves spot prices for multiple instance types
func (c *PricingClient) GetRegionSpotPrices(ctx context.Context, instanceTypes []string, region string) (map[string]float64, error) {
	prices := make(map[string]float64)

	// Get recent spot price history for all instance types
	input := &ec2.DescribeSpotPriceHistoryInput{
		ProductDescriptions: []string{"Linux/UNIX", "Linux/UNIX (Amazon VPC)"},
		MaxResults:          aws.Int32(100),
		StartTime:           aws.Time(time.Now().Add(-1 * time.Hour)),
	}

	// Convert string slice to InstanceType slice
	var instanceTypeValues []ec2types.InstanceType
	for _, it := range instanceTypes {
		instanceTypeValues = append(instanceTypeValues, ec2types.InstanceType(it))
	}
	input.InstanceTypes = instanceTypeValues

	result, err := c.ec2Client.DescribeSpotPriceHistory(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get spot price history: %w", err)
	}

	// Parse prices
	for _, spotPrice := range result.SpotPriceHistory {
		instanceType := string(spotPrice.InstanceType)
		priceStr := aws.ToString(spotPrice.SpotPrice)

		if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
			// Keep the most recent price for each instance type
			if existingPrice, exists := prices[instanceType]; !exists || price < existingPrice {
				prices[instanceType] = price
			}
		}
	}

	return prices, nil
}

// EstimateDataTransferCosts estimates data transfer costs for HPC workloads
func (c *PricingClient) EstimateDataTransferCosts(jobSize string, region string) (float64, error) {
	// Basic data transfer cost estimation
	// This could be enhanced based on actual workload patterns

	baseCost := 0.09 // $0.09/GB for data transfer out (typical rate)

	// Parse job size and estimate data movement
	// This is a simplified approach - real implementation would need workload analysis
	switch {
	case strings.Contains(strings.ToLower(jobSize), "small"):
		return baseCost * 1.0, nil // 1 GB
	case strings.Contains(strings.ToLower(jobSize), "medium"):
		return baseCost * 5.0, nil // 5 GB
	case strings.Contains(strings.ToLower(jobSize), "large"):
		return baseCost * 20.0, nil // 20 GB
	default:
		return baseCost * 10.0, nil // 10 GB default
	}
}

// ClearCache clears the pricing cache
func (c *PricingClient) ClearCache() {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()
	c.cache = make(map[string]*cachedPricing)
}

// GetCacheSize returns the number of cached entries
func (c *PricingClient) GetCacheSize() int {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	return len(c.cache)
}