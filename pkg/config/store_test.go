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

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestLoadGlobalConfig verifies global configuration loading from ConfigMap.
func TestLoadGlobalConfig(t *testing.T) {
	tests := []struct {
		name        string
		configData  string
		expectError bool
		validateTTL *int32
	}{
		{
			name: "Valid config with TTL",
			configData: `
ttlSecondsAfterFinished: 3600
successfulHistoryLimit: 5
failedHistoryLimit: 3`,
			expectError: false,
			validateTTL: intPtr(int32(3600)),
		},
		{
			name:        "Empty config",
			configData:  "",
			expectError: false,
			validateTTL: nil,
		},
		{
			name: "Config with namespaces",
			configData: `
ttlSecondsAfterFinished: 3600
namespaces:
  test-ns:
    ttlSecondsAfterFinished: 7200`,
			expectError: false,
			validateTTL: intPtr(int32(3600)),
		},
		{
			name:        "Invalid YAML",
			configData:  "ttlSecondsAfterFinished: invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						LabelPartOf:     LabelPartOfValue,
						LabelConfigType: LabelConfigTypeGlobal,
					},
				},
				Data: map[string]string{PrunerGlobalConfigKey: tt.configData},
			}

			err := ps.LoadGlobalConfig(context.Background(), cm)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.validateTTL, ps.globalConfig.TTLSecondsAfterFinished)
			}
		})
	}
}

// TestLoadNamespaceConfig verifies namespace-specific configuration loading.
func TestLoadNamespaceConfig(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		configData  string
		expectError bool
	}{
		{
			name:      "Valid namespace config",
			namespace: "test-ns",
			configData: `
ttlSecondsAfterFinished: 1800
successfulHistoryLimit: 3`,
			expectError: false,
		},
		{
			name:      "Config with PipelineRun selectors",
			namespace: "test-ns",
			configData: `
pipelineRuns:
  - name: my-pipeline
    ttlSecondsAfterFinished: 7200`,
			expectError: false,
		},
		{
			name:      "Config with TaskRun selectors",
			namespace: "test-ns",
			configData: `
taskRuns:
  - selector:
      - matchLabels:
          app: myapp`,
			expectError: false,
		},
		{
			name:        "Invalid YAML",
			namespace:   "test-ns",
			configData:  "invalid: yaml: ::::",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: tt.namespace,
					Labels: map[string]string{
						LabelPartOf:     LabelPartOfValue,
						LabelConfigType: LabelConfigTypeNamespace,
					},
				},
				Data: map[string]string{PrunerNamespaceConfigKey: tt.configData},
			}

			err := ps.LoadNamespaceConfig(context.Background(), tt.namespace, cm)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				_, exists := ps.namespaceConfig[tt.namespace]
				assert.True(t, exists)
			}
		})
	}
}

// TestLoadConfigRejectsUnlabeledConfigMaps verifies that ConfigMaps without required labels are rejected.
// This is a regression test for the vulnerability where unlabeled ConfigMaps bypassed webhook validation.
func TestLoadConfigRejectsUnlabeledConfigMaps(t *testing.T) {
	tests := []struct {
		name           string
		configMapName  string
		namespace      string
		labels         map[string]string
		loadFunc       string // "global" or "namespace"
		expectError    bool
		errorSubstring string
	}{
		{
			name:           "Namespace config without any labels",
			configMapName:  PrunerNamespaceConfigMapName,
			namespace:      "test-ns",
			labels:         nil,
			loadFunc:       "namespace",
			expectError:    true,
			errorSubstring: "ConfigMap must have labels",
		},
		{
			name:          "Namespace config without part-of label",
			configMapName: PrunerNamespaceConfigMapName,
			namespace:     "test-ns",
			labels: map[string]string{
				LabelConfigType: LabelConfigTypeNamespace,
			},
			loadFunc:       "namespace",
			expectError:    true,
			errorSubstring: "must have label app.kubernetes.io/part-of",
		},
		{
			name:          "Namespace config without config-type label",
			configMapName: PrunerNamespaceConfigMapName,
			namespace:     "test-ns",
			labels: map[string]string{
				LabelPartOf: LabelPartOfValue,
			},
			loadFunc:       "namespace",
			expectError:    true,
			errorSubstring: "must have label pruner.tekton.dev/config-type",
		},
		{
			name:          "Namespace config with wrong config-type label",
			configMapName: PrunerNamespaceConfigMapName,
			namespace:     "test-ns",
			labels: map[string]string{
				LabelPartOf:     LabelPartOfValue,
				LabelConfigType: LabelConfigTypeGlobal, // Wrong type!
			},
			loadFunc:       "namespace",
			expectError:    true,
			errorSubstring: "expected 'namespace', got 'global'",
		},
		{
			name:           "Global config without any labels",
			configMapName:  PrunerConfigMapName,
			namespace:      "tekton-pipelines",
			labels:         nil,
			loadFunc:       "global",
			expectError:    true,
			errorSubstring: "ConfigMap must have labels",
		},
		{
			name:          "Global config with wrong config-type label",
			configMapName: PrunerConfigMapName,
			namespace:     "tekton-pipelines",
			labels: map[string]string{
				LabelPartOf:     LabelPartOfValue,
				LabelConfigType: LabelConfigTypeNamespace, // Wrong type!
			},
			loadFunc:       "global",
			expectError:    true,
			errorSubstring: "expected 'global', got 'namespace'",
		},
		{
			name:          "Namespace config with correct labels succeeds",
			configMapName: PrunerNamespaceConfigMapName,
			namespace:     "test-ns",
			labels: map[string]string{
				LabelPartOf:     LabelPartOfValue,
				LabelConfigType: LabelConfigTypeNamespace,
			},
			loadFunc:    "namespace",
			expectError: false,
		},
		{
			name:          "Global config with correct labels succeeds",
			configMapName: PrunerConfigMapName,
			namespace:     "tekton-pipelines",
			labels: map[string]string{
				LabelPartOf:     LabelPartOfValue,
				LabelConfigType: LabelConfigTypeGlobal,
			},
			loadFunc:    "global",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.configMapName,
					Namespace: tt.namespace,
					Labels:    tt.labels,
				},
				Data: map[string]string{},
			}

			var err error
			if tt.loadFunc == "global" {
				cm.Data[PrunerGlobalConfigKey] = "ttlSecondsAfterFinished: 3600"
				err = ps.LoadGlobalConfig(context.Background(), cm)
			} else {
				cm.Data[PrunerNamespaceConfigKey] = "ttlSecondsAfterFinished: 1800"
				err = ps.LoadNamespaceConfig(context.Background(), tt.namespace, cm)
			}

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorSubstring != "" {
					assert.Contains(t, err.Error(), tt.errorSubstring)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteNamespaceConfig verifies namespace configuration deletion.
func TestDeleteNamespaceConfig(t *testing.T) {
	ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
	ttl := int32(3600)
	ps.namespaceConfig["ns1"] = NamespaceSpec{PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl}}
	ps.namespaceConfig["ns2"] = NamespaceSpec{PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl}}

	ps.DeleteNamespaceConfig(context.Background(), "ns1")

	assert.Len(t, ps.namespaceConfig, 1)
	_, exists := ps.namespaceConfig["ns1"]
	assert.False(t, exists)
	_, exists = ps.namespaceConfig["ns2"]
	assert.True(t, exists)
}

// TestGetPipelineConfig verifies pipeline config retrieval with fallback hierarchy.
func TestGetPipelineConfig(t *testing.T) {
	ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
	globalTTL := int32(3600)
	globalLimit := int32(5)
	ps.globalConfig = GlobalConfig{
		PrunerConfig: PrunerConfig{
			TTLSecondsAfterFinished: &globalTTL,
			SuccessfulHistoryLimit:  &globalLimit,
		},
	}

	ttl, source := ps.GetPipelineTTLSecondsAfterFinished("test-ns", "", SelectorSpec{})
	assert.NotNil(t, ttl)
	assert.NotEmpty(t, source)
	assert.Equal(t, globalTTL, *ttl)

	limit, source := ps.GetPipelineSuccessHistoryLimitCount("test-ns", "", SelectorSpec{})
	assert.NotNil(t, limit)
	assert.NotEmpty(t, source)
	assert.Equal(t, globalLimit, *limit)
}

// TestGetTaskConfig verifies task config retrieval with fallback hierarchy.
func TestGetTaskConfig(t *testing.T) {
	ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
	globalTTL := int32(1800)
	globalLimit := int32(10)
	ps.globalConfig = GlobalConfig{
		PrunerConfig: PrunerConfig{
			TTLSecondsAfterFinished: &globalTTL,
			SuccessfulHistoryLimit:  &globalLimit,
		},
	}

	ttl, source := ps.GetTaskTTLSecondsAfterFinished("test-ns", "", SelectorSpec{})
	assert.NotNil(t, ttl)
	assert.NotEmpty(t, source)
	assert.Equal(t, globalTTL, *ttl)

	limit, source := ps.GetTaskSuccessHistoryLimitCount("test-ns", "", SelectorSpec{})
	assert.NotNil(t, limit)
	assert.NotEmpty(t, source)
	assert.Equal(t, globalLimit, *limit)
}

// TestConcurrentAccess verifies thread-safe config access via mutex.
func TestConcurrentAccess(t *testing.T) {
	ps := &prunerConfigStore{namespaceConfig: make(map[string]NamespaceSpec)}
	ttl := int32(3600)
	ps.globalConfig = GlobalConfig{PrunerConfig: PrunerConfig{TTLSecondsAfterFinished: &ttl}}

	done := make(chan bool, 2)

	// Concurrent writer
	go func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
			Data:       map[string]string{PrunerGlobalConfigKey: "ttlSecondsAfterFinished: 7200"},
		}
		for i := 0; i < 50; i++ {
			_ = ps.LoadGlobalConfig(context.Background(), cm)
		}
		done <- true
	}()

	// Concurrent reader
	go func() {
		for i := 0; i < 50; i++ {
			_, _ = ps.GetPipelineTTLSecondsAfterFinished("test-ns", "", SelectorSpec{})
		}
		done <- true
	}()

	<-done
	<-done
}

func intPtr(i int32) *int32 { return &i }
