package types

// Stats represents the response from Kubelet's /stats/summary endpoint
type Stats struct {
	Node      NodeStats  `json:"node"`
	Pods      []PodStats `json:"pods"`
	Timestamp string     `json:"timestamp"`
}

// NodeStats contains node-level resource usage stats
type NodeStats struct {
	NodeName string      `json:"nodeName"`
	CPU      CPUStats    `json:"cpu"`
	Memory   MemoryStats `json:"memory"`
}

// PodStats contains pod-level resource usage stats
type PodStats struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		UID       string `json:"uid"`
	} `json:"podRef"`
	Containers []ContainerStats `json:"containers"`
	CPU        CPUStats         `json:"cpu"`
	Memory     MemoryStats      `json:"memory"`
}

// ContainerStats contains container-level resource usage stats
type ContainerStats struct {
	Name   string      `json:"name"`
	CPU    CPUStats    `json:"cpu"`
	Memory MemoryStats `json:"memory"`
}

// CPUStats contains CPU usage metrics
type CPUStats struct {
	UsageNanoCores       uint64 `json:"usageNanoCores"`
	UsageCoreNanoSeconds uint64 `json:"usageCoreNanoSeconds"`
}

// MemoryStats contains memory usage metrics
type MemoryStats struct {
	UsageBytes      uint64 `json:"usageBytes"`
	AvailableBytes  uint64 `json:"availableBytes"`
	WorkingSetBytes uint64 `json:"workingSetBytes"`
}
