package types

import (
	"testing"
	"time"
)

func TestJobRequest_Validate(t *testing.T) {
	tests := []struct {
		name      string
		job       JobRequest
		wantError bool
	}{
		{
			name: "valid job request",
			job: JobRequest{
				Nodes:         2,
				CPUsPerTask:   4,
				NTasksPerNode: 1,
				TimeLimit:     time.Hour,
			},
			wantError: false,
		},
		{
			name: "zero nodes",
			job: JobRequest{
				Nodes:       0,
				CPUsPerTask: 4,
				TimeLimit:   time.Hour,
			},
			wantError: true,
		},
		{
			name: "negative CPUs",
			job: JobRequest{
				Nodes:       2,
				CPUsPerTask: -1,
				TimeLimit:   time.Hour,
			},
			wantError: true,
		},
		{
			name: "zero time limit",
			job: JobRequest{
				Nodes:       2,
				CPUsPerTask: 4,
				TimeLimit:   0,
			},
			wantError: true,
		},
		{
			name: "auto-correct ntasks per node",
			job: JobRequest{
				Nodes:         2,
				CPUsPerTask:   4,
				NTasksPerNode: 0, // Should be corrected to 1
				TimeLimit:     time.Hour,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("JobRequest.Validate() error = %v, wantError %v", err, tt.wantError)
			}

			// Check that NTasksPerNode was corrected
			if !tt.wantError && tt.job.NTasksPerNode == 0 {
				if tt.job.NTasksPerNode != 1 {
					t.Errorf("Expected NTasksPerNode to be corrected to 1, got %d", tt.job.NTasksPerNode)
				}
			}
		})
	}
}

func TestJobRequest_TotalCPUs(t *testing.T) {
	job := JobRequest{
		Nodes:         3,
		CPUsPerTask:   4,
		NTasksPerNode: 2,
	}

	expected := 3 * 4 * 2 // 24
	got := job.TotalCPUs()

	if got != expected {
		t.Errorf("JobRequest.TotalCPUs() = %d, want %d", got, expected)
	}
}

func TestJobRequest_HasGPUs(t *testing.T) {
	tests := []struct {
		name string
		tres map[string]int
		want bool
	}{
		{
			name: "has GPUs",
			tres: map[string]int{"gpu": 4},
			want: true,
		},
		{
			name: "no GPUs",
			tres: map[string]int{},
			want: false,
		},
		{
			name: "zero GPUs",
			tres: map[string]int{"gpu": 0},
			want: false,
		},
		{
			name: "nil TRES",
			tres: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := JobRequest{TRES: tt.tres}
			got := job.HasGPUs()
			if got != tt.want {
				t.Errorf("JobRequest.HasGPUs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobRequest_TotalGPUs(t *testing.T) {
	tests := []struct {
		name  string
		nodes int
		tres  map[string]int
		want  int
	}{
		{
			name:  "4 GPUs per node, 2 nodes",
			nodes: 2,
			tres:  map[string]int{"gpu": 4},
			want:  8,
		},
		{
			name:  "no GPUs",
			nodes: 2,
			tres:  map[string]int{},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := JobRequest{
				Nodes: tt.nodes,
				TRES:  tt.tres,
			}
			got := job.TotalGPUs()
			if got != tt.want {
				t.Errorf("JobRequest.TotalGPUs() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBatchScript_ToJobRequest(t *testing.T) {
	batch := BatchScript{
		JobName:       "test-job",
		Nodes:         2,
		CPUsPerTask:   4,
		NTasksPerNode: 1,
		TimeLimit:     2 * time.Hour,
		Memory:        "8G",
		GRES:          map[string]int{"gpu": 2},
		Account:       "test-account",
	}

	job := batch.ToJobRequest()

	if job.JobName != batch.JobName {
		t.Errorf("JobName = %s, want %s", job.JobName, batch.JobName)
	}
	if job.Nodes != batch.Nodes {
		t.Errorf("Nodes = %d, want %d", job.Nodes, batch.Nodes)
	}
	if job.TotalGPUs() != 4 { // 2 nodes * 2 GPUs
		t.Errorf("TotalGPUs = %d, want 4", job.TotalGPUs())
	}
}

func TestBatchScript_IsArrayJob(t *testing.T) {
	tests := []struct {
		name          string
		rawDirectives map[string]string
		want          bool
	}{
		{
			name:          "is array job",
			rawDirectives: map[string]string{"array": "1-10"},
			want:          true,
		},
		{
			name:          "not array job",
			rawDirectives: map[string]string{},
			want:          false,
		},
		{
			name:          "nil directives",
			rawDirectives: nil,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := BatchScript{RawDirectives: tt.rawDirectives}
			got := batch.IsArrayJob()
			if got != tt.want {
				t.Errorf("BatchScript.IsArrayJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBatchScript_IsExclusive(t *testing.T) {
	tests := []struct {
		name          string
		rawDirectives map[string]string
		want          bool
	}{
		{
			name:          "is exclusive",
			rawDirectives: map[string]string{"exclusive": "true"},
			want:          true,
		},
		{
			name:          "not exclusive",
			rawDirectives: map[string]string{"exclusive": "false"},
			want:          false,
		},
		{
			name:          "no exclusive directive",
			rawDirectives: map[string]string{},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := BatchScript{RawDirectives: tt.rawDirectives}
			got := batch.IsExclusive()
			if got != tt.want {
				t.Errorf("BatchScript.IsExclusive() = %v, want %v", got, tt.want)
			}
		})
	}
}