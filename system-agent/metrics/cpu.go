package metrics

import (
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/shirou/gopsutil/v3/cpu"
)

// calculateDelta computes the difference between two CPU times and returns total and idle percentages
func calculateDelta(prev, curr cpu.TimesStat) (float64, float64) {
	// Calculate deltas for each metric
	userDelta := curr.User - prev.User
	systemDelta := curr.System - prev.System
	idleDelta := curr.Idle - prev.Idle
	iowaitDelta := curr.Iowait - prev.Iowait
	irqDelta := curr.Irq - prev.Irq
	softirqDelta := curr.Softirq - prev.Softirq
	stealDelta := curr.Steal - prev.Steal

	// Total time delta
	totalDelta := userDelta + systemDelta + idleDelta + iowaitDelta + irqDelta + softirqDelta + stealDelta

	if totalDelta == 0 {
		return 0.0, 100.0
	}

	// Calculate percentages
	idlePercent := (idleDelta / totalDelta) * 100.0
	usagePercent := 100.0 - idlePercent

	return usagePercent, idlePercent
}

// CollectCPU gathers CPU metrics
func CollectCPU() ([]models.Metric, error) {
	// Get initial CPU times
	prevTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}

	// Wait for 1 second
	time.Sleep(time.Second)

	// Get CPU times again
	currTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}

	// Calculate usage and idle percentages
	usagePercent, idlePercent := calculateDelta(prevTimes[0], currTimes[0])

	now := time.Now().UTC()
	metrics := []models.Metric{
		{
			Name:      "cpu.usage",
			Value:     usagePercent,
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "percent",
		},
		{
			Name:      "cpu.idle",
			Value:     idlePercent,
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "percent",
		},
	}

	return metrics, nil
}
