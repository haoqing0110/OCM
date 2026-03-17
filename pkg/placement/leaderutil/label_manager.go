package leaderutil

import (
	"context"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// LeaderLabelKey is the label key to identify the leader pod
	LeaderLabelKey = "cluster.open-cluster-management.io/leader"
	// LeaderLabelValue is the label value for the leader pod
	LeaderLabelValue = "true"
)

// LeaderLabelManager manages the leader label on the pod
type LeaderLabelManager struct {
	kubeClient kubernetes.Interface
	namespace  string
	podName    string
}

// NewLeaderLabelManager creates a new LeaderLabelManager
func NewLeaderLabelManager(kubeClient kubernetes.Interface) *LeaderLabelManager {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		klog.Warning("POD_NAMESPACE not set, using default namespace")
		namespace = "open-cluster-management"
	}
	podName := os.Getenv("POD_NAME")

	return &LeaderLabelManager{
		kubeClient: kubeClient,
		namespace:  namespace,
		podName:    podName,
	}
}

// MarkAsLeader adds the leader label to the current pod
func (m *LeaderLabelManager) MarkAsLeader(ctx context.Context) error {
	return m.setLeaderLabel(ctx, true)
}

// UnmarkAsLeader removes the leader label from the current pod
func (m *LeaderLabelManager) UnmarkAsLeader(ctx context.Context) error {
	return m.setLeaderLabel(ctx, false)
}

func (m *LeaderLabelManager) setLeaderLabel(ctx context.Context, isLeader bool) error {
	if m.podName == "" {
		klog.Warning("POD_NAME not set, cannot update leader label")
		return nil
	}

	pod, err := m.kubeClient.CoreV1().Pods(m.namespace).Get(ctx, m.podName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get pod %s/%s: %v", m.namespace, m.podName, err)
		return err
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	labelChanged := false
	if isLeader {
		if pod.Labels[LeaderLabelKey] != LeaderLabelValue {
			pod.Labels[LeaderLabelKey] = LeaderLabelValue
			labelChanged = true
			klog.Infof("Adding leader label to pod %s/%s", m.namespace, m.podName)
		}
	} else {
		if _, exists := pod.Labels[LeaderLabelKey]; exists {
			delete(pod.Labels, LeaderLabelKey)
			labelChanged = true
			klog.Infof("Removing leader label from pod %s/%s", m.namespace, m.podName)
		}
	}

	if !labelChanged {
		klog.V(4).Infof("Leader label already in desired state for pod %s/%s", m.namespace, m.podName)
		return nil
	}

	_, err = m.kubeClient.CoreV1().Pods(m.namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update pod %s/%s: %v", m.namespace, m.podName, err)
		return err
	}

	klog.Infof("Successfully updated leader label on pod %s/%s (isLeader=%v)", m.namespace, m.podName, isLeader)
	return nil
}
