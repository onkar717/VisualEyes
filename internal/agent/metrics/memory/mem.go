package memory

import (
	"time"

	"github.com/onkar717/visual-eyes/internal/models"
	"github.com/shirou/gopsutil/v3/mem"
)

func Collect() ([]models.Metric, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// Calculate actual used memory (excluding buffers and cache)
	actualUsed := v.Total - v.Available // v.Available accounts for buffers and cache

	metrics := []models.Metric{
		{
			Name:      "memory.total",
			Value:     float64(v.Total),
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "memory.used",
			Value:     float64(actualUsed),
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "memory.free",
			Value:     float64(v.Available), // Using Available instead of Free
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "memory.buffers",
			Value:     float64(v.Buffers),
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "memory.cached",
			Value:     float64(v.Cached),
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "memory.usage_percent",
			Value:     v.UsedPercent,
			Tags:      map[string]string{"type": "virtual"},
			Timestamp: now,
			Unit:      "percent",
		},
	}

	return metrics, nil
}
