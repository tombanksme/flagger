package canary

import (
	"context"
	"fmt"
	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	knative "knative.dev/serving/pkg/client/clientset/versioned"
)

type KnativeController struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	knativeClient knative.Interface
}

// IsPrimaryReady checks if the primary revision is ready
func (kc *KnativeController) IsPrimaryReady(cd *flaggerv1.Canary) (bool, error) {
	service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	revisionName, exists := service.Annotations["flagger.app/primary-revision"]
	if !exists {
		return true, fmt.Errorf("service %s.%s primary revision annotation not found", cd.Spec.TargetRef.Name, cd.Namespace)
	}
	revision, err := kc.knativeClient.ServingV1().Revisions(cd.Namespace).Get(context.TODO(), revisionName, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("revision %s.%s get query error: %w", revisionName, cd.Namespace, err)
	}
	if !revision.IsReady() {
		return true, fmt.Errorf("revision %s.%s is not ready", revision.Name, cd.Namespace)
	}
	return true, nil
}

// IsCanaryReady checks if the canary revision is ready
func (kc *KnativeController) IsCanaryReady(cd *flaggerv1.Canary) (bool, error) {
	service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	revision, err := kc.knativeClient.ServingV1().Revisions(cd.Namespace).Get(context.TODO(), service.Status.LatestCreatedRevisionName, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("revision %s.%s get query error: %w", service.Status.LatestCreatedRevisionName, cd.Namespace, err)
	}
	if !revision.IsReady() {
		return true, fmt.Errorf("revision %s.%s is not ready", revision.Name, cd.Namespace)
	}
	return true, nil
}
func (kc *KnativeController) GetMetadata(canary *flaggerv1.Canary) (string, string, map[string]int32, error) {
	// TODO: Do we need this for Knative?
	return "", "", make(map[string]int32), nil
}

// SyncStatus encodes list of revisions and updates the canary status
func (kc *KnativeController) SyncStatus(cd *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return syncCanaryStatus(kc.flaggerClient, cd, status, service.Status.LatestCreatedRevisionName, func(copy *flaggerv1.Canary) {})
}

// SetStatusFailedChecks updates the canary failed checks counter
func (kc *KnativeController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(kc.flaggerClient, cd, val)
}

// SetStatusWeight updates the canary status weight value
func (kc *KnativeController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	return setStatusWeight(kc.flaggerClient, cd, val)
}

// SetStatusIterations updates the canary status iterations value
func (kc *KnativeController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(kc.flaggerClient, cd, val)
}

// SetStatusPhase updates the canary status phase
func (kc *KnativeController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(kc.flaggerClient, cd, phase)
}
func (kc *KnativeController) Initialize(cd *flaggerv1.Canary) (bool, error) {
	if cd.Status.Phase == "" || cd.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return true, fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
		}
		service.Annotations["flagger.app/primary-revision"] = service.Status.LatestCreatedRevisionName
		percent := int64(100)
		traffic := make([]v1.TrafficTarget, 1)
		traffic = append(traffic, v1.TrafficTarget{Percent: &percent, RevisionName: service.Status.LatestCreatedRevisionName})
		service.Spec.Traffic = traffic
		_, err = kc.knativeClient.ServingV1().Services(cd.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
		if err != nil {
			return true, fmt.Errorf("service %s.%s update query error %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
		}
	}
	return true, nil
}
func (kc *KnativeController) Promote(cd *flaggerv1.Canary) error {
	service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	service.Annotations["flagger.app/primary-revision"] = service.Status.LatestCreatedRevisionName
	_, err = kc.knativeClient.ServingV1().Services(cd.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("service %s.%s update query error %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return nil
}
func (kc *KnativeController) HasTargetChanged(cd *flaggerv1.Canary) (bool, error) {
	service, err := kc.knativeClient.ServingV1().Services(cd.Namespace).Get(context.TODO(), cd.Spec.TargetRef.Name, metav1.GetOptions{})
	if err != nil {
		return true, fmt.Errorf("service %s.%s get query error: %w", cd.Spec.TargetRef.Name, cd.Namespace, err)
	}
	return hasSpecChanged(cd, service.Status.LatestCreatedRevisionName)
}
func (kc *KnativeController) HaveDependenciesChanged(canary *flaggerv1.Canary) (bool, error) {
	// TODO: Not sure if we'd need this for Knative deployments
	return false, nil
}
func (kc *KnativeController) ScaleToZero(canary *flaggerv1.Canary) error {
	// Not Implemented: Not needed for Knative deployments
	return nil
}
func (kc *KnativeController) ScaleFromZero(canary *flaggerv1.Canary) error {
	// Not Implemented: Not needed for Knative deployments.
	return nil
}
func (kc *KnativeController) Finalize(canary *flaggerv1.Canary) error {
	return nil
}
