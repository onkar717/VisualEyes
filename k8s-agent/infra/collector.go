// Package infra collects Kubernetes infrastructure-level constraints:
// ResourceQuotas, PVC binding status, and HPA scaling state.
// The RCA infra-diagnosis stage uses this to distinguish application bugs
// from resource/scheduling constraints.
package infra

import (
	"context"
	"log/slog"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ResourceQuotaInfo is a summary of one namespace's ResourceQuota.
type ResourceQuotaInfo struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Hard      map[string]string `json:"hard"`  // resource name → limit
	Used      map[string]string `json:"used"`  // resource name → current usage
}

// PVCInfo summarises one PersistentVolumeClaim.
type PVCInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Phase      string `json:"phase"`       // Pending | Bound | Lost
	VolumeName string `json:"volumeName"`  // empty if unbound
	Capacity   string `json:"capacity"`    // storage request
}

// HPAInfo summarises one HorizontalPodAutoscaler.
type HPAInfo struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	Target          string `json:"target"`          // deployment/replicaset name
	MinReplicas     int32  `json:"minReplicas"`
	MaxReplicas     int32  `json:"maxReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
	DesiredReplicas int32  `json:"desiredReplicas"`
	// AtMaxReplicas is true when currentReplicas == maxReplicas   saturation signal.
	AtMaxReplicas bool   `json:"atMaxReplicas"`
}

// InfraSnapshot bundles all infra-constraint data for one collection cycle.
type InfraSnapshot struct {
	Quotas []ResourceQuotaInfo `json:"quotas"`
	PVCs   []PVCInfo           `json:"pvcs"`
	HPAs   []HPAInfo           `json:"hpas"`
}

// Collect queries the Kubernetes API for quota, PVC, and HPA state across all namespaces.
func Collect(ctx context.Context, client kubernetes.Interface) (*InfraSnapshot, error) {
	snap := &InfraSnapshot{}

	// ResourceQuotas
	quotaList, err := client.CoreV1().ResourceQuotas(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("infra: list resource quotas failed", "error", err)
	} else {
		for _, rq := range quotaList.Items {
			info := ResourceQuotaInfo{
				Namespace: rq.Namespace,
				Name:      rq.Name,
				Hard:      resourceListToMap(rq.Status.Hard),
				Used:      resourceListToMap(rq.Status.Used),
			}
			snap.Quotas = append(snap.Quotas, info)
		}
	}

	// PVCs   only include non-Bound ones (Pending/Lost are actionable).
	pvcList, err := client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("infra: list pvcs failed", "error", err)
	} else {
		for _, pvc := range pvcList.Items {
			capacity := ""
			if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				capacity = req.String()
			}
			snap.PVCs = append(snap.PVCs, PVCInfo{
				Namespace:  pvc.Namespace,
				Name:       pvc.Name,
				Phase:      string(pvc.Status.Phase),
				VolumeName: pvc.Spec.VolumeName,
				Capacity:   capacity,
			})
		}
	}

	// HPAs
	hpaList, err := client.AutoscalingV2().HorizontalPodAutoscalers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Warn("infra: list hpas failed", "error", err)
	} else {
		for _, hpa := range hpaList.Items {
			minReplicas := int32(1)
			if hpa.Spec.MinReplicas != nil {
				minReplicas = *hpa.Spec.MinReplicas
			}
			snap.HPAs = append(snap.HPAs, HPAInfo{
				Namespace:       hpa.Namespace,
				Name:            hpa.Name,
				Target:          targetRef(hpa),
				MinReplicas:     minReplicas,
				MaxReplicas:     hpa.Spec.MaxReplicas,
				CurrentReplicas: hpa.Status.CurrentReplicas,
				DesiredReplicas: hpa.Status.DesiredReplicas,
				AtMaxReplicas:   hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas,
			})
		}
	}

	return snap, nil
}

func resourceListToMap(rl corev1.ResourceList) map[string]string {
	m := make(map[string]string, len(rl))
	for k, v := range rl {
		m[string(k)] = v.String()
	}
	return m
}

func targetRef(hpa autoscalingv2.HorizontalPodAutoscaler) string {
	ref := hpa.Spec.ScaleTargetRef
	return ref.Kind + "/" + ref.Name
}
