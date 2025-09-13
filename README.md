# AWS SLURM Burst Advisor

A specialized tool designed for **AWS ParallelCluster** environments to help HPC users make intelligent decisions about bursting workloads to AWS. Analyzes local cluster conditions versus AWS costs and performance to provide data-driven burst recommendations.

## Overview

The AWS SLURM Burst Advisor is built specifically for AWS-integrated HPC environments using SLURM with the AWS plugin. It analyzes your job requirements against current cluster conditions and real-time AWS pricing to provide intelligent recommendations on where to run your workload.

**Key Decision Factors:**
- Current queue depth and estimated wait times on local cluster
- Local cluster utilization and available capacity
- Real-time AWS EC2 spot and on-demand pricing
- True cost of local cluster resources (hardware, power, maintenance)
- Time value considerations and urgency requirements
- AWS instance availability and startup times

## Features

- **Batch Script Analysis**: Parse existing SLURM batch scripts to extract job requirements
- **Real-time Queue Analysis**: Query current cluster state and queue conditions
- **AWS Cost Integration**: Live pricing from AWS APIs with spot price monitoring
- **Local Cost Modeling**: Realistic cost accounting for local cluster resources
- **AWS-Optimized Recommendations**: Intelligent burst-to-AWS recommendations based on configurable preferences
- **Multiple Input Methods**: Command-line parameters, batch files, or positional arguments
- **AWS-Native**: Built specifically for AWS ParallelCluster and SLURM AWS plugin environments

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
aws-slurm-burst-advisor --batch-file=job.sbatch --burst-partition=gpu-aws

# Manual job specification
aws-slurm-burst-advisor --target-partition=cpu --burst-partition=cpu-aws \
                        --nodes=4 --cpus-per-task=8 --time=2:00:00

# Quick positional syntax
aws-slurm-burst-advisor job.sbatch gpu-aws

# Override partition from batch script
aws-slurm-burst-advisor --batch-file=job.sbatch \
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

### Local Costs Configuration (`local-costs.yaml`)

| Setting | Description | Example |
|---------|-------------|---------|
| `partitions.{name}.cost_per_cpu_hour` | Cost per CPU core per hour | `0.05` |
| `partitions.{name}.cost_per_node_hour` | Base cost per node per hour | `0.10` |
| `partitions.{name}.cost_per_gpu_hour` | Cost per GPU per hour | `2.50` |
| `partitions.{name}.maintenance_factor` | Maintenance cost multiplier | `1.3` |
| `partitions.{name}.power_cost_factor` | Power cost multiplier | `1.2` |

## Troubleshooting

### Common Issues

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
aws-slurm-burst-advisor --verbose your-args
```

### Validation

Test configuration:

```bash
make validate-config
```

## License

[MIT License](LICENSE)

## Support

- GitHub Issues: [Report bugs and request features](https://github.com/scttfrdmn/aws-slurm-burst-advisor/issues)
- Documentation: [Wiki pages](https://github.com/scttfrdmn/aws-slurm-burst-advisor/wiki)

## Acknowledgments

- [SchedMD](https://schedmd.com/) for SLURM workload manager
- [AWS Samples](https://github.com/aws-samples/aws-plugin-for-slurm) for SLURM AWS plugin
- HPC community for feedback and requirements

---

*Built with ❤️ for the HPC community*