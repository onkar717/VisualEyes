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
	TotalNodes    int `json:"total_nodes"`
	ReadyNodes    int `json:"ready_nodes"`
	TotalPods     int `json:"total_pods"`
	RunningPods   int `json:"running_pods"`
	PendingPods   int `json:"pending_pods"`
	FailedPods    int `json:"failed_pods"`
	CrashloopPods int `json:"crashloop_pods"`
	OpenIncidents int `json:"open_incidents"`

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

	TotalNodes    int `json:"total_nodes"`
	ReadyNodes    int `json:"ready_nodes"`
	TotalPods     int `json:"total_pods"`
	RunningPods   int `json:"running_pods"`
	PendingPods   int `json:"pending_pods"`
	FailedPods    int `json:"failed_pods"`
	CrashloopPods int `json:"crashloop_pods"`
	OpenIncidents int `json:"open_incidents"`
}
