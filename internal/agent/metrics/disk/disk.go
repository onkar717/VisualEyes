package disk

import (
	"context"
	"runtime"
	"time"

	"github.com/onkar717/visual-eyes/internal/models"
	"github.com/shirou/gopsutil/v3/disk"
)

// CollectDisk collects disk metrics and sends them through the provided channel
func CollectDisk(ctx context.Context, metricsChan chan<- models.Metric) error {
	// Get disk partitions
	partitions, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return err
	}

	now := time.Now()

	// Collect metrics for each partition
	for _, partition := range partitions {
		usage, err := disk.UsageWithContext(ctx, partition.Mountpoint)
		if err != nil {
			continue // Skip this partition if we can't get usage
		}

		tags := map[string]string{
			"device":     partition.Device,
			"mountpoint": partition.Mountpoint,
			"fstype":     partition.Fstype,
		}

		// Send disk space metrics
		metricsChan <- models.Metric{
			Name:      "disk.total",
			Value:     float64(usage.Total),
			Tags:      tags,
			Timestamp: now,
			Unit:      "bytes",
		}

		metricsChan <- models.Metric{
			Name:      "disk.used",
			Value:     float64(usage.Used),
			Tags:      tags,
			Timestamp: now,
			Unit:      "bytes",
		}

		metricsChan <- models.Metric{
			Name:      "disk.free",
			Value:     float64(usage.Free),
			Tags:      tags,
			Timestamp: now,
			Unit:      "bytes",
		}

		metricsChan <- models.Metric{
			Name:      "disk.usage",
			Value:     usage.UsedPercent,
			Tags:      tags,
			Timestamp: now,
			Unit:      "percent",
		}

		// Send inode metrics
		metricsChan <- models.Metric{
			Name:      "disk.inodes.total",
			Value:     float64(usage.InodesTotal),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}

		metricsChan <- models.Metric{
			Name:      "disk.inodes.used",
			Value:     float64(usage.InodesUsed),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}

		metricsChan <- models.Metric{
			Name:      "disk.inodes.free",
			Value:     float64(usage.InodesFree),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}
	}

	// Get disk I/O statistics
	ioStats, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return err
	}

	// Send I/O metrics for each device
	for device, stats := range ioStats {
		tags := map[string]string{"device": device}

		metricsChan <- models.Metric{
			Name:      "disk.io.reads",
			Value:     float64(stats.ReadCount),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.writes",
			Value:     float64(stats.WriteCount),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.read_bytes",
			Value:     float64(stats.ReadBytes),
			Tags:      tags,
			Timestamp: now,
			Unit:      "bytes",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.write_bytes",
			Value:     float64(stats.WriteBytes),
			Tags:      tags,
			Timestamp: now,
			Unit:      "bytes",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.read_time",
			Value:     float64(stats.ReadTime),
			Tags:      tags,
			Timestamp: now,
			Unit:      "milliseconds",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.write_time",
			Value:     float64(stats.WriteTime),
			Tags:      tags,
			Timestamp: now,
			Unit:      "milliseconds",
		}

		metricsChan <- models.Metric{
			Name:      "disk.io.iops_in_progress",
			Value:     float64(stats.IopsInProgress),
			Tags:      tags,
			Timestamp: now,
			Unit:      "count",
		}
	}

	return nil
}

// getRootPath returns the appropriate root path for the current OS
func getRootPath() string {
	if runtime.GOOS == "windows" {
		return "C:\\"
	}
	return "/"
}

func Collect() ([]models.Metric, error) {
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
	}

	return metrics, nil
}
