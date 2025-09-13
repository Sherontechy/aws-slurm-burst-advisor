#!/bin/bash
#SBATCH --job-name=ml-training
#SBATCH --partition=gpu
#SBATCH --nodes=2
#SBATCH --ntasks-per-node=1
#SBATCH --cpus-per-task=16
#SBATCH --gres=gpu:4
#SBATCH --time=4:00:00
#SBATCH --mem=64G
#SBATCH --output=ml_training_%j.out
#SBATCH --error=ml_training_%j.err
#SBATCH --mail-type=BEGIN,END,FAIL
#SBATCH --mail-user=user@example.com

# Load required modules
module load cuda/11.8
module load python/3.9

# Activate virtual environment
source /home/user/venv/bin/activate

# Set up distributed training environment
export NCCL_DEBUG=INFO
export NCCL_SOCKET_IFNAME=^docker0,lo

# Run distributed training
srun python -u train_model.py \
    --data-dir /shared/datasets/imagenet \
    --model resnet50 \
    --batch-size 256 \
    --epochs 100 \
    --lr 0.1 \
    --distributed \
    --output-dir /shared/results/ml-training-${SLURM_JOB_ID}