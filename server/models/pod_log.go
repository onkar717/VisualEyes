package models

import "time"

// PodLog represents a single log line emitted by a container inside a pod.
type PodLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Pod       string    `gorm:"index;not null"          json:"pod"`
	Namespace string    `gorm:"index;not null"          json:"namespace"`
	Container string    `gorm:"index"                   json:"container"`
	Node      string    `gorm:"index"                   json:"node"`
	Stream    string    `json:"stream"` // stdout | stderr
	Line      string    `gorm:"type:text;not null"      json:"line"`
	Timestamp time.Time `gorm:"index;not null"          json:"timestamp"`
}
