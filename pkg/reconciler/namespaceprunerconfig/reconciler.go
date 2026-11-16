package namespaceprunerconfig

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/tektoncd/pruner/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

// Reconciler implements controller.Reconciler for NamespacePrunerConfig resources
type Reconciler struct {
	kubeclient kubernetes.Interface

	// onConfigChange is called with the namespace when its pruner config was
	// created, changed or deleted, so the new limits are applied right away
	// instead of on the next completed run. Optional.
	onConfigChange func(namespace string)
	// startTime is when the controller started. ConfigMaps that already
	// existed then are covered by the startup garbage collection.
	startTime time.Time

	mu sync.Mutex
	// seen holds the last handled ResourceVersion per namespace
	seen map[string]string
}

// configChanged records the ConfigMap state for the namespace and reports
// whether it differs from what was handled before. A nil cm means the
// ConfigMap is gone.
func (r *Reconciler) configChanged(namespace string, cm *corev1.ConfigMap) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = map[string]string{}
	}

	last, known := r.seen[namespace]
	if cm == nil {
		delete(r.seen, namespace)
		return known
	}
	r.seen[namespace] = cm.ResourceVersion
	if !known {
		// first time we see it: only new ConfigMaps count as a change
		return cm.CreationTimestamp.Time.After(r.startTime)
	}
	return last != cm.ResourceVersion
}

func (r *Reconciler) notifyConfigChange(namespace string, cm *corev1.ConfigMap) {
	if r.onConfigChange == nil || !r.configChanged(namespace, cm) {
		return
	}
	r.onConfigChange(namespace)
}

// Reconcile is called when a ConfigMap with name "tekton-pruner-namespace-spec" changes
func (r *Reconciler) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)
	logger.Debugf("Reconciling namespace pruner config: %s", key)

	// Parse the key to get namespace and name
	namespace, name, err := parseKey(key)
	if err != nil {
		logger.Errorf("Invalid key: %s", key)
		return err
	}

	// Get the ConfigMap
	cm, err := r.kubeclient.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// If the ConfigMap is not found, it was deleted
		if errors.IsNotFound(err) {
			logger.Infof("Namespace ConfigMap deleted: %s/%s", namespace, name)
			config.PrunerConfigStore.DeleteNamespaceConfig(ctx, namespace)
			r.notifyConfigChange(namespace, nil)
			return nil
		}
		logger.Errorf("Failed to get ConfigMap %s/%s: %v", namespace, name, err)
		return err
	}

	// Load the namespace config
	logger.Infof("Loading namespace config from ConfigMap: %s/%s", namespace, name)
	if err := config.PrunerConfigStore.LoadNamespaceConfig(ctx, namespace, cm); err != nil {
		logger.Errorf("Failed to load namespace config from ConfigMap %s/%s: %v", namespace, name, err)
		return err
	}

	logger.Infof("Successfully loaded namespace config: %s/%s", namespace, name)
	r.notifyConfigChange(namespace, cm)
	return nil
}

// parseKey parses the key in the format "namespace/name" and returns namespace and name
func parseKey(key string) (namespace, name string, err error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return "", "", controller.NewPermanentError(nil)
	}
	return parts[0], parts[1], nil
}
