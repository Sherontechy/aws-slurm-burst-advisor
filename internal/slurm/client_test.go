package slurm

import (
	"testing"
	"time"

	"github.com/scttfrdmn/aws-slurm-burst-advisor/internal/types"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		binPath     string
		expectedPath string
	}{
		{
			name:        "custom bin path",
			binPath:     "/opt/slurm/bin",
			expectedPath: "/opt/slurm/bin",
		},
		{
			name:        "empty bin path uses default",
			binPath:     "",
			expectedPath: defaultSLURMBinPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.binPath)
			if client.binPath != tt.expectedPath {
				t.Errorf("NewClient() binPath = %s, want %s", client.binPath, tt.expectedPath)
			}
			if client.timeout != defaultTimeout {
				t.Errorf("NewClient() timeout = %v, want %v", client.timeout, defaultTimeout)
			}
		})
	}
}

func TestClient_SetTimeout(t *testing.T) {
	client := NewClient("")

	// Test valid timeout
	newTimeout := 60 * time.Second
	client.SetTimeout(newTimeout)
	if client.timeout != newTimeout {
		t.Errorf("SetTimeout() timeout = %v, want %v", client.timeout, newTimeout)
	}

	// Test zero timeout (should not change)
	originalTimeout := client.timeout
	client.SetTimeout(0)
	if client.timeout != originalTimeout {
		t.Errorf("SetTimeout(0) should not change timeout, got %v", client.timeout)
	}

	// Test negative timeout (should not change)
	client.SetTimeout(-10 * time.Second)
	if client.timeout != originalTimeout {
		t.Errorf("SetTimeout(negative) should not change timeout, got %v", client.timeout)
	}
}

func TestClient_parseNodeCount(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name     string
		nodeSpec string
		want     int
		wantErr  bool
	}{
		{
			name:     "simple number",
			nodeSpec: "4",
			want:     4,
			wantErr:  false,
		},
		{
			name:     "range specification",
			nodeSpec: "compute-[001-020]",
			want:     20,
			wantErr:  false,
		},
		{
			name:     "list specification",
			nodeSpec: "node-[01,02,03,04]",
			want:     4,
			wantErr:  false,
		},
		{
			name:     "empty specification",
			nodeSpec: "",
			want:     0,
			wantErr:  true,
		},
		{
			name:     "negative number",
			nodeSpec: "-5",
			want:     0,
			wantErr:  true,
		},
		{
			name:     "invalid range",
			nodeSpec: "node-[020-001]", // end < start
			want:     1,
			wantErr:  false, // Falls back to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.parseNodeCount(tt.nodeSpec)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNodeCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseNodeCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClient_parseSlurmTime(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name    string
		timeStr string
		want    time.Duration
	}{
		{
			name:    "hours:minutes:seconds",
			timeStr: "2:30:45",
			want:    2*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:    "minutes:seconds",
			timeStr: "30:45",
			want:    30*time.Minute + 45*time.Second,
		},
		{
			name:    "just minutes",
			timeStr: "120",
			want:    120 * time.Minute,
		},
		{
			name:    "days-hours:minutes:seconds",
			timeStr: "2-12:30:00",
			want:    2*24*time.Hour + 12*time.Hour + 30*time.Minute,
		},
		{
			name:    "unlimited",
			timeStr: "UNLIMITED",
			want:    0,
		},
		{
			name:    "empty string",
			timeStr: "",
			want:    0,
		},
		{
			name:    "invalid format",
			timeStr: "invalid",
			want:    0,
		},
		{
			name:    "invalid seconds",
			timeStr: "1:90:00", // 90 seconds is invalid
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.parseSlurmTime(tt.timeStr)
			if got != tt.want {
				t.Errorf("parseSlurmTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_parseJobState(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name     string
		stateStr string
		want     types.JobState
	}{
		{
			name:     "running",
			stateStr: "RUNNING",
			want:     types.JobStateRunning,
		},
		{
			name:     "running short",
			stateStr: "R",
			want:     types.JobStateRunning,
		},
		{
			name:     "pending",
			stateStr: "PENDING",
			want:     types.JobStatePending,
		},
		{
			name:     "pending short",
			stateStr: "PD",
			want:     types.JobStatePending,
		},
		{
			name:     "completed",
			stateStr: "COMPLETED",
			want:     types.JobStateCompleted,
		},
		{
			name:     "failed",
			stateStr: "FAILED",
			want:     types.JobStateFailed,
		},
		{
			name:     "cancelled",
			stateStr: "CANCELLED",
			want:     types.JobStateCancelled,
		},
		{
			name:     "unknown state",
			stateStr: "UNKNOWN",
			want:     types.JobStatePending, // Conservative default
		},
		{
			name:     "case insensitive",
			stateStr: "running",
			want:     types.JobStateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.parseJobState(tt.stateStr)
			if got != tt.want {
				t.Errorf("parseJobState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_parseNodeState(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name     string
		stateStr string
		want     types.NodeState
	}{
		{
			name:     "idle",
			stateStr: "idle",
			want:     types.NodeStateIdle,
		},
		{
			name:     "allocated",
			stateStr: "alloc",
			want:     types.NodeStateAllocated,
		},
		{
			name:     "mixed",
			stateStr: "mix",
			want:     types.NodeStateMixed,
		},
		{
			name:     "down",
			stateStr: "down",
			want:     types.NodeStateDown,
		},
		{
			name:     "draining",
			stateStr: "drain",
			want:     types.NodeStateDraining,
		},
		{
			name:     "unknown state defaults to down",
			stateStr: "unknown",
			want:     types.NodeStateDown,
		},
		{
			name:     "case insensitive",
			stateStr: "IDLE",
			want:     types.NodeStateIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.parseNodeState(tt.stateStr)
			if got != tt.want {
				t.Errorf("parseNodeState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_calculateEstimatedWaitTime(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name        string
		pendingJobs []types.JobSummary
		want        time.Duration
	}{
		{
			name:        "no pending jobs",
			pendingJobs: []types.JobSummary{},
			want:        0,
		},
		{
			name: "jobs with time limits",
			pendingJobs: []types.JobSummary{
				{TimeLimit: 2 * time.Hour},
				{TimeLimit: 1 * time.Hour},
			},
			want: time.Duration(float64(90*time.Minute) * 0.7 * 2), // avg 1.5h * 70% efficiency * 2 jobs
		},
		{
			name: "jobs without time limits",
			pendingJobs: []types.JobSummary{
				{TimeLimit: 0},
				{TimeLimit: 0},
			},
			want: 2 * time.Hour, // fallback: 1 hour per job
		},
		{
			name: "mixed jobs",
			pendingJobs: []types.JobSummary{
				{TimeLimit: 2 * time.Hour},
				{TimeLimit: 0}, // No time limit
			},
			want: time.Duration(float64(2*time.Hour) * 0.7 * 2), // Based on job with time limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.calculateEstimatedWaitTime(tt.pendingJobs)
			if got != tt.want {
				t.Errorf("calculateEstimatedWaitTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_parseSlurmTimestamp(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name      string
		timestamp string
		wantZero  bool
	}{
		{
			name:      "ISO format",
			timestamp: "2023-12-01T14:30:00",
			wantZero:  false,
		},
		{
			name:      "date time format",
			timestamp: "2023-12-01 14:30:00",
			wantZero:  false,
		},
		{
			name:      "time only",
			timestamp: "14:30:00",
			wantZero:  false,
		},
		{
			name:      "empty timestamp",
			timestamp: "",
			wantZero:  true,
		},
		{
			name:      "N/A timestamp",
			timestamp: "N/A",
			wantZero:  true,
		},
		{
			name:      "unknown timestamp",
			timestamp: "Unknown",
			wantZero:  true,
		},
		{
			name:      "invalid format",
			timestamp: "invalid-timestamp",
			wantZero:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.parseSlurmTimestamp(tt.timestamp)
			isZero := got.IsZero()
			if isZero != tt.wantZero {
				t.Errorf("parseSlurmTimestamp() zero = %v, wantZero %v", isZero, tt.wantZero)
			}
		})
	}
}

// Benchmark tests
func BenchmarkClient_parseSlurmTime(b *testing.B) {
	client := NewClient("")
	timeStr := "2:30:45"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.parseSlurmTime(timeStr)
	}
}

func BenchmarkClient_parseNodeCount(b *testing.B) {
	client := NewClient("")
	nodeSpec := "compute-[001-100]"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.parseNodeCount(nodeSpec)
	}
}