package load

import (
	"runtime"
	"time"

	"github.com/onkar717/visual-eyes/internal/models"
	"github.com/shirou/gopsutil/v3/load"
)

func Collect() ([]models.Metric, error) {
	// Windows doesn't support load average
	if runtime.GOOS == "windows" {
		return []models.Metric{}, nil
	}

	// Get load average statistics
	loadStat, err := load.Avg()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	metrics := []models.Metric{
		{
			Name:      "load.1min",
			Value:     loadStat.Load1,
			Tags:      map[string]string{"period": "1m"},
			Timestamp: now,
			Unit:      "load",
		},
		{
			Name:      "load.5min",
			Value:     loadStat.Load5,
			Tags:      map[string]string{"period": "5m"},
			Timestamp: now,
			Unit:      "load",
		},
	}

	return metrics, nil
}
