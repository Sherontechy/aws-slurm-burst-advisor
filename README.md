# AWS SLURM Burst Advisor

A specialized tool designed for **academic researchers** using HPC clusters to make intelligent decisions about bursting computational workloads to AWS EC2. Helps maximize research productivity while staying within grant budgets by analyzing local cluster conditions versus AWS EC2 costs and performance.

## Overview

The AWS SLURM Burst Advisor is built specifically for research computing environments using SLURM with the AWS plugin. It helps researchers make data-driven decisions about where to run computations by analyzing job requirements against current cluster conditions and real-time AWS pricing.

**Perfect for Academic Research:**
- **Grant Budget Optimization**: Compare true costs between local and AWS execution
- **Research Timeline Management**: Factor in paper deadlines and conference submissions
- **Resource Efficiency**: Maximize computational throughput within budget constraints
- **Cost Transparency**: Understand real costs of local cluster usage vs cloud bursting

**Decision Factors:**
- Current queue depth and estimated wait times on local cluster
- Local cluster utilization and available capacity
- Real-time AWS EC2 spot and on-demand pricing with savings opportunities
- True cost of local cluster resources (amortized hardware, power, staff time)
- Research timeline urgency (paper deadlines, proposal submissions)
- AWS instance availability and startup times

## Features

- **Batch Script Analysis**: Parse existing SLURM batch scripts to extract job requirements
- **Real-time Queue Analysis**: Query current cluster state and queue conditions
- **AWS Cost Integration**: Live pricing from AWS APIs with spot price monitoring
- **Local Cost Modeling**: Realistic cost accounting for local cluster resources
- **AWS-Optimized Recommendations**: Intelligent burst-to-AWS recommendations based on configurable preferences
- **Multiple Input Methods**: Command-line parameters, batch files, or positional arguments
- **AWS-Native**: Built specifically for AWS EC2 and SLURM AWS plugin environments

## Installation

### From Source

```bash
git clone https://github.com/scttfrdmn/aws-slurm-burst-advisor.git
cd aws-slurm-burst-advisor
make build
sudo make install-system
```

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/scttfrdmn/aws-slurm-burst-advisor/releases).

```bash
# Linux AMD64
wget https://github.com/scttfrdmn/aws-slurm-burst-advisor/releases/latest/download/aws-slurm-burst-advisor-linux-amd64.tar.gz
tar -xzf aws-slurm-burst-advisor-linux-amd64.tar.gz
sudo cp aws-slurm-burst-advisor /usr/local/bin/
```

### Quick Start Alias

For convenience, the installation creates a short alias `asba`:

```bash
# Both commands are equivalent:
aws-slurm-burst-advisor job.sbatch gpu-aws
asba job.sbatch gpu-aws
```

## Configuration

### Quick Start

Create example configuration files:

```bash
make config
```

This creates:
- `configs/config.yaml` - Main configuration
- `configs/local-costs.yaml` - Local cluster cost model

### Configuration Files

The tool looks for configuration in these locations (in order):
1. `--config` flag parameter
2. `$HOME/.aws-slurm-burst-advisor.yaml`
3. `/etc/aws-slurm-burst-advisor/config.yaml`
4. `./config.yaml`

### AWS Setup

1. **AWS Plugin Configuration**: Ensure you have the [AWS SLURM plugin v2](https://github.com/aws-samples/aws-plugin-for-slurm/tree/plugin-v2) configured with `partitions.json`

2. **AWS Credentials**: Configure AWS credentials using one of:
   ```bash
   # AWS CLI
   aws configure

   # Environment variables
   export AWS_ACCESS_KEY_ID=your-key
   export AWS_SECRET_ACCESS_KEY=your-secret
   export AWS_DEFAULT_REGION=us-east-1

   # IAM roles (for EC2 instances)
   # Automatically detected
   ```

3. **Required AWS Permissions**:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": [
           "ec2:DescribeSpotPriceHistory",
           "ec2:DescribeInstanceTypes",
           "pricing:GetProducts"
         ],
         "Resource": "*"
       }
     ]
   }
   ```

### Local Cost Configuration

Customize `local-costs.yaml` with your cluster's actual costs:

```yaml
partitions:
  cpu:
    cost_per_cpu_hour: 0.05    # Adjust based on your hardware costs
    cost_per_node_hour: 0.10   # Include facility overhead
    maintenance_factor: 1.3    # Maintenance and support overhead
    power_cost_factor: 1.2     # Power and cooling costs
```

## Usage

### Command-Line Interface

```bash
# Analyze batch script against AWS burst partition
asba --batch-file=job.sbatch --burst-partition=gpu-aws

# Manual job specification
asba --target-partition=cpu --burst-partition=cpu-aws \
     --nodes=4 --cpus-per-task=8 --time=2:00:00

# Quick positional syntax
asba job.sbatch gpu-aws

# Override partition from batch script
asba --batch-file=job.sbatch \
     --target-partition=cpu \
     --burst-partition=cpu-aws
```

### Example Output

```
Analyzing job from gpu_training.sbatch:
  Job: ml-training
  Resources: 2 nodes, 16 CPUs/task, 4h0m0s
  GRES: map[gpu:4]
  Original partition: gpu

ANALYSIS RESULTS
================

TARGET (gpu - Local Cluster):
  Queue depth: 8 jobs ahead
  Est. wait time: 2h 45m
  Available capacity: 4/16 nodes idle
  Cost breakdown:
    Compute cost:     $32.00
    Node cost:        $8.00
    Overhead cost:    $12.00
    Total cost:       $52.00

BURST (gpu-aws - AWS):
  Instance type: p3.8xlarge (32 vCPUs, 4 Tesla V100)
  Current spot price: $2.48/hour
  Startup time: ~3 minutes
  Cost breakdown:
    Compute cost:     $40.32
    Data transfer:    $1.80
    Overhead cost:    $2.01
    Total cost:       $44.13

RECOMMENDATION: Burst to AWS
├─ Time difference: +2h 42m (burst saves time)
├─ Cost difference: -$7.87 (burst costs 15% less)
├─ Break-even point: N/A (burst is both faster and cheaper)
└─ Confidence: 85% (based on current queue state)

Reasoning:
• Significant time savings: 2h42m by using AWS
• AWS costs $7.87 less (15% cost reduction)
• Heavy queue load on local cluster (8 jobs ahead)
• GPU job using p3.8xlarge instances on AWS
```

### Batch Script Support

The tool can parse standard SLURM batch scripts:

```bash
#!/bin/bash
#SBATCH --job-name=my-analysis
#SBATCH --partition=gpu
#SBATCH --nodes=2
#SBATCH --ntasks-per-node=1
#SBATCH --cpus-per-task=16
#SBATCH --gres=gpu:4
#SBATCH --time=4:00:00
#SBATCH --mem=64G

python train_model.py
```

All SLURM directives are automatically extracted and analyzed.

## Academic Research Use Cases

### **Machine Learning Training**
```bash
# Analyze GPU training job with deadline pressure
asba ml_training.sbatch gpu-aws

# Example decision factors:
# - Local queue: 8 jobs ahead, 6h wait time
# - AWS p3.8xlarge: $2.40/hour spot, 3min startup
# - Recommendation: Burst to AWS (saves 5h45m for $12 extra)
# - Perfect for conference deadline scenarios
```

### **Large-Scale Simulations**
```bash
# CPU-intensive climate simulation with flexible timeline
asba --nodes=16 --cpus-per-task=4 --time=12:00:00 \
     --target-partition=cpu --burst-partition=cpu-aws

# Consider: Is 12 extra hours worth $50 savings?
# Ideal for long-running research with flexible deadlines
```

### **Data Processing Pipelines**
```bash
# Memory-intensive genomics processing
asba genomics_pipeline.sbatch memory-aws

# Factors in:
# - Data transfer costs for large datasets
# - Processing urgency for research timelines
# - Memory requirements vs AWS instance types
```

### **Grant Budget Planning**
```bash
# Analyze multiple jobs for quarterly budget estimation
for job in experiments/*.sbatch; do
  echo "=== $(basename $job) ==="
  asba "$job" gpu-aws --json | jq -r '.recommendation | "Cost: $\(.cost_difference) | Time: \(.time_savings)"'
done

# Helps with:
# - NSF/NIH grant budget justification
# - Quarterly research spending planning
# - Cost-per-experiment optimization
```

### **Research Workflow Automation**
```bash
# Smart job submission based on analysis
analyze_and_submit() {
  local job_script=$1
  local aws_partition=$2

  if asba "$job_script" "$aws_partition" --json | jq -e '.recommendation.preferred == "aws"' > /dev/null; then
    echo "Submitting to AWS for faster results..."
    sbatch --partition="$aws_partition" "$job_script"
  else
    echo "Using local cluster for cost efficiency..."
    sbatch "$job_script"
  fi
}

# Usage in research pipelines
analyze_and_submit training_job.sbatch gpu-aws
analyze_and_submit data_analysis.sbatch cpu-aws
```

## Cost Calculation Methodology

### **Local Cluster True Costs**
The tool calculates realistic local cluster costs including:

- **Hardware Amortization**: Server/GPU costs divided by expected lifetime
- **Power & Cooling**: Electricity and HVAC overhead (typically 20-50% of hardware cost)
- **Staff Time**: System administration and maintenance (often overlooked)
- **Facility Costs**: Data center space, networking, security
- **Opportunity Cost**: What else could the budget fund?

### **AWS EC2 Cost Components**
- **Compute**: EC2 instance pricing (spot vs on-demand)
- **Data Transfer**: Upload/download costs for research data
- **Storage**: EBS volumes for temporary data
- **Startup Overhead**: Time cost of instance provisioning

### **Research Budget Impact**
```bash
# Example: 2-year ML research project
Local cluster allocation: $50,000/year
AWS burst budget: $5,000/year

# Tool helps answer:
# - Which experiments should use AWS vs local?
# - How to maximize research output within budget?
# - When is time-to-results worth the cost premium?
```

## Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/scttfrdmn/aws-slurm-burst-advisor.git
cd aws-slurm-burst-advisor

# Set up development environment
make dev-setup

# Build
make build

# Run tests
make test

# Run with hot reload during development
make dev
```

### Project Structure

```
aws-slurm-burst-advisor/
├── cmd/aws-slurm-burst-advisor/    # Main application
├── internal/
│   ├── analyzer/               # Decision engine and cost calculators
│   ├── aws/                    # AWS pricing and integration
│   ├── config/                 # Configuration management
│   ├── slurm/                  # SLURM interface and batch parser
│   └── types/                  # Core data types
├── configs/                    # Example configuration files
├── examples/                   # Example batch scripts
├── docs/                       # Documentation
└── build/                      # Build artifacts
```

### Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Make changes and add tests
4. Run quality checks: `make check`
5. Submit a pull request

## Configuration Reference

### Main Configuration (`config.yaml`)

| Setting | Description | Default |
|---------|-------------|---------|
| `slurm_bin_path` | Path to SLURM binaries | `/usr/bin` |
| `partitions_config_path` | Path to AWS plugin partitions.json | `/etc/slurm/partitions.json` |
| `local_costs_config_path` | Path to local costs configuration | `/etc/aws-slurm-burst-advisor/local-costs.yaml` |
| `aws.region` | AWS region for instances | `us-east-1` |
| `aws.pricing_region` | AWS region for pricing API | `us-east-1` |
| `decision_weights.cost_weight` | Weight for cost considerations (0-1) | `0.3` |
| `decision_weights.time_weight` | Weight for time considerations (0-1) | `0.7` |
| `decision_weights.time_value_per_hour` | Dollar value of researcher time | `50.0` |

### **Academic Configuration Tips**

**For Graduate Students / Postdocs:**
```yaml
decision_weights:
  cost_weight: 0.7        # Budget-conscious
  time_weight: 0.3        # Less time pressure
  time_value_per_hour: 25 # Lower hourly value
```

**For Faculty with Deadlines:**
```yaml
decision_weights:
  cost_weight: 0.2        # Cost less important
  time_weight: 0.8        # Time critical
  time_value_per_hour: 100 # Higher opportunity cost
```

**For Large Research Groups:**
```yaml
local_costs:
  partitions:
    gpu:
      cost_per_gpu_hour: 4.50  # Reflect true A100/H100 costs
      maintenance_factor: 1.6   # Higher for research workloads
```

### Local Costs Configuration (`local-costs.yaml`)

| Setting | Description | Example |
|---------|-------------|---------|
| `partitions.{name}.cost_per_cpu_hour` | Cost per CPU core per hour | `0.05` |
| `partitions.{name}.cost_per_node_hour` | Base cost per node per hour | `0.10` |
| `partitions.{name}.cost_per_gpu_hour` | Cost per GPU per hour | `2.50` |
| `partitions.{name}.maintenance_factor` | Maintenance cost multiplier | `1.3` |
| `partitions.{name}.power_cost_factor` | Power cost multiplier | `1.2` |

## Troubleshooting

### **Research Environment Setup**

**"My local costs seem wrong"**
- Update `local-costs.yaml` with your institution's actual hardware costs
- Include full overhead: power, cooling, staff time, facility costs
- Many universities underestimate true cluster costs by 2-3x

**"AWS costs seem high"**
- Check if you're comparing spot vs on-demand pricing
- Verify data transfer estimates for your workflow
- Consider Reserved Instances for predictable workloads

**"Recommendations don't match my intuition"**
- Adjust `time_value_per_hour` based on your research stage
- Consider deadline pressure vs budget constraints
- Factor in grant renewal timing and available funds

### Common Technical Issues

**1. "SLURM commands not found"**
```bash
# Check SLURM installation
which squeue sinfo scontrol

# Update slurm_bin_path in config.yaml
slurm_bin_path: "/opt/slurm/bin"  # or your SLURM path
```

**2. "AWS partition not found"**
```bash
# Verify partitions.json exists and is readable
ls -la /etc/slurm/partitions.json

# Check AWS plugin configuration
scontrol show partition gpu-aws
```

**3. "Failed to get AWS pricing"**
```bash
# Test AWS credentials
aws sts get-caller-identity

# Check required permissions
aws ec2 describe-spot-price-history --instance-types c5.large --max-items 1
```

**4. "No partition information found"**
```bash
# Test SLURM connectivity
sinfo -p your-partition

# Check partition exists and user has access
scontrol show partition your-partition
```

### Debug Mode

Enable verbose logging:

```bash
asba --verbose your-args
```

### Validation

Test configuration:

```bash
make validate-config
```

## License

[MIT License](LICENSE)

Copyright (c) 2025 Scott Friedman

## Support

- GitHub Issues: [Report bugs and request features](https://github.com/scttfrdmn/aws-slurm-burst-advisor/issues)
- Documentation: [Wiki pages](https://github.com/scttfrdmn/aws-slurm-burst-advisor/wiki)

## Acknowledgments

- [SchedMD](https://schedmd.com/) for SLURM workload manager
- [AWS Samples](https://github.com/aws-samples/aws-plugin-for-slurm) for SLURM AWS plugin
- HPC community for feedback and requirements

---

*Built with ❤️ for the HPC community*