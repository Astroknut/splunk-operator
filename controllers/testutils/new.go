package testutils

import (
	enterprisev4 "github.com/splunk/splunk-operator/api/v4"
	commonapi "github.com/splunk/splunk-operator/pkg/splunk/common"
	corev1 "k8s.io/api/core/v1"

	//"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pullPolicy = corev1.PullIfNotPresent

// NewStandalone returns new Standalone instance with is config hash
func NewStandalone(name, ns, image string) *enterprisev4.Standalone {

	c := &commonapi.Spec{
		ImagePullPolicy: string(pullPolicy),
		/*
			SchedulerName :
			Affinity :
			Tolerations :
			Resources :
			ServiceTemplate :
		*/
	}

	cs := &enterprisev4.CommonSplunkSpec{
		Mock:    true,
		Spec:    *c,
		Volumes: []corev1.Volume{},
		MonitoringConsoleRef: corev1.ObjectReference{
			Name: "mcName",
		},
		/*
			EtcVolumeStorageConfig :
			VarVolumeStorageConfig :
			Defaults :
			DefaultsURL :
			DefaultsURLApps :
			LicenseURL:
			LicenseMasterRef :
			ClusterMasterRef :
			MonitoringConsoleRef :
			ServiceAccount :
			ExtraEnv :
			ReadinessInitialDelaySeconds :
			LivenessInitialDelaySeconds :
		*/
	}

	ad := &enterprisev4.Standalone{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "enterprise.splunk.com/v4",
			Kind:       "Standalone",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Finalizers: []string{"enterprise.splunk.com/delete-pvc"},
		},
	}

	ad.Spec = enterprisev4.StandaloneSpec{
		CommonSplunkSpec: *cs,
		/*
			Replicas:
			SmartStore:
			AppFrameworkConfig :
		*/
	}
	return ad
}
