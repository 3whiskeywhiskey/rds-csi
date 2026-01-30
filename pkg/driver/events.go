package driver

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

// Event reasons - use consistent naming for filtering
const (
	EventReasonMountFailure       = "MountFailure"
	EventReasonRecoveryFailed     = "RecoveryFailed"
	EventReasonStaleMountDetected = "StaleMountDetected"
)

// EventPoster posts Kubernetes events for mount operations
type EventPoster struct {
	recorder  record.EventRecorder
	clientset kubernetes.Interface
}

// eventSinkAdapter adapts the EventInterface to record.EventSink
// record.EventSink has methods without context, but EventInterface requires context
type eventSinkAdapter struct {
	eventInterface typedcorev1.EventInterface
}

func (a *eventSinkAdapter) Create(event *corev1.Event) (*corev1.Event, error) {
	return a.eventInterface.Create(context.Background(), event, metav1.CreateOptions{})
}

func (a *eventSinkAdapter) Update(event *corev1.Event) (*corev1.Event, error) {
	return a.eventInterface.Update(context.Background(), event, metav1.UpdateOptions{})
}

func (a *eventSinkAdapter) Patch(event *corev1.Event, data []byte) (*corev1.Event, error) {
	return a.eventInterface.Patch(context.Background(), event.Name, types.JSONPatchType, data, metav1.PatchOptions{})
}

// NewEventPoster creates a new EventPoster
// Accepts kubernetes.Interface and creates EventRecorder for posting events to PVCs
func NewEventPoster(clientset kubernetes.Interface) *EventPoster {
	// Create event broadcaster
	broadcaster := record.NewBroadcaster()

	// Start logging events to klog for visibility
	broadcaster.StartLogging(klog.Infof)

	// Start recording events to Kubernetes EventSink
	// Use adapter to convert EventInterface to EventSink (context requirement difference)
	broadcaster.StartRecordingToSink(&eventSinkAdapter{
		eventInterface: clientset.CoreV1().Events(""),
	})

	// Create event recorder with driver component name
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{
		Component: "rds-csi-node",
	})

	return &EventPoster{
		recorder:  recorder,
		clientset: clientset,
	}
}

// PostMountFailure posts a Warning event about a mount failure to the PVC
// Parameters: ctx, pvcNamespace, pvcName, volumeID, nodeName, reason, message
// Message format: "[volumeID] on [nodeName]: [message]"
func (ep *EventPoster) PostMountFailure(ctx context.Context, pvcNamespace, pvcName, volumeID, nodeName, message string) error {
	// Get PVC object to attach event
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		// Don't fail the operation just because event couldn't be posted
		// PVC might be deleted, terminating, or temporarily unavailable
		klog.Warningf("Failed to get PVC %s/%s for event posting: %v", pvcNamespace, pvcName, err)
		return nil
	}

	// Format event message with volume and node context
	eventMessage := fmt.Sprintf("[%s] on [%s]: %s", volumeID, nodeName, message)

	// Post Warning event to PVC
	ep.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonMountFailure, eventMessage)

	klog.V(2).Infof("Posted mount failure event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)

	return nil
}

// PostRecoveryFailed posts a Warning event about a failed recovery attempt to the PVC
// Includes attempt count and final error in message
func (ep *EventPoster) PostRecoveryFailed(ctx context.Context, pvcNamespace, pvcName, volumeID, nodeName string, attemptCount int, finalErr error) error {
	// Get PVC object to attach event
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		// Don't fail the operation just because event couldn't be posted
		klog.Warningf("Failed to get PVC %s/%s for recovery failure event posting: %v", pvcNamespace, pvcName, err)
		return nil
	}

	// Format event message with recovery context
	eventMessage := fmt.Sprintf("[%s] on [%s]: Recovery failed after %d attempts: %v", volumeID, nodeName, attemptCount, finalErr)

	// Post Warning event to PVC
	ep.recorder.Event(pvc, corev1.EventTypeWarning, EventReasonRecoveryFailed, eventMessage)

	klog.V(2).Infof("Posted recovery failure event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)

	return nil
}

// PostStaleMountDetected posts a Normal event about stale mount detection to the PVC
// Normal event type (not Warning) since detection is informational
// Includes old device path and new device path in message
func (ep *EventPoster) PostStaleMountDetected(ctx context.Context, pvcNamespace, pvcName, volumeID, nodeName, oldDevicePath, newDevicePath string) error {
	// Get PVC object to attach event
	pvc, err := ep.clientset.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		// Don't fail the operation just because event couldn't be posted
		klog.Warningf("Failed to get PVC %s/%s for stale mount event posting: %v", pvcNamespace, pvcName, err)
		return nil
	}

	// Format event message with device path info
	eventMessage := fmt.Sprintf("[%s] on [%s]: Stale mount detected - old device: %s, new device: %s", volumeID, nodeName, oldDevicePath, newDevicePath)

	// Post Normal event to PVC (informational, not a failure)
	ep.recorder.Event(pvc, corev1.EventTypeNormal, EventReasonStaleMountDetected, eventMessage)

	klog.V(2).Infof("Posted stale mount detection event to PVC %s/%s: %s", pvcNamespace, pvcName, eventMessage)

	return nil
}
