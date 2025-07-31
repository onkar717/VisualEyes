package metrics

import (
	"time"

	"github.com/onkar717/visual-eyes/backend/models"
	"github.com/shirou/gopsutil/v3/net"
)

// calculateNetworkDeltas computes the bytes/second for sent and received data
func calculateNetworkDeltas(prev, curr []net.IOCountersStat) (float64, float64) {
	if len(prev) == 0 || len(curr) == 0 {
		return 0, 0
	}

	// Sum up all network traffic across interfaces
	var prevBytesSent, prevBytesRecv uint64
	var currBytesSent, currBytesRecv uint64

	for _, iface := range prev {
		prevBytesSent += iface.BytesSent
		prevBytesRecv += iface.BytesRecv
	}

	for _, iface := range curr {
		currBytesSent += iface.BytesSent
		currBytesRecv += iface.BytesRecv
	}

	// Calculate deltas (bytes/second)
	bytesSentDelta := float64(currBytesSent - prevBytesSent)
	bytesRecvDelta := float64(currBytesRecv - prevBytesRecv)

	return bytesSentDelta, bytesRecvDelta
}

func CollectNetwork() ([]models.Metric, error) {
	// Get initial counters
	prevCounters, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}

	// Wait for 1 second
	time.Sleep(time.Second)

	// Get counters again
	currCounters, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}

	// Calculate bytes/second
	bytesSentPerSec, bytesRecvPerSec := calculateNetworkDeltas(prevCounters, currCounters)

	// Get current total values
	var totalBytesSent uint64
	var totalBytesRecv uint64
	for _, iface := range currCounters {
		totalBytesSent += iface.BytesSent
		totalBytesRecv += iface.BytesRecv
	}

	now := time.Now().UTC()
	metrics := []models.Metric{
		{
			Name:      "network.bytes_sent",
			Value:     float64(totalBytesSent),
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "network.bytes_recv",
			Value:     float64(totalBytesRecv),
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "bytes",
		},
		{
			Name:      "network.bytes_sent_per_sec",
			Value:     bytesSentPerSec,
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "bytes/sec",
		},
		{
			Name:      "network.bytes_recv_per_sec",
			Value:     bytesRecvPerSec,
			Tags:      map[string]string{"type": "total"},
			Timestamp: now,
			Unit:      "bytes/sec",
		},
	}

	return metrics, nil
}
