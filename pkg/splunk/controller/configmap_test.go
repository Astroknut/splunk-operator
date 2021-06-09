// Copyright (c) 2018-2021 Splunk Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	spltest "github.com/splunk/splunk-operator/pkg/splunk/test"
	splutil "github.com/splunk/splunk-operator/pkg/splunk/util"
)

func TestApplyConfigMap(t *testing.T) {
	funcCalls := []spltest.MockFuncCall{{MetaName: "*v1.ConfigMap-test-defaults"}}
	createCalls := map[string][]spltest.MockFuncCall{"Get": funcCalls, "Create": funcCalls}
	updateCalls := map[string][]spltest.MockFuncCall{"Get": funcCalls, "Update": funcCalls}
	current := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "defaults",
			Namespace: "test",
		},
	}
	revised := current.DeepCopy()
	revised.Data = map[string]string{"a": "b"}
	reconcile := func(c *spltest.MockClient, cr interface{}) error {
		_, err := ApplyConfigMap(c, cr.(*corev1.ConfigMap))
		return err
	}
	spltest.ReconcileTester(t, "TestApplyConfigMap", &current, revised, createCalls, updateCalls, reconcile, false)
}

func TestGetConfigMap(t *testing.T) {
	current := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "defaults",
			Namespace: "test",
		},
	}

	client := spltest.NewMockClient()
	namespacedName := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}

	_, err := GetConfigMap(client, namespacedName)
	if err == nil {
		t.Errorf("Should return an error, when the configMap doesn't exist")
	}

	_, err = ApplyConfigMap(client, &current)
	if err != nil {
		t.Errorf("Failed to create the configMap. Error: %s", err.Error())
	}

	_, err = GetConfigMap(client, namespacedName)
	if err != nil {
		t.Errorf("Should not return an error, when the configMap exists")
	}
}

func TestGetConfigMapResourceVersion(t *testing.T) {
	current := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "defaults",
			Namespace: "test",
		},
	}

	client := spltest.NewMockClient()
	namespacedName := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}

	_, err := GetConfigMap(client, namespacedName)
	if err == nil {
		t.Errorf("Should return an error, when the configMap doesn't exist")
	}

	_, err = GetConfigMapResourceVersion(client, namespacedName)
	if err == nil {
		t.Errorf("Should return an error, when the configMap doesn't exist")
	}

	_, err = ApplyConfigMap(client, &current)
	if err != nil {
		t.Errorf("Failed to create the configMap. Error: %s", err.Error())
	}

	_, err = GetConfigMapResourceVersion(client, namespacedName)
	if err != nil {
		t.Errorf("Should not return an error, when the configMap exists")
	}
}

func TestGetMCConfigMap(t *testing.T) {
	current := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "defaults",
			Namespace: "test",
		},
	}

	client := spltest.NewMockClient()
	namespacedName := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}

	_, err := GetMCConfigMap(client, namespacedName)
	if err != nil {
		t.Errorf("Should never return an error as it should have created a empty configmap")
	}

	_, err = ApplyConfigMap(client, &current)
	if err != nil {
		t.Errorf("Failed to create the configMap. Error: %s", err.Error())
	}

	_, err = GetMCConfigMap(client, namespacedName)
	if err != nil {
		t.Errorf("Should not return an error, when the configMap exists")
	}
}

func TestSetConfigMapOwnerRef(t *testing.T) {
	current := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-test-monitoring-console",
			Namespace: "test",
		},
	}

	c := spltest.NewMockClient()
	cr := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-test-monitoring-console",
			Namespace: "test",
		},
	}
	namespacedName := types.NamespacedName{Namespace: "test", Name: "splunk-test-monitoring-console"}

	err := SetConfigMapOwnerRef(c, &cr, namespacedName)
	if err.Error() != "NotFound" {
		t.Errorf("Couldn't detect resource %s", current.GetName())
	}

	// Create statefulset
	err = splutil.CreateResource(c, &cr)
	if err != nil {
		t.Errorf("Failed to create owner reference  %s", current.GetName())
	}

	//create configmap
	_, err = ApplyConfigMap(c, &current)
	if err != nil {
		t.Errorf("Failed to create the configMap. Error: %s", err.Error())
	}

	// Test existing owner reference
	err = SetConfigMapOwnerRef(c, &cr, namespacedName)
	if err != nil {
		t.Errorf("Couldn't set owner ref for resource %s", current.GetName())
	}

	// Try adding same owner again
	err = SetConfigMapOwnerRef(c, &cr, namespacedName)
	if err != nil {
		t.Errorf("Couldn't set owner ref for statefulset %s", current.GetName())
	}
}
