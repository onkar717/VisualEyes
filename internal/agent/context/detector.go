package context

import (
	"os"
	"strings"
)

// ContextInfo holds information about the agent's runtime context
type ContextInfo struct {
	IsKubernetes             bool
	IsRunningInsideContainer bool
}

// Detect determines the runtime context of the agent
func Detect() *ContextInfo {
	return &ContextInfo{
		IsKubernetes:             isKubernetes(),
		IsRunningInsideContainer: isContainer(),
	}
}

// isKubernetes checks if we're running in a Kubernetes cluster
func isKubernetes() bool {
	// Check for Kubernetes service account token
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err == nil {
		return true
	}

	// Check for Kubernetes environment variables
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	return false
}

// isContainer checks if we're running inside a container
func isContainer() bool {
	// Check for container environment
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup for container indicators
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		content := string(data)
		return strings.Contains(content, "docker") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "containerd")
	}

	return false
}
