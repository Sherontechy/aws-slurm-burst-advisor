#!/bin/bash
#SBATCH --job-name=data-analysis
#SBATCH --partition=cpu
#SBATCH --nodes=4
#SBATCH --ntasks-per-node=8
#SBATCH --cpus-per-task=2
#SBATCH --time=2:30:00
#SBATCH --mem-per-cpu=4G
#SBATCH --output=analysis_%j.out
#SBATCH --error=analysis_%j.err

# Load required modules
module load gcc/11.2.0
module load openmpi/4.1.2
module load python/3.10

# Set up MPI environment
export OMP_NUM_THREADS=${SLURM_CPUS_PER_TASK}

# Run distributed data processing
srun python -m mpi4py analyze_dataset.py \
    --input-dir /shared/data/raw \
    --output-dir /shared/results/analysis-${SLURM_JOB_ID} \
    --chunk-size 1000 \
    --processes ${SLURM_NTASKS}