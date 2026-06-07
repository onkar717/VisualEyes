package models

import "time"

// ClusterHealth is a registered cluster with its latest health snapshot.
// The k8s-agent upserts this on every metric push via /api/clusters/heartbeat.
type ClusterHealth struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null"     json:"name"`
	Namespace   string    `json:"namespace"` // primary namespace being monitored
	LastSeen    time.Time `gorm:"index"      json:"last_seen"`
	HealthScore float64   `json:"health_score"`

	// Latest pod/node snapshot counts
	TotalNodes    int     `json:"total_nodes"`
	ReadyNodes    int     `json:"ready_nodes"`
	TotalPods     int     `json:"total_pods"`
	RunningPods   int     `json:"running_pods"`
	PendingPods   int     `json:"pending_pods"`
	FailedPods    int     `json:"failed_pods"`
	CrashloopPods int     `json:"crashloop_pods"`
	OpenIncidents int     `json:"open_incidents"`
	// Average cluster-wide resource utilisation (0–100 pct).
	CPUUsagePct float64 `json:"cpu_usage_pct"`
	MemUsagePct float64 `json:"mem_usage_pct"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// ClusterSnapshot is an immutable time-series record of a cluster's health
// at a point in time. Written on every heartbeat so operators can chart
// health-score trends and pod/node counts over time.
type ClusterSnapshot struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ClusterName string    `gorm:"index;not null"           json:"cluster_name"`
	RecordedAt  time.Time `gorm:"index;not null"           json:"recorded_at"`
	HealthScore float64   `json:"health_score"`

	TotalNodes    int     `json:"total_nodes"`
	ReadyNodes    int     `json:"ready_nodes"`
	TotalPods     int     `json:"total_pods"`
	RunningPods   int     `json:"running_pods"`
	PendingPods   int     `json:"pending_pods"`
	FailedPods    int     `json:"failed_pods"`
	CrashloopPods int     `json:"crashloop_pods"`
	OpenIncidents int     `json:"open_incidents"`
	CPUUsagePct   float64 `json:"cpu_usage_pct"`
	MemUsagePct   float64 `json:"mem_usage_pct"`
}

// ComputeHealthScore calculates a 0–100 health score heuristically and stores
// it in HealthScore. Incorporates node readiness, pod health, resource pressure,
// and open incident count   matching reference project scoring formula.
func (s *ClusterSnapshot) ComputeHealthScore() {
	score := 100.0
	if s.TotalNodes > 0 {
		nodeHealth := float64(s.ReadyNodes) / float64(s.TotalNodes) * 100
		score -= (100 - nodeHealth) * 0.4
	}
	if s.TotalPods > 0 {
		podHealth := float64(s.RunningPods) / float64(s.TotalPods) * 100
		score -= (100 - podHealth) * 0.3
	}
	// CPU/Mem pressure   each caps at 15 points deduction.
	if s.CPUUsagePct > 0 {
		score -= min64(s.CPUUsagePct*0.1, 15)
	}
	if s.MemUsagePct > 0 {
		score -= min64(s.MemUsagePct*0.1, 15)
	}
	score -= float64(s.OpenIncidents) * 5
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	s.HealthScore = score
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
