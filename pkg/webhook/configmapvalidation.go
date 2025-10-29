/*
Copyright 2025 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tektoncd/pruner/pkg/config"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"
	"knative.dev/pkg/webhook"
	certresources "knative.dev/pkg/webhook/certificates/resources"
)

type ValidateConfigMap struct {
	Client      kubernetes.Interface
	SecretName  string
	WebhookName string
}

var _ webhook.AdmissionController = (*ValidateConfigMap)(nil)
var _ webhook.StatelessAdmissionController = (*ValidateConfigMap)(nil)
var _ controller.Reconciler = (*ValidateConfigMap)(nil)

// ThisTypeDoesNotDependOnInformerState implements StatelessAdmissionController
func (v *ValidateConfigMap) ThisTypeDoesNotDependOnInformerState() {}

// Reconcile updates the ValidatingWebhookConfiguration with CA bundle
func (v *ValidateConfigMap) Reconcile(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)

	// Get the secret containing the CA certificate
	secret, err := v.Client.CoreV1().Secrets(system.Namespace()).Get(ctx, v.SecretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("Secret %s not found yet, skipping webhook configuration update", v.SecretName)
		return nil
	} else if err != nil {
		logger.Errorw("Error fetching secret", "secret", v.SecretName, "error", err)
		return err
	}

	// Get the CA certificate from the secret
	caCert, ok := secret.Data[certresources.CACert]
	if !ok {
		logger.Infof("CA cert not yet present in secret %s", v.SecretName)
		return nil
	}

	// Get the ValidatingWebhookConfiguration
	vwc, err := v.Client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, v.WebhookName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		logger.Infof("ValidatingWebhookConfiguration %s not found yet", v.WebhookName)
		return nil
	} else if err != nil {
		logger.Errorw("Error fetching ValidatingWebhookConfiguration", "name", v.WebhookName, "error", err)
		return err
	}

	// Update the CA bundle in all webhooks
	updated := false
	for i := range vwc.Webhooks {
		if string(vwc.Webhooks[i].ClientConfig.CABundle) != string(caCert) {
			vwc.Webhooks[i].ClientConfig.CABundle = caCert
			updated = true
		}
	}

	if !updated {
		logger.Debug("CA bundle already up to date")
		return nil
	}

	// Update the webhook configuration
	_, err = v.Client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(ctx, vwc, metav1.UpdateOptions{})
	if err != nil {
		logger.Errorw("Error updating ValidatingWebhookConfiguration", "name", v.WebhookName, "error", err)
		return err
	}

	logger.Infow("Successfully updated ValidatingWebhookConfiguration with CA bundle", "name", v.WebhookName)
	return nil
}

func (v *ValidateConfigMap) Path() string {
	return "/validate-configmap"
}

// Admit handles the admission request
func (v *ValidateConfigMap) Admit(ctx context.Context, request *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	logger := logging.FromContext(ctx)

	// Only validate ConfigMaps
	if request.Kind.Kind != "ConfigMap" {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// Parse the ConfigMap
	var cm corev1.ConfigMap
	if err := json.Unmarshal(request.Object.Raw, &cm); err != nil {
		logger.Warnw("Failed to unmarshal ConfigMap", "error", err)
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// if this cm contains any pruner configuration
	hasPrunerConfig := false
	if cm.Data != nil {
		if _, hasGlobal := cm.Data[config.PrunerGlobalConfigKey]; hasGlobal {
			hasPrunerConfig = true
		}
		if _, hasNamespace := cm.Data[config.PrunerNamespaceConfigKey]; hasNamespace {
			hasPrunerConfig = true
		}
	}

	if !hasPrunerConfig {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	logger.Infow("Validating pruner ConfigMap",
		"name", cm.Name,
		"namespace", cm.Namespace,
		"operation", request.Operation,
		"hasGlobalConfig", cm.Data[config.PrunerGlobalConfigKey] != "",
		"hasNamespaceConfig", cm.Data[config.PrunerNamespaceConfigKey] != "")

	// Validate using the centralized validation function
	if err := config.ValidateConfigMap(&cm); err != nil {
		logger.Errorw("ConfigMap validation failed", "name", cm.Name, "error", err)
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("Invalid pruner configuration: %v", err),
				Reason:  metav1.StatusReasonInvalid,
				Code:    422,
			},
		}
	}

	logger.Infow("ConfigMap validation successful", "name", cm.Name)
	return &admissionv1.AdmissionResponse{Allowed: true}
}
