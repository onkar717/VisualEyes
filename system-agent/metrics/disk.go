package metrics

import (
	"runtime"
	"time"

	"github.com/onkar717/visual-eyes/server/models"
	"github.com/shirou/gopsutil/v3/disk"
)

// getRootPath returns the appropriate root path for the current OS
func getRootPath() string {
	if runtime.GOOS == "windows" {
		return "C:\\"
	}
	return "/"
}

// CollectDisk gathers disk metrics
func CollectDisk() ([]models.Metric, error) {
	// Get root partition usage based on OS
	usage, err := disk.Usage(getRootPath())
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	metrics := []models.Metric{
		{
			Name:      "disk.used",
			Value:     float64(usage.Used),
			Tags:      map[string]string{"mountpoint": getRootPath()},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "disk.free",
			Value:     float64(usage.Free),
			Tags:      map[string]string{"mountpoint": getRootPath()},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "disk.total",
			Value:     float64(usage.Total),
			Tags:      map[string]string{"mountpoint": getRootPath()},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "disk.usage_percent",
			Value:     usage.UsedPercent,
			Tags:      map[string]string{"mountpoint": getRootPath()},
			Timestamp: now,
			Unit:      "percent",
		},
	}

	return metrics, nil
}
