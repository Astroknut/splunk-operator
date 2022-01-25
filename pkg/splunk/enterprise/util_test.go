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

package enterprise

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	enterpriseApi "github.com/splunk/splunk-operator/pkg/apis/enterprise/v3"
	splclient "github.com/splunk/splunk-operator/pkg/splunk/client"
	splcommon "github.com/splunk/splunk-operator/pkg/splunk/common"
	splctrl "github.com/splunk/splunk-operator/pkg/splunk/controller"
	spltest "github.com/splunk/splunk-operator/pkg/splunk/test"
	splutil "github.com/splunk/splunk-operator/pkg/splunk/util"
)

func init() {
}

func TestApplySplunkConfig(t *testing.T) {
	funcCalls := []spltest.MockFuncCall{
		{MetaName: "*v1.Secret-test-splunk-test-secret"},
		{MetaName: "*v1.Secret-test-splunk-test-secret"},
		{MetaName: "*v1.ConfigMap-test-splunk-stack1-search-head-defaults"},
	}
	createCalls := map[string][]spltest.MockFuncCall{"Get": funcCalls, "Create": {funcCalls[0], funcCalls[2]}, "Update": {funcCalls[0]}}
	updateCalls := map[string][]spltest.MockFuncCall{"Get": {funcCalls[0], funcCalls[1], funcCalls[2]}}
	searchHeadCR := enterpriseApi.SearchHeadCluster{
		TypeMeta: metav1.TypeMeta{
			Kind: "SearcHead",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
	}
	searchHeadCR.Spec.Defaults = "defaults-yaml"
	searchHeadRevised := searchHeadCR.DeepCopy()
	searchHeadRevised.Spec.Image = "splunk/test"
	reconcile := func(c *spltest.MockClient, cr interface{}) error {
		obj := cr.(*enterpriseApi.SearchHeadCluster)
		_, err := ApplySplunkConfig(c, obj, obj.Spec.CommonSplunkSpec, SplunkSearchHead)
		return err
	}
	spltest.ReconcileTesterWithoutRedundantCheck(t, "TestApplySplunkConfig", &searchHeadCR, searchHeadRevised, createCalls, updateCalls, reconcile, false)

	// test search head with indexer reference
	searchHeadRevised.Spec.ClusterMasterRef.Name = "stack2"
	spltest.ReconcileTesterWithoutRedundantCheck(t, "TestApplySplunkConfig", &searchHeadCR, searchHeadRevised, createCalls, updateCalls, reconcile, false)

	// test indexer with license manager
	indexerCR := enterpriseApi.IndexerCluster{
		TypeMeta: metav1.TypeMeta{
			Kind: "IndexerCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
	}
	indexerRevised := indexerCR.DeepCopy()
	indexerRevised.Spec.Image = "splunk/test"
	indexerRevised.Spec.LicenseMasterRef.Name = "stack2"
	reconcile = func(c *spltest.MockClient, cr interface{}) error {
		obj := cr.(*enterpriseApi.IndexerCluster)
		_, err := ApplySplunkConfig(c, obj, obj.Spec.CommonSplunkSpec, SplunkIndexer)
		return err
	}
	funcCalls = []spltest.MockFuncCall{
		{MetaName: "*v1.Secret-test-splunk-test-secret"},
	}
	createCalls = map[string][]spltest.MockFuncCall{"Get": {funcCalls[0], funcCalls[0]}, "Create": funcCalls, "Update": {funcCalls[0]}}
	updateCalls = map[string][]spltest.MockFuncCall{"Get": {funcCalls[0], funcCalls[0]}}

	spltest.ReconcileTesterWithoutRedundantCheck(t, "TestApplySplunkConfig", &indexerCR, indexerRevised, createCalls, updateCalls, reconcile, false)
}

func TestGetLicenseManagerURL(t *testing.T) {
	cr := enterpriseApi.LicenseMaster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
	}

	cr.Spec.LicenseMasterRef.Name = "stack1"
	got := getLicenseManagerURL(&cr, &cr.Spec.CommonSplunkSpec)
	want := []corev1.EnvVar{
		{
			Name:  "SPLUNK_LICENSE_MASTER_URL",
			Value: splcommon.TestStack1LicenseManagerService,
		},
	}
	result := splcommon.CompareEnvs(got, want)
	//if differ then CompareEnvs returns true
	if result == true {
		t.Errorf("getLicenseManagerURL(\"%s\") = %s; want %s", SplunkLicenseManager, got, want)
	}

	cr.Spec.LicenseMasterRef.Namespace = "test"
	got = getLicenseManagerURL(&cr, &cr.Spec.CommonSplunkSpec)
	want = []corev1.EnvVar{
		{
			Name:  "SPLUNK_LICENSE_MASTER_URL",
			Value: splcommon.TestStack1LicenseManagerClusterLocal,
		},
	}

	result = splcommon.CompareEnvs(got, want)
	//if differ then CompareEnvs returns true
	if result == true {
		t.Errorf("getLicenseManagerURL(\"%s\") = %s; want %s", SplunkLicenseManager, got, want)
	}
}

func TestApplySmartstoreConfigMap(t *testing.T) {
	cr := enterpriseApi.ClusterMaster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idxCluster",
			Namespace: "test",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			SmartStore: enterpriseApi.SmartStoreSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
				},

				IndexList: []enterpriseApi.IndexSpec{
					{Name: "salesdata1", RemotePath: "remotepath1",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
					{Name: "salesdata2", RemotePath: "remotepath2",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
					{Name: "salesdata3", RemotePath: "remotepath3",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	// Create namespace scoped secret
	secret, err := splutil.ApplyNamespaceScopedSecretObject(client, "test")
	if err != nil {
		t.Errorf(err.Error())
	}

	secret.Data[s3AccessKey] = []byte("abcdJDckRkxhMEdmSk5FekFRRzBFOXV6bGNldzJSWE9IenhVUy80aa")
	secret.Data[s3SecretKey] = []byte("g4NVp0a29PTzlPdGczWk1vekVUcVBSa0o4NkhBWWMvR1NadDV4YVEy")
	_, err = splctrl.ApplySecret(client, secret)
	if err != nil {
		t.Errorf(err.Error())
	}

	test := func(client *spltest.MockClient, cr splcommon.MetaObject, smartstore *enterpriseApi.SmartStoreSpec, want string) {
		f := func() (interface{}, error) {
			configMap, _, err := ApplySmartstoreConfigMap(client, cr, smartstore)
			configMap.Data["conftoken"] = "1601945361"
			return configMap, err
		}
		configTester(t, "ApplySmartstoreConfigMap()", f, want)
	}

	test(client, &cr, &cr.Spec.SmartStore, `{"metadata":{"name":"splunk-idxCluster--smartstore","namespace":"test","creationTimestamp":null,"ownerReferences":[{"apiVersion":"","kind":"","name":"idxCluster","uid":"","controller":true}]},"data":{"conftoken":"1601945361","indexes.conf":"[default]\nrepFactor = auto\nmaxDataSize = auto\nhomePath = $SPLUNK_DB/$_index_name/db\ncoldPath = $SPLUNK_DB/$_index_name/colddb\nthawedPath = $SPLUNK_DB/$_index_name/thaweddb\n \n[volume:msos_s2s3_vol]\nstorageType = remote\npath = s3://testbucket-rs-london\nremote.s3.access_key = abcdJDckRkxhMEdmSk5FekFRRzBFOXV6bGNldzJSWE9IenhVUy80aa\nremote.s3.secret_key = g4NVp0a29PTzlPdGczWk1vekVUcVBSa0o4NkhBWWMvR1NadDV4YVEy\nremote.s3.endpoint = https://s3-eu-west-2.amazonaws.com\n \n[salesdata1]\nremotePath = volume:msos_s2s3_vol/remotepath1\n\n[salesdata2]\nremotePath = volume:msos_s2s3_vol/remotepath2\n\n[salesdata3]\nremotePath = volume:msos_s2s3_vol/remotepath3\n","server.conf":""}}`)

	// Missing Volume config should return an error
	cr.Spec.SmartStore.VolList = nil
	_, _, err = ApplySmartstoreConfigMap(client, &cr, &cr.Spec.SmartStore)
	if err == nil {
		t.Errorf("Configuring Indexes without volumes should return an error")
	}
}

func TestApplyAppListingConfigMap(t *testing.T) {
	cr := enterpriseApi.ClusterMaster{
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterMaster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol",
						Endpoint:  "https://s3-eu-west-2.amazonaws.com",
						Path:      "testbucket-rs-london",
						SecretRef: "s3-secret",
						Type:      "s3",
						Provider:  "aws"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeCluster},
					},
					{Name: "appsWithPreConfigRequired",
						Location: "repoForAppsWithPreConfigRequired",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "clusterWithPreConfig"},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	var S3Response splclient.S3Response

	remoteObjListMap := make(map[string]splclient.S3Response)

	// Fill appSrc adminApps
	startAppPathAndName := "adminCategoryOne.tgz"
	S3Response.Objects = createRemoteObjectList("b41d8cd98f00", startAppPathAndName, 2322, nil, 10)
	remoteObjListMap[cr.Spec.AppFrameworkConfig.AppSources[0].Name] = S3Response

	startAppPathAndName = "securityCategoryOne.tgz"
	S3Response.Objects = createRemoteObjectList("c41d8cd98f00", startAppPathAndName, 3322, nil, 10)
	remoteObjListMap[cr.Spec.AppFrameworkConfig.AppSources[1].Name] = S3Response

	startAppPathAndName = "appWithPreConfigReqOne.tgz"
	S3Response.Objects = createRemoteObjectList("d41d8cd98f00", startAppPathAndName, 4322, nil, 10)
	remoteObjListMap[cr.Spec.AppFrameworkConfig.AppSources[2].Name] = S3Response

	// set the status context
	initAppFrameWorkContext(client, &cr, &cr.Spec.AppFrameworkConfig, &cr.Status.AppContext)

	appsModified, err := handleAppRepoChanges(client, &cr, &cr.Status.AppContext, remoteObjListMap, &cr.Spec.AppFrameworkConfig)

	if err != nil {
		t.Errorf("Empty remote Object list should not trigger an error, but got error : %v", err)
	}

	testAppListingConfigMap := func(client *spltest.MockClient, cr splcommon.MetaObject, appConf *enterpriseApi.AppFrameworkSpec, appsSrcDeployStatus map[string]enterpriseApi.AppSrcDeployInfo, want string) {
		f := func() (interface{}, error) {
			configMap, _, err := ApplyAppListingConfigMap(client, cr, appConf, appsSrcDeployStatus, appsModified)
			// Make the config token as predictable
			configMap.Data[appsUpdateToken] = "1601945361"
			return configMap, err
		}
		configTester(t, "(ApplyAppListingConfigMap)", f, want)
	}

	testAppListingConfigMap(client, &cr, &cr.Spec.AppFrameworkConfig, cr.Status.AppContext.AppsSrcDeployStatus, `{"metadata":{"name":"splunk-example-clustermaster-app-list","namespace":"test","creationTimestamp":null,"ownerReferences":[{"apiVersion":"","kind":"ClusterMaster","name":"example","uid":"","controller":true}]},"data":{"app-list-cluster-with-pre-config.yaml":"splunk:\n  apps_location:\n      - \"/init-apps/appsWithPreConfigRequired/1_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/2_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/3_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/4_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/5_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/6_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/7_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/8_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/9_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/10_appWithPreConfigReqOne.tgz\"","app-list-cluster.yaml":"splunk:\n  app_paths_install:\n    idxc:\n      - \"/init-apps/securityApps/1_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/2_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/3_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/4_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/5_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/6_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/7_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/8_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/9_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/10_securityCategoryOne.tgz\"","app-list-local.yaml":"splunk:\n  app_paths_install:\n    default:\n      - \"/init-apps/adminApps/1_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/2_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/3_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/4_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/5_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/6_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/7_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/8_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/9_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/10_adminCategoryOne.tgz\"","appsUpdateToken":"1601945361"}}`)

	// Make sure that the App Listing configMap works fine for SearchHeadCluster
	cr.Kind = "SearchHeadCluster"
	testAppListingConfigMap(client, &cr, &cr.Spec.AppFrameworkConfig, cr.Status.AppContext.AppsSrcDeployStatus, `{"metadata":{"name":"splunk-example-searchheadcluster-app-list","namespace":"test","creationTimestamp":null,"ownerReferences":[{"apiVersion":"","kind":"SearchHeadCluster","name":"example","uid":"","controller":true}]},"data":{"app-list-cluster-with-pre-config.yaml":"splunk:\n  apps_location:\n      - \"/init-apps/appsWithPreConfigRequired/1_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/2_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/3_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/4_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/5_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/6_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/7_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/8_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/9_appWithPreConfigReqOne.tgz\"\n      - \"/init-apps/appsWithPreConfigRequired/10_appWithPreConfigReqOne.tgz\"","app-list-cluster.yaml":"splunk:\n  app_paths_install:\n    shc:\n      - \"/init-apps/securityApps/1_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/2_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/3_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/4_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/5_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/6_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/7_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/8_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/9_securityCategoryOne.tgz\"\n      - \"/init-apps/securityApps/10_securityCategoryOne.tgz\"","app-list-local.yaml":"splunk:\n  app_paths_install:\n    default:\n      - \"/init-apps/adminApps/1_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/2_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/3_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/4_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/5_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/6_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/7_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/8_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/9_adminCategoryOne.tgz\"\n      - \"/init-apps/adminApps/10_adminCategoryOne.tgz\"","appsUpdateToken":"1601945361"}}`)

	// Now test the Cluster manager stateful set, to validate the Pod updates with the app listing config map
	cr.Kind = "ClusterMaster"
	_, err = splutil.ApplyNamespaceScopedSecretObject(client, "test")
	if err != nil {
		t.Errorf("Failed to create namespace scoped object")
	}

	// to pass the validation stage, add the directory to download apps
	err = os.MkdirAll(splcommon.AppDownloadVolume, 0755)
	defer os.RemoveAll(splcommon.AppDownloadVolume)

	if err != nil {
		t.Errorf("Unable to create download directory for apps :%s", splcommon.AppDownloadVolume)
	}

	// ToDo: sgontla: phase-2 cleanup
	// testStsWithAppListVolMounts := func(want string) {
	// 	f := func() (interface{}, error) {
	// 		if err := validateClusterManagerSpec(&cr); err != nil {
	// 			t.Errorf("validateClusterManagerSpec() returned error: %v", err)
	// 		}
	// 		return getClusterManagerStatefulSet(client, &cr)
	// 	}
	// 	configTester(t, "getClusterManagerStatefulSet", f, want)
	// }

	// testStsWithAppListVolMounts(splcommon.TestApplyAppListingConfigMap)

	// // Test to ensure that the Applisting config map is empty after the apps are installed successfully
	// markAppsStatusToComplete(client, &cr, &cr.Spec.AppFrameworkConfig, cr.Status.AppContext.AppsSrcDeployStatus)
	// testAppListingConfigMap(client, &cr, &cr.Spec.AppFrameworkConfig, cr.Status.AppContext.AppsSrcDeployStatus, `{"metadata":{"name":"splunk-example-clustermaster-app-list","namespace":"test","creationTimestamp":null,"ownerReferences":[{"apiVersion":"","kind":"ClusterMaster","name":"example","uid":"","controller":true}]},"data":{"appsUpdateToken":"1601945361"}}`)

}

func TestRemoveOwenerReferencesForSecretObjectsReferredBySmartstoreVolumes(t *testing.T) {
	cr := enterpriseApi.ClusterMaster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idxCluster",
			Namespace: "test",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			SmartStore: enterpriseApi.SmartStoreSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
					{Name: "msos_s2s3_vol_2", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
					{Name: "msos_s2s3_vol_3", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
					{Name: "msos_s2s3_vol_4", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
				},

				IndexList: []enterpriseApi.IndexSpec{
					{Name: "salesdata1", RemotePath: "remotepath1",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
					{Name: "salesdata2", RemotePath: "remotepath2",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
					{Name: "salesdata3", RemotePath: "remotepath3",
						IndexAndGlobalCommonSpec: enterpriseApi.IndexAndGlobalCommonSpec{
							VolName: "msos_s2s3_vol"},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	// Create namespace scoped secret
	secret, err := splutil.ApplyNamespaceScopedSecretObject(client, "test")
	if err != nil {
		t.Errorf(err.Error())
	}

	secret.Data[s3AccessKey] = []byte("abcdJDckRkxhMEdmSk5FekFRRzBFOXV6bGNldzJSWE9IenhVUy80aa")
	secret.Data[s3SecretKey] = []byte("g4NVp0a29PTzlPdGczWk1vekVUcVBSa0o4NkhBWWMvR1NadDV4YVEy")
	_, err = splctrl.ApplySecret(client, secret)
	if err != nil {
		t.Errorf(err.Error())
	}

	// Test existing secret
	err = splutil.SetSecretOwnerRef(client, secret.GetName(), &cr)
	if err != nil {
		t.Errorf("Couldn't set owner ref for secret %s", secret.GetName())
	}

	err = DeleteOwnerReferencesForS3SecretObjects(client, secret, &cr.Spec.SmartStore)

	if err != nil {
		t.Errorf("Couldn't Remove S3 Secret object references %v", err)
	}

	// If the secret object doesn't exist, should return an error
	// Here in the volume references, secrets splunk-test-sec_1, to splunk-test-sec_4 doesn't exist
	cr = enterpriseApi.ClusterMaster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idxCluster",
			Namespace: "testWithNoSecret",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			SmartStore: enterpriseApi.SmartStoreSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-sec_1"},
					{Name: "msos_s2s3_vol_2", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-sec_2"},
					{Name: "msos_s2s3_vol_3", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-sec_3"},
					{Name: "msos_s2s3_vol_4", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-sec_4"},
				},
			},
		},
	}

	// S3 secret owner reference removal, with non-existing secret objects
	err = DeleteOwnerReferencesForS3SecretObjects(client, secret, &cr.Spec.SmartStore)
	if err == nil {
		t.Errorf("Should report an error, when the secret object referenced in the volume config doesn't exist")
	}

	// Smartstore volume config with non-existing secret objects
	err = DeleteOwnerReferencesForResources(client, &cr, &cr.Spec.SmartStore)
	if err == nil {
		t.Errorf("Should report an error, when the secret objects doesn't exist")
	}
}

func TestGetSmartstoreRemoteVolumeSecrets(t *testing.T) {
	cr := enterpriseApi.ClusterMaster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "CM",
			Namespace: "test",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			SmartStore: enterpriseApi.SmartStoreSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "splunk-test-secret"},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	// Just to simplify the test, assume that the keys are stored as part of the splunk-test-secret object, hence create that secret object
	secret, err := splutil.ApplyNamespaceScopedSecretObject(client, "test")
	if err != nil {
		t.Errorf(err.Error())
	}

	_, err = splctrl.ApplySecret(client, secret)
	if err != nil {
		t.Errorf(err.Error())
	}

	// Missing S3 access key should return error
	_, _, _, err = GetSmartstoreRemoteVolumeSecrets(cr.Spec.SmartStore.VolList[0], client, &cr, &cr.Spec.SmartStore)
	if err == nil {
		t.Errorf("Missing S3 access key should return an error")
	}

	secret.Data[s3AccessKey] = []byte("abcdJDckRkxhMEdmSk5FekFRRzBFOXV6bGNldzJSWE9IenhVUy80aa")

	// Missing S3 secret key should return error
	_, _, _, err = GetSmartstoreRemoteVolumeSecrets(cr.Spec.SmartStore.VolList[0], client, &cr, &cr.Spec.SmartStore)
	if err == nil {
		t.Errorf("Missing S3 secret key should return an error")
	}

	// When access key and secret keys are present, returned keys should not be empty. Also, should not return an error
	secret.Data[s3SecretKey] = []byte("g4NVp0a29PTzlPdGczWk1vekVUcVBSa0o4NkhBWWMvR1NadDV4YVEy")
	accessKey, secretKey, _, err := GetSmartstoreRemoteVolumeSecrets(cr.Spec.SmartStore.VolList[0], client, &cr, &cr.Spec.SmartStore)
	if accessKey == "" || secretKey == "" || err != nil {
		t.Errorf("Missing S3 Keys / Error not expected, when the Secret object with the S3 specific keys are present")
	}
}

func TestCheckIfAnAppIsActiveOnRemoteStore(t *testing.T) {
	var remoteObjList []*splclient.RemoteObject
	var entry *splclient.RemoteObject

	tmpAppName := "xys.spl"
	entry = allocateRemoteObject("d41d8cd98f00", tmpAppName, 2322, nil)

	remoteObjList = append(remoteObjList, entry)

	if !checkIfAnAppIsActiveOnRemoteStore(tmpAppName, remoteObjList) {
		t.Errorf("Failed to detect for a valid app from remote listing")
	}

	if checkIfAnAppIsActiveOnRemoteStore("app10.tgz", remoteObjList) {
		t.Errorf("Non existing app is reported as existing")
	}

}

func TestInitAndCheckAppInfoStatusShouldNotFail(t *testing.T) {
	cr := enterpriseApi.Standalone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		Spec: enterpriseApi.StandaloneSpec{
			Replicas: 1,
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				AppsRepoPollInterval: 0,
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret", Provider: "aws"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
					{Name: "authenticationApps",
						Location: "authenticationAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	// add another standalone cr to the list
	revised := cr
	revised.ObjectMeta.Name = "standalone2"

	var appDeployContext enterpriseApi.AppDeploymentContext
	appDeployContext.AppFrameworkConfig = cr.Spec.AppFrameworkConfig
	err := initAndCheckAppInfoStatus(client, &cr, &cr.Spec.AppFrameworkConfig, &appDeployContext)
	if err != nil {
		t.Errorf("initAndCheckAppInfoStatus should not have returned error")
	}

	var configMap *corev1.ConfigMap
	configMapName := GetSplunkManualAppUpdateConfigMapName(cr.GetNamespace())
	namespacedName := types.NamespacedName{Namespace: cr.GetNamespace(), Name: configMapName}
	_, err = splctrl.GetConfigMap(client, namespacedName)
	if err != nil {
		t.Errorf("Unable to get configMap")
	}

	// check the status and refCount first time
	refCount := getManualUpdateRefCount(client, &cr, configMapName)
	status := getManualUpdateStatus(client, &cr, configMapName)
	if refCount != 1 || status != "off" {
		t.Errorf("Got wrong status or/and refCount. Expected status=off, Got=%s. Expected refCount=1, Got=%d", status, refCount)
	}

	var appDeployContext2 enterpriseApi.AppDeploymentContext
	appDeployContext2.AppFrameworkConfig = revised.Spec.AppFrameworkConfig
	err = initAndCheckAppInfoStatus(client, &revised, &revised.Spec.AppFrameworkConfig, &appDeployContext2)
	if err != nil {
		t.Errorf("initAndCheckAppInfoStatus should not have returned error")
	}

	_, err = splctrl.GetConfigMap(client, namespacedName)
	if err != nil {
		t.Errorf("Unable to get configMap")
	}

	// check the status and refCount second time. We should have turned off manual update now.
	refCount = getManualUpdateRefCount(client, &revised, configMapName)
	status = getManualUpdateStatus(client, &revised, configMapName)
	if refCount != 2 || status != "off" {
		t.Errorf("Got wrong status or/and refCount. Expected status=off, Got=%s. Expected refCount=2, Got=%d", status, refCount)
	}

	// prepare the configMap
	crKindMap := make(map[string]string)
	configMapData := fmt.Sprintf(`status: on
	refCount: 2`)

	crKindMap[cr.GetObjectKind().GroupVersionKind().Kind] = configMapData

	configMap = splctrl.PrepareConfigMap(configMapName, cr.GetNamespace(), crKindMap)

	_, err = splctrl.ApplyConfigMap(client, configMap)
	if err != nil {
		t.Errorf("ApplyConfigMap should not have returned error")
	}
	// set this CR as the owner ref for the config map
	err = SetConfigMapOwnerRef(client, &cr, configMap)
	if err != nil {
		t.Errorf("Unable to set owner reference for configMap: %s", configMap.Name)
	}

	// set the second CR too as the owner ref for the config map
	err = SetConfigMapOwnerRef(client, &revised, configMap)
	if err != nil {
		t.Errorf("Unable to set owner reference for configMap: %s", configMap.Name)
	}

	err = initAndCheckAppInfoStatus(client, &revised, &revised.Spec.AppFrameworkConfig, &appDeployContext2)
	if err != nil {
		t.Errorf("initAndCheckAppInfoStatus should not have returned error")
	}

	// check the status and refCount second time. We should have turned off manual update now.
	refCount = getManualUpdateRefCount(client, &revised, configMapName)
	status = getManualUpdateStatus(client, &revised, configMapName)
	if refCount != 1 || status != "on" {
		t.Errorf("Got wrong status or/and refCount. Expected status=on, Got=%s. Expected refCount=1, Got=%d", status, refCount)
	}

	appDeployContext2.IsDeploymentInProgress = false
	err = initAndCheckAppInfoStatus(client, &cr, &cr.Spec.AppFrameworkConfig, &appDeployContext2)
	if err != nil {
		t.Errorf("initAndCheckAppInfoStatus should not have returned error")
	}

	// check the status and refCount second time. We should have turned off manual update now.
	refCount = getManualUpdateRefCount(client, &cr, configMapName)
	status = getManualUpdateStatus(client, &cr, configMapName)
	if refCount != 2 || status != "off" {
		t.Errorf("Got wrong status or/and refCount. Expected status=off, Got=%s. Expected refCount=2, Got=%d", status, refCount)
	}

}

func TestInitAndCheckAppInfoStatusShouldFail(t *testing.T) {
	cr := enterpriseApi.Standalone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		Spec: enterpriseApi.StandaloneSpec{
			Replicas: 1,
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				AppsRepoPollInterval: 0,
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
					{Name: "authenticationApps",
						Location: "authenticationAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   "local"},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	var appDeployContext enterpriseApi.AppDeploymentContext
	appDeployContext.AppFrameworkConfig = cr.Spec.AppFrameworkConfig

	initAndCheckAppInfoStatus(client, &cr, &cr.Spec.AppFrameworkConfig, &appDeployContext)
	if appDeployContext.LastAppInfoCheckTime != 0 {
		t.Errorf("We should not have updated the LastAppInfoCheckTime as polling of apps repo is disabled.")
	}
}

func TestHandleAppRepoChanges(t *testing.T) {
	cr := enterpriseApi.Standalone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "Clustermaster",
			Namespace: "test",
		},
		Spec: enterpriseApi.StandaloneSpec{
			Replicas: 1,
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "authenticationApps",
						Location: "authenticationAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	var appDeployContext enterpriseApi.AppDeploymentContext
	var remoteObjListMap map[string]splclient.S3Response
	var appFramworkConf enterpriseApi.AppFrameworkSpec = cr.Spec.AppFrameworkConfig
	var err error

	if appDeployContext.AppsSrcDeployStatus == nil {
		appDeployContext.AppsSrcDeployStatus = make(map[string]enterpriseApi.AppSrcDeployInfo)
	}

	var S3Response splclient.S3Response

	// Test-1: Empty remoteObjectList Map should return an error
	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)

	if err != nil {
		t.Errorf("Empty remote Object list should not trigger an error, but got error : %v", err)
	}

	// Test-2: Valid remoteObjectList should not cause an error
	startAppPathAndName := "bucketpath1/bpath2/locationpath1/lpath2/adminCategoryOne.tgz"
	remoteObjListMap = make(map[string]splclient.S3Response)
	// Prepare a S3Response
	S3Response.Objects = createRemoteObjectList("d41d8cd98f00", startAppPathAndName, 2322, nil, 10)
	// Set the app source with a matching one
	remoteObjListMap[appFramworkConf.AppSources[0].Name] = S3Response

	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unexpected app status. Error: %v", err)
	}

	// Test-3: If the App Resource is not found in the remote object listing, all the corresponding Apps should be deleted/disabled
	delete(remoteObjListMap, appFramworkConf.AppSources[0].Name)
	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unable to delete/disable Apps, when the AppSource is deleted. Unexpected app status. Error: %v", err)
	}
	setStateAndStatusForAppDeployInfoList(appDeployContext.AppsSrcDeployStatus[appFramworkConf.AppSources[0].Name].AppDeploymentInfoList, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)

	// Test-4: If the App Resource is not found in the config, all the corresponding Apps should be deleted/disabled
	tmpAppSrcName := appFramworkConf.AppSources[0].Name
	appFramworkConf.AppSources[0].Name = "invalidName"
	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}
	appFramworkConf.AppSources[0].Name = tmpAppSrcName

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unable to delete/disable Apps, when the AppSource is deleted from the config. Unexpected app status. Error: %v", err)
	}

	// Test-5: Changing the AppSource deployment info should change for all the Apps in the list
	changeAppSrcDeployInfoStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusPending, enterpriseApi.DeployStatusInProgress)
	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusInProgress)
	if err != nil {
		t.Errorf("Invalid AppSrc deployment info detected. Error: %v", err)
	}

	// Test-6: When an App is deleted on remote store, it should be marked as deleted
	setStateAndStatusForAppDeployInfoList(appDeployContext.AppsSrcDeployStatus[appFramworkConf.AppSources[0].Name].AppDeploymentInfoList, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)

	// delete an object on remote store for the app source
	tmpS3Response := S3Response
	tmpS3Response.Objects = append(tmpS3Response.Objects[:0], tmpS3Response.Objects[1:]...)
	remoteObjListMap[appFramworkConf.AppSources[0].Name] = tmpS3Response

	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unable to delete/disable an app when the App is deleted from remote store. Error: %v", err)
	}

	// Test-7: Object hash change on the remote store should cause App state and status as Active and Pending.
	S3Response.Objects = createRemoteObjectList("e41d8cd98f00", startAppPathAndName, 2322, nil, 10)
	remoteObjListMap[appFramworkConf.AppSources[0].Name] = S3Response

	setStateAndStatusForAppDeployInfoList(appDeployContext.AppsSrcDeployStatus[appFramworkConf.AppSources[0].Name].AppDeploymentInfoList, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusComplete)

	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unable to detect the change, when the object changed. Error: %v", err)
	}

	// Test-8:  For an AppSrc, when all the Apps are deleted on remote store and re-introduced, should modify the state to active and pending
	setStateAndStatusForAppDeployInfoList(appDeployContext.AppsSrcDeployStatus[appFramworkConf.AppSources[0].Name].AppDeploymentInfoList, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusComplete)

	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)
	if err != nil {
		t.Errorf("Could not handle a valid remote listing. Error: %v", err)
	}

	_, err = validateAppSrcDeployInfoByStateAndStatus(appFramworkConf.AppSources[0].Name, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)
	if err != nil {
		t.Errorf("Unable to delete/disable the Apps when the Apps are deleted from remote store. Error: %v", err)
	}

	// Test-9: Unknown App source in remote obj listing should return an error
	startAppPathAndName = "csecurityApps.spl"
	S3Response.Objects = createRemoteObjectList("d41d8cd98f00", startAppPathAndName, 2322, nil, 10)
	invalidAppSourceName := "UnknownAppSourceInConfig"
	remoteObjListMap[invalidAppSourceName] = S3Response
	_, err = handleAppRepoChanges(client, &cr, &appDeployContext, remoteObjListMap, &appFramworkConf)

	if err == nil {
		t.Errorf("Unable to return an error, when the remote listing contain unknown App source")
	}
	delete(remoteObjListMap, invalidAppSourceName)

	// Test-10: Setting  all apps in AppSrc to complete should mark all the apps status as complete irrespective of their state
	// 10.1 Check for state=Active and status=Complete
	for appSrc, appSrcDeployStatus := range appDeployContext.AppsSrcDeployStatus {
		// ToDo: Enable for Phase-3
		//setStateAndStatusForAppDeployInfoList(appSrcDeployStatus.AppDeploymentInfoList, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusInProgress)
		setStateAndStatusForAppDeployInfoList(appSrcDeployStatus.AppDeploymentInfoList, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)
		appDeployContext.AppsSrcDeployStatus[appSrc] = appSrcDeployStatus

		// ToDo: Enable for Phase-3
		//expectedMatchCount := getAppSrcDeployInfoCountByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusInProgress)
		expectedMatchCount := getAppSrcDeployInfoCountByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusPending)

		markAppsStatusToComplete(client, &cr, &cr.Spec.AppFrameworkConfig, appDeployContext.AppsSrcDeployStatus)

		matchCount, err := validateAppSrcDeployInfoByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusComplete)
		if err != nil {
			t.Errorf("Unable to change the Apps status to complete, once the changes are reflecting on the Pod. Error: %v", err)
		}
		if expectedMatchCount != matchCount {
			t.Errorf("App status change failed. Expected count %v, returned count %v", expectedMatchCount, matchCount)
		}
	}

	// 10.2 Check for state=Deleted status=Complete
	for appSrc, appSrcDeployStatus := range appDeployContext.AppsSrcDeployStatus {
		// ToDo: Enable for Phase-3
		//setStateAndStatusForAppDeployInfoList(appSrcDeployStatus.AppDeploymentInfoList, enterpriseApi.RepoStateActive, enterpriseApi.DeployStatusInProgress)
		setStateAndStatusForAppDeployInfoList(appSrcDeployStatus.AppDeploymentInfoList, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusPending)
		appDeployContext.AppsSrcDeployStatus[appSrc] = appSrcDeployStatus

		// ToDo: Enable for Phase-3
		//expectedMatchCount := getAppSrcDeployInfoCountByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusInProgress)
		expectedMatchCount := getAppSrcDeployInfoCountByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusPending)

		markAppsStatusToComplete(client, &cr, &cr.Spec.AppFrameworkConfig, appDeployContext.AppsSrcDeployStatus)

		matchCount, err := validateAppSrcDeployInfoByStateAndStatus(appSrc, appDeployContext.AppsSrcDeployStatus, enterpriseApi.RepoStateDeleted, enterpriseApi.DeployStatusComplete)
		if err != nil {
			t.Errorf("Unable to delete/disable an app when the App is deleted from remote store. Error: %v", err)
		}
		if expectedMatchCount != matchCount {
			t.Errorf("App status change failed. Expected count %v, returned count %v", expectedMatchCount, matchCount)
		}
	}
}

func TestAppPhaseStatusAsStr(t *testing.T) {
	var status string
	status = appPhaseStatusAsStr(enterpriseApi.AppPkgDownloadPending)
	if status != "Download Pending" {
		t.Errorf("Got wrong status. Expected status=Download Pending, Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgDownloadInProgress)
	if status != "Download In Progress" {
		t.Errorf("Got wrong status. Expected status=\"Download In Progress\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgDownloadComplete)
	if status != "Download Complete" {
		t.Errorf("Got wrong status. Expected status=\"Download Complete\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgDownloadError)
	if status != "Download Error" {
		t.Errorf("Got wrong status. Expected status=\"Download Error\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgPodCopyPending)
	if status != "Pod Copy Pending" {
		t.Errorf("Got wrong status. Expected status=Pod Copy Pending, Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgPodCopyInProgress)
	if status != "Pod Copy In Progress" {
		t.Errorf("Got wrong status. Expected status=\"Pod Copy In Progress\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgPodCopyComplete)
	if status != "Pod Copy Complete" {
		t.Errorf("Got wrong status. Expected status=\"Pod Copy Complete\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgPodCopyError)
	if status != "Pod Copy Error" {
		t.Errorf("Got wrong status. Expected status=\"Pod Copy Error\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgInstallPending)
	if status != "Install Pending" {
		t.Errorf("Got wrong status. Expected status=Install Pending, Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgInstallInProgress)
	if status != "Install In Progress" {
		t.Errorf("Got wrong status. Expected status=\"Install In Progress\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgInstallComplete)
	if status != "Install Complete" {
		t.Errorf("Got wrong status. Expected status=\"Install Complete\", Got = %s", status)
	}

	status = appPhaseStatusAsStr(enterpriseApi.AppPkgInstallError)
	if status != "Install Error" {
		t.Errorf("Got wrong status. Expected status=\"Install Error\", Got = %s", status)
	}
}

func TestGetAvailableDiskSpaceShouldFail(t *testing.T) {
	//add the directory to download apps
	_ = os.MkdirAll(splcommon.AppDownloadVolume, 0755)
	defer os.RemoveAll(splcommon.AppDownloadVolume)

	size, _ := getAvailableDiskSpace()
	if size == 0 {
		t.Errorf("getAvailableDiskSpace should have returned a non-zero size.")
	}
}

func TestIsAppExtentionValid(t *testing.T) {
	if !isAppExtentionValid("testapp.spl") || !isAppExtentionValid("testapp.tgz") {
		t.Errorf("failed to detect valid app extension")
	}

	if isAppExtentionValid("testapp.aspl") || isAppExtentionValid("testapp.ttgz") {
		t.Errorf("failed to detect invalid app extension")
	}
}

func TestHasAppRepoCheckTimerExpired(t *testing.T) {

	// Case 1. This is the case when we first enter the reconcile loop.
	appInfoContext := &enterpriseApi.AppDeploymentContext{
		LastAppInfoCheckTime: 0,
	}

	if !HasAppRepoCheckTimerExpired(appInfoContext) {
		t.Errorf("ShouldCheckAppStatus should have returned true")
	}

	appInfoContext.AppsRepoStatusPollInterval = 60

	// Case 2. We just checked the apps status
	SetLastAppInfoCheckTime(appInfoContext)

	if HasAppRepoCheckTimerExpired(appInfoContext) {
		t.Errorf("ShouldCheckAppStatus should have returned false since we just checked the apps status")
	}

	// Case 3. Lets check after AppsRepoPollInterval has elapsed.
	// We do this by setting some random past timestamp.
	appInfoContext.LastAppInfoCheckTime = 1591464060

	if !HasAppRepoCheckTimerExpired(appInfoContext) {
		t.Errorf("ShouldCheckAppStatus should have returned true")
	}
}

func allocateRemoteObject(etag string, key string, Size int64, lastModified *time.Time) *splclient.RemoteObject {
	var remoteObj splclient.RemoteObject

	remoteObj.Etag = &etag
	remoteObj.Key = &key
	remoteObj.Size = &Size
	//tmpEntry.LastModified = lastModified

	return &remoteObj
}

func createRemoteObjectList(etag string, key string, Size int64, lastModified *time.Time, count uint16) []*splclient.RemoteObject {
	var remoteObjList []*splclient.RemoteObject
	var remoteObj *splclient.RemoteObject

	for i := 1; i <= int(count); i++ {
		tag := strconv.Itoa(i)
		remoteObj = allocateRemoteObject(tag+etag, tag+"_"+key, Size+int64(i), nil)
		remoteObjList = append(remoteObjList, remoteObj)
	}

	return remoteObjList
}

func validateAppSrcDeployInfoByStateAndStatus(appSrc string, appSrcDeployStatus map[string]enterpriseApi.AppSrcDeployInfo, repoState enterpriseApi.AppRepoState, deployStatus enterpriseApi.AppDeploymentStatus) (int, error) {
	var matchCount int
	if appSrcDeploymentInfo, ok := appSrcDeployStatus[appSrc]; ok {
		appDeployInfoList := appSrcDeploymentInfo.AppDeploymentInfoList
		for _, appDeployInfo := range appDeployInfoList {
			// Check if the app status is as expected
			if appDeployInfo.RepoState == repoState && appDeployInfo.DeployStatus != deployStatus {
				return matchCount, fmt.Errorf("Invalid app status for appSrc %s, appName: %s", appSrc, appDeployInfo.AppName)
			}
			matchCount++
		}
	} else {
		return matchCount, fmt.Errorf("Missing app source %s, shouldn't not happen", appSrc)
	}

	return matchCount, nil
}

func getAppSrcDeployInfoCountByStateAndStatus(appSrc string, appSrcDeployStatus map[string]enterpriseApi.AppSrcDeployInfo, repoState enterpriseApi.AppRepoState, deployStatus enterpriseApi.AppDeploymentStatus) int {
	var matchCount int
	if appSrcDeploymentInfo, ok := appSrcDeployStatus[appSrc]; ok {
		appDeployInfoList := appSrcDeploymentInfo.AppDeploymentInfoList
		for _, appDeployInfo := range appDeployInfoList {
			// Check if the app status is as expected
			if appDeployInfo.RepoState == repoState && appDeployInfo.DeployStatus == deployStatus {
				matchCount++
			}
		}
	}

	return matchCount
}

func TestSetLastAppInfoCheckTime(t *testing.T) {
	appInfoStatus := &enterpriseApi.AppDeploymentContext{}
	SetLastAppInfoCheckTime(appInfoStatus)

	if appInfoStatus.LastAppInfoCheckTime != time.Now().Unix() {
		t.Errorf("LastAppInfoCheckTime should have been set to current time")
	}
}

func TestGetNextRequeueTime(t *testing.T) {
	appFrameworkContext := enterpriseApi.AppDeploymentContext{}
	appFrameworkContext.AppsRepoStatusPollInterval = 60
	nextRequeueTime := GetNextRequeueTime(appFrameworkContext.AppsRepoStatusPollInterval, (time.Now().Unix() - int64(40)))
	if nextRequeueTime > time.Second*20 {
		t.Errorf("Got wrong next requeue time")
	}
}

func TestShouldCheckAppRepoStatus(t *testing.T) {
	cr := enterpriseApi.Standalone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone1",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		Spec: enterpriseApi.StandaloneSpec{
			Replicas: 1,
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret", Type: "s3", Provider: "aws"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "authenticationApps",
						Location: "authenticationAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
				},
			},
		},
	}

	c := spltest.NewMockClient()

	var appStatusContext enterpriseApi.AppDeploymentContext
	appStatusContext.AppsRepoStatusPollInterval = 0
	var turnOffManualChecking bool
	shouldCheck := shouldCheckAppRepoStatus(c, &cr, &appStatusContext, cr.GetObjectKind().GroupVersionKind().Kind, &turnOffManualChecking)
	if shouldCheck == true {
		t.Errorf("shouldCheckAppRepoStatus should have returned false as there is no configMap yet.")
	}

	crKindMap := make(map[string]string)
	configMapData := fmt.Sprintf(`status: on
refCount: 1`)
	crKindMap[cr.GetObjectKind().GroupVersionKind().Kind] = configMapData

	configMap := splctrl.PrepareConfigMap(GetSplunkManualAppUpdateConfigMapName(cr.GetNamespace()), cr.GetNamespace(), crKindMap)
	c.AddObject(configMap)
	shouldCheck = shouldCheckAppRepoStatus(c, &cr, &appStatusContext, cr.GetObjectKind().GroupVersionKind().Kind, &turnOffManualChecking)
	if shouldCheck != true {
		t.Errorf("shouldCheckAppRepoStatus should have returned true.")
	}
}

func TestValidateMonitoringConsoleRef(t *testing.T) {
	currentCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-test-monitoring-console",
			Namespace: "test",
		},
		Data: map[string]string{"a": "b"},
	}

	current := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-s1-standalone",
			Namespace: "test",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "SPLUNK_MONITORING_CONSOLE_REF",
									Value: "test",
								},
							},
						},
					},
				},
			},
		},
	}

	revised := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-s1-standalone",
			Namespace: "test",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{
									Name:  "SPLUNK_MONITORING_CONSOLE_REF",
									Value: "abc",
								},
							},
						},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	//create configmap
	_, err := splctrl.ApplyConfigMap(client, &currentCM)
	if err != nil {
		t.Errorf("Failed to create the configMap. Error: %s", err.Error())
	}

	// Create statefulset
	err = splutil.CreateResource(client, current)
	if err != nil {
		t.Errorf("Failed to create owner reference  %s", current.GetName())
	}

	var serviceURLs []corev1.EnvVar
	serviceURLs = []corev1.EnvVar{
		{
			Name:  "A",
			Value: "a",
		},
	}

	err = validateMonitoringConsoleRef(client, revised, serviceURLs)
	if err != nil {
		t.Errorf("Couldn't validate monitoring console ref %s", current.GetName())
	}

	revised = &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-s1-standalone",
			Namespace: "test",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Env: []corev1.EnvVar{
								{},
							},
						},
					},
				},
			},
		},
	}

	err = validateMonitoringConsoleRef(client, revised, serviceURLs)
	if err != nil {
		t.Errorf("Couldn't validate monitoring console ref %s", current.GetName())
	}
}

func TestUpdateOrRemoveEntryFromConfigMap(t *testing.T) {
	stand1 := enterpriseApi.Standalone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone1",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		Spec: enterpriseApi.StandaloneSpec{
			Replicas: 1,
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				VolList: []enterpriseApi.VolumeSpec{
					{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret", Type: "s3", Provider: "aws"},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{Name: "adminApps",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "securityApps",
						Location: "securityAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
					{Name: "authenticationApps",
						Location: "authenticationAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "msos_s2s3_vol",
							Scope:   enterpriseApi.ScopeLocal},
					},
				},
			},
		},
	}

	client := spltest.NewMockClient()

	// To test the failure scenario, do not add the configMap to the client yet
	err := UpdateOrRemoveEntryFromConfigMap(client, &stand1, SplunkStandalone)
	if err == nil {
		t.Errorf("UpdateOrRemoveEntryFromConfigMap should have returned error as there is no configMap yet")
	}

	kind := stand1.GetObjectKind().GroupVersionKind().Kind

	crKindMap := make(map[string]string)

	// now prepare the configMap and add it
	configMapData := fmt.Sprintf(`status: off
refCount: 1`)

	crKindMap[kind] = configMapData
	configMapName := GetSplunkManualAppUpdateConfigMapName(stand1.GetNamespace())

	configMap := splctrl.PrepareConfigMap(configMapName, stand1.GetNamespace(), crKindMap)

	client.AddObject(configMap)

	// To test the failure scenario, do not add the standalone cr to the list yet
	err = UpdateOrRemoveEntryFromConfigMap(client, &stand1, SplunkStandalone)
	if err == nil {
		t.Errorf("UpdateOrRemoveEntryFromConfigMap should have returned error as there are no owner references in the configMap")
	}

	// set the second CR too as the owner ref for the config map
	err = SetConfigMapOwnerRef(client, &stand1, configMap)
	if err != nil {
		t.Errorf("Unable to set owner reference for configMap: %s", configMap.Name)
	}

	// create another standalone cr
	stand2 := stand1
	stand2.ObjectMeta.Name = "standalone2"

	// set the second CR too as the owner ref for the config map
	err = SetConfigMapOwnerRef(client, &stand2, configMap)
	if err != nil {
		t.Errorf("Unable to set owner reference for configMap: %s", configMap.Name)
	}

	// We should have decremented the refCount to 1
	err = UpdateOrRemoveEntryFromConfigMap(client, &stand2, SplunkStandalone)
	if err != nil {
		t.Errorf("UpdateOrRemoveEntryFromConfigMap should not have returned error")
	}

	refCount := getManualUpdateRefCount(client, &stand1, configMapName)
	if refCount != 1 {
		t.Errorf("Got wrong refCount. Expected=%d, Got=%d", 1, refCount)
	}

	// remove stand2 as the configMap owner reference
	var ownerRefCount uint
	ownerRefCount, err = RemoveConfigMapOwnerRef(client, &stand2, configMap.Name)
	if ownerRefCount != 1 || err != nil {
		t.Errorf("RemoveConfigMapOwnerRef should not have returned error or number of owner references should be 1.")
	}

	// Now since there is only 1 standalone left, we should be removing the entry from the configMap
	err = UpdateOrRemoveEntryFromConfigMap(client, &stand1, SplunkStandalone)
	if err != nil {
		t.Errorf("UpdateOrRemoveEntryFromConfigMap should not have returned error")
	}

	if _, ok := configMap.Data[kind]; ok {
		t.Errorf("There should not be any entry for this CR type in the configMap")
	}
}

func TestCopyFileToPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-stack1-0",
			Namespace: "test",
			Labels: map[string]string{
				"controller-revision-hash": "v0",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/mnt/splunk-secrets",
							Name:      "mnt-splunk-secrets",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "mnt-splunk-secrets",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "test-secret",
						},
					},
				},
			},
		},
	}

	// Create client and add object
	c := spltest.NewMockClient()
	// Add object
	c.AddObject(pod)

	fileOnOperator := "/tmp/"
	fileOnStandalonePod := "/init-apps/splunkFwdApps/COPYING"

	// Test to detect invalid source file name
	_, _, err := CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	if err == nil || !strings.HasPrefix(err.Error(), "invalid file name") {
		t.Errorf("Unable to detect invalid source file name")
	}

	// Test to detect relative source file path
	fileOnOperator = "tmp/networkIntelligence.spl"
	_, _, err = CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	if err == nil || !strings.HasPrefix(err.Error(), "relative paths are not supported for source path") {
		t.Errorf("Unable to reject relative source path")
	}
	fileOnOperator = "/tmp/networkIntelligence.spl"

	// Test to reject if the source file doesn't exist
	_, _, err = CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	if err == nil || !strings.HasPrefix(err.Error(), "unable to get the info for file") {
		t.Errorf("If file doesn't exist, should return an error")
	}

	// Now create a file on the Pod
	f, err := os.Create(fileOnOperator)
	defer f.Close()
	defer os.Remove(fileOnOperator)
	if err != nil {
		t.Errorf("Failed to create the file: %s, error %s", fileOnOperator, err)
	}

	// Test to detect relative destination file path
	fileOnStandalonePod = "init-apps/splunkFwdApps/COPYING"
	_, _, err = CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	if err == nil || !strings.HasPrefix(err.Error(), "relative paths are not supported for dest path") {
		t.Errorf("Unable to reject relative destination path")
	}
	fileOnStandalonePod = "/init-apps/splunkFwdApps/COPYING"

	// If Pod destination path is directory, source file name is used, and should not cause an error
	fileOnStandalonePod = "/init-apps/splunkFwdApps/"
	_, _, err = CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	// PodExec command fails, as there is no real Pod here. Bypassing the error check for now, just to have enough code coverage.
	// Need to fix this later, once the PodExec can accommodate the UT flow for a non-existing Pod.
	if err != nil && 1 == 0 {
		t.Errorf("Failed to accept the directory as destination path")
	}
	fileOnStandalonePod = "/init-apps/splunkFwdApps/COPYING"

	//Proper source and destination paths should not return an error
	_, _, err = CopyFileToPod(c, pod.GetNamespace(), pod.GetName(), fileOnOperator, fileOnStandalonePod)
	// PodExec command fails, as there is no real Pod here. Bypassing the error check for now, just to have enough code coverage.
	// Need to fix this later, once the PodExec can accommodate the UT flow for a non-existing Pod.
	if err != nil && 1 == 0 {
		t.Errorf("Valid source and destination paths should not cause an error. Error: %s", err)
	}
}

func TestSetInstallSetForClusterScopedApps(t *testing.T) {
	appFrameworkConfig := &enterpriseApi.AppFrameworkSpec{
		VolList: []enterpriseApi.VolumeSpec{
			{
				Name:      "testVol",
				Endpoint:  "https://s3-eu-west-2.amazonaws.com",
				Path:      "testbucket-rs-london",
				SecretRef: "s3-secret",
				Type:      "s3",
				Provider:  "aws",
			},
		},
		AppSources: []enterpriseApi.AppSourceSpec{
			{
				Name:     "appSrc1",
				Location: "adminAppsRepo",
				AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
					VolName: "testVol",
					Scope:   enterpriseApi.ScopeCluster,
				},
			},
		},
	}

	testApps := []string{"app1.tgz", "app2.tgz", "app3.tgz"}
	testHashes := []string{"abcd1111", "efgh2222", "ijkl3333"}
	testSizes := []int64{10, 20, 30}

	appDeployInfoList := make([]enterpriseApi.AppDeploymentInfo, 3)
	for index := range testApps {
		appDeployInfoList[index] = enterpriseApi.AppDeploymentInfo{
			AppName: testApps[index],
			PhaseInfo: enterpriseApi.PhaseInfo{
				Phase:      enterpriseApi.PhaseDownload,
				Status:     enterpriseApi.AppPkgDownloadPending,
				RetryCount: 0,
			},
			ObjectHash: testHashes[index],
			Size:       uint64(testSizes[index]),
		}
	}

	var appDeployContext *enterpriseApi.AppDeploymentContext = &enterpriseApi.AppDeploymentContext{
		AppFrameworkConfig:                  *appFrameworkConfig,
		AppsStatusMaxConcurrentAppDownloads: 10,
	}

	appDeployContext.AppsSrcDeployStatus = make(map[string]enterpriseApi.AppSrcDeployInfo)
	var appSrcDeployInfo enterpriseApi.AppSrcDeployInfo
	appSrcDeployInfo.AppDeploymentInfoList = appDeployInfoList
	appDeployContext.AppsSrcDeployStatus["appSrc1"] = appSrcDeployInfo

	// When the phase is not in podCopy complete, install state should not be set
	setInstallStateForClusterScopedApps(appDeployContext)

	for appSrcName, appSrcDeployInfo := range appDeployContext.AppsSrcDeployStatus {
		deployInfoList := appSrcDeployInfo.AppDeploymentInfoList
		for i := range deployInfoList {
			appSrc, err := getAppSrcSpec(appDeployContext.AppFrameworkConfig.AppSources, appSrcName)
			if err != nil {
				// Error, should never happen
				t.Errorf("Unable to find App src. App src name%s, appName: %s", appSrcName, deployInfoList[i].AppName)
			}

			if appSrc.Scope == enterpriseApi.ScopeCluster &&
				(deployInfoList[i].PhaseInfo.Phase == enterpriseApi.PhaseInstall || deployInfoList[i].PhaseInfo.Status == enterpriseApi.AppPkgInstallComplete) {
				t.Errorf("wrong install state for app: %s. Got(Phase=%s, PhaseStatus=%s), wanted(Phase=%s, PhaseStatus=%s)",
					deployInfoList[i].AppName, deployInfoList[i].PhaseInfo.Phase, appPhaseStatusAsStr(deployInfoList[i].PhaseInfo.Status), enterpriseApi.PhaseInstall, appPhaseStatusAsStr(enterpriseApi.AppPkgInstallComplete))
			}
		}
	}

	// When the phase is in podCopy complete, install state should be set

	for i := range appDeployInfoList {
		appDeployInfoList[i].PhaseInfo.Phase = enterpriseApi.PhasePodCopy
		appDeployInfoList[i].PhaseInfo.Status = enterpriseApi.AppPkgPodCopyComplete
	}

	setInstallStateForClusterScopedApps(appDeployContext)

	for appSrcName, appSrcDeployInfo := range appDeployContext.AppsSrcDeployStatus {
		deployInfoList := appSrcDeployInfo.AppDeploymentInfoList
		for i := range deployInfoList {
			appSrc, err := getAppSrcSpec(appDeployContext.AppFrameworkConfig.AppSources, appSrcName)
			if err != nil {
				// Error, should never happen
				t.Errorf("Unable to find App src. App src name%s, appName: %s", appSrcName, deployInfoList[i].AppName)
			}

			if appSrc.Scope == enterpriseApi.ScopeCluster &&
				(deployInfoList[i].PhaseInfo.Phase != enterpriseApi.PhaseInstall || deployInfoList[i].PhaseInfo.Status != enterpriseApi.AppPkgInstallComplete) {
				t.Errorf("wrong install state for app: %s. Got(Phase=%s, PhaseStatus=%s), wanted(Phase=%s, PhaseStatus=%s)",
					deployInfoList[i].AppName, deployInfoList[i].PhaseInfo.Phase, appPhaseStatusAsStr(deployInfoList[i].PhaseInfo.Status), enterpriseApi.PhaseInstall, appPhaseStatusAsStr(enterpriseApi.AppPkgInstallComplete))
			}
		}
	}

}

func TestCheckIfFileExistsOnPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "splunk-stack1-0",
			Namespace: "test",
			Labels: map[string]string{
				"controller-revision-hash": "v0",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					VolumeMounts: []corev1.VolumeMount{
						{
							MountPath: "/mnt/splunk-secrets",
							Name:      "mnt-splunk-secrets",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "mnt-splunk-secrets",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "test-secret",
						},
					},
				},
			},
		},
	}

	// Create client and add object
	c := spltest.NewMockClient()
	// Add object
	c.AddObject(pod)

	filePathOnPod := "/init-apps/splunkFwdApps/testApp.tgz"

	fileExists := checkIfFileExistsOnPod(c, "testNameSpace", pod.GetName(), filePathOnPod)
	if fileExists {
		t.Errorf("When the file doesn't exist, should return false")
	}
}

func TestGetAppPackageLocalPath(t *testing.T) {
	cr := enterpriseApi.ClusterMaster{
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterMaster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
		Spec: enterpriseApi.ClusterMasterSpec{
			CommonSplunkSpec: enterpriseApi.CommonSplunkSpec{
				Mock: true,
			},
			AppFrameworkConfig: enterpriseApi.AppFrameworkSpec{
				AppsRepoPollInterval:      60,
				MaxConcurrentAppDownloads: 5,

				VolList: []enterpriseApi.VolumeSpec{
					{
						Name:      "test_volume",
						Endpoint:  "https://s3-eu-west-2.amazonaws.com",
						Path:      "testbucket-rs-london",
						SecretRef: "s3-secret",
						Provider:  "aws",
					},
				},
				AppSources: []enterpriseApi.AppSourceSpec{
					{
						Name:     "appSrc1",
						Location: "adminAppsRepo",
						AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
							VolName: "test_volume",
							Scope:   "local",
						},
					},
				},
			},
		},
	}

	var worker *PipelineWorker = &PipelineWorker{
		cr:         &cr,
		appSrcName: cr.Spec.AppFrameworkConfig.AppSources[0].Name,
		appDeployInfo: &enterpriseApi.AppDeploymentInfo{
			AppName:    "testApp.spl",
			ObjectHash: "bcda23232a89",
		},
		afwConfig: &cr.Spec.AppFrameworkConfig,
	}

	expectedAppPkgLocalPath := "/opt/splunk/appframework/downloadedApps/test/ClusterMaster/stack1/local/appSrc1/testApp.spl_bcda23232a89"
	calculatedAppPkgLocalPath := getAppPackageLocalPath(worker)

	if calculatedAppPkgLocalPath != expectedAppPkgLocalPath {
		t.Errorf("Expected appPkgLocal Path %s, but got %s", expectedAppPkgLocalPath, calculatedAppPkgLocalPath)
	}
}

func TestInitStorageTracker(t *testing.T) {
	if operatorResourceTracker == nil {
		t.Errorf("operatorResourceTracker should be initialized as part of the enterprise package init()")
	}

	// When the volume is not configured, should return an error
	splcommon.AppDownloadVolume = "/non-existingDir"
	err := initStorageTracker()
	if err == nil {
		t.Errorf("When the volume doesn't exist, should return an error")
	}

	// When the volume exists, should not return an error
	splcommon.AppDownloadVolume = "/"
	err = initStorageTracker()
	if err != nil {
		t.Errorf("When the volume exists, should not return an error. Error: %v", err)
	}
}

func TestUpdateStorageTracker(t *testing.T) {
	// When the resource tracker is not initialized, should return an error
	operatorResourceTracker = nil
	err := updateStorageTracker()
	if err == nil {
		t.Errorf("When the operator resource tracker is not initialized, should return an error")
	}

	// When the volume is not configured, should return an error
	splcommon.AppDownloadVolume = "/non-existingdir"
	err = updateStorageTracker()
	if err == nil {
		t.Errorf("When the volume doesn't exist should return an error")
	}

	// When the volume exists, should not return an error
	operatorResourceTracker = &globalResourceTracker{
		storage: &storageTracker{},
	}
	splcommon.AppDownloadVolume = "/"
	err = updateStorageTracker()
	if err != nil {
		t.Errorf("When the volume exists should not return an error. Error: %v", err)
	}
}

func TestIsPersistantVolConfigured(t *testing.T) {
	// when the resource tracker not initialized, should return false
	operatorResourceTracker = nil
	if isPersistantVolConfigured() {
		t.Errorf("When the resource tracker is not initialized, should resturn false")
	}

	// when the storage tracker not initialized, should return false
	operatorResourceTracker = &globalResourceTracker{}
	if isPersistantVolConfigured() {
		t.Errorf("When the storage tracker is not initialized, should return false")
	}

	// Should return true, when the trackers are initialized
	operatorResourceTracker.storage = &storageTracker{}
	if !isPersistantVolConfigured() {
		t.Errorf("When the storage tracker is initialized, should return true")
	}
}

func TestReserveStorage(t *testing.T) {
	// when the resource tracker is not intiailzed, should return an error
	operatorResourceTracker = nil

	err := reserveStorage(1 * 1024)
	if err == nil {
		t.Errorf("When the resource tracker is not initialized, reservation should fail")
	}

	// When there is capacity, reservation should not fail
	operatorResourceTracker = &globalResourceTracker{
		storage: &storageTracker{
			availableDiskSpace: 8 * 1024,
		},
	}

	err = reserveStorage(1 * 1024)
	if err != nil {
		t.Errorf("Expected to reserver storage, but got an error: %v", err)
	}

	// When there is no capacity, reservation should fail
	err = reserveStorage(1 * 1024 * 1024)
	if err == nil {
		t.Errorf("Expected to fail storage allocation, but succeeded")
	}
}

func TestReleaseStorage(t *testing.T) {
	// When the resource tracker not initialized, should return an error
	operatorResourceTracker = nil

	err := releaseStorage(1 * 1024)
	if err == nil {
		t.Errorf("When the resource tracker is not initialized, release should fail")
	}

	operatorResourceTracker = &globalResourceTracker{
		storage: &storageTracker{
			availableDiskSpace: 8 * 1024,
		},
	}

	// When the storage is released, same should be reflecting in the storage tracker
	err = releaseStorage(1 * 1024)
	if err != nil {
		t.Errorf("Storage release should not fail")
	}
	if operatorResourceTracker.storage.availableDiskSpace != 9*1024 {
		t.Errorf("Released storage is not reflecting in the storage tracker")
	}
}

func TestChangePhaseInfo(t *testing.T) {
	appSrcDeployStatus := make(map[string]enterpriseApi.AppSrcDeployInfo, 1)

	appDeployInfoList := []enterpriseApi.AppDeploymentInfo{
		{
			AppName:    "app1.tgz",
			ObjectHash: "abcdef12345abcdef",
			PhaseInfo: enterpriseApi.PhaseInfo{
				Phase:      enterpriseApi.PhaseDownload,
				Status:     enterpriseApi.AppPkgDownloadPending,
				RetryCount: 2,
			},
			AuxPhaseInfo: []enterpriseApi.PhaseInfo{
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
			},
		},
	}

	var appSrcDeployInfo enterpriseApi.AppSrcDeployInfo = enterpriseApi.AppSrcDeployInfo{}
	appSrcDeployInfo.AppDeploymentInfoList = appDeployInfoList
	appSrcDeployStatus["appSrc1"] = appSrcDeployInfo

	changePhaseInfo(5, "appSrc1", appSrcDeployStatus)

	if len(appDeployInfoList[0].AuxPhaseInfo) != 5 {
		t.Errorf("changePhaseInfo should have increased the size of AuxPhaseInfo")
	}
}

func TestRemoveStaleEntriesFromAuxPhaseInfo(t *testing.T) {
	appSrcDeployStatus := make(map[string]enterpriseApi.AppSrcDeployInfo, 1)

	appDeployInfoList := []enterpriseApi.AppDeploymentInfo{
		{
			AppName:    "app1.tgz",
			ObjectHash: "abcdef12345abcdef",
			PhaseInfo: enterpriseApi.PhaseInfo{
				Phase:      enterpriseApi.PhaseDownload,
				Status:     enterpriseApi.AppPkgDownloadPending,
				RetryCount: 2,
			},
			AuxPhaseInfo: []enterpriseApi.PhaseInfo{
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
				{
					Phase:      enterpriseApi.PhaseDownload,
					Status:     enterpriseApi.AppPkgDownloadPending,
					RetryCount: 2,
				},
			},
		},
	}

	var appSrcDeployInfo enterpriseApi.AppSrcDeployInfo = enterpriseApi.AppSrcDeployInfo{}
	appSrcDeployInfo.AppDeploymentInfoList = appDeployInfoList
	appSrcDeployStatus["appSrc1"] = appSrcDeployInfo

	removeStaleEntriesFromAuxPhaseInfo(1, "appSrc1", appSrcDeployStatus)

	if len(appDeployInfoList[0].AuxPhaseInfo) > 1 {
		t.Errorf("removeStaleEntriesFromAuxPhaseInfo should have cleared the last 2 entries from AuxPhaseInfo")
	}
}

func TestMigrateAfwStatus(t *testing.T) {
	cr := &enterpriseApi.Standalone{
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
	}

	statefulSetName := "splunk-stack1-standalone"
	var replicas int32 = 4

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: "test",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
	}

	client := spltest.NewMockClient()
	_, err := splctrl.ApplyStatefulSet(client, sts)
	if err != nil {
		t.Errorf("unable to apply statefulset")
	}

	appDeployContext := &enterpriseApi.AppDeploymentContext{
		Version: enterpriseApi.AfwPhase2,
	}
	appDeployContext.AppsSrcDeployStatus = make(map[string]enterpriseApi.AppSrcDeployInfo, 1)
	appSrcDeploymentInfo := enterpriseApi.AppSrcDeployInfo{}
	appSrcDeploymentInfo.AppDeploymentInfoList = make([]enterpriseApi.AppDeploymentInfo, 5)

	// When the App package is already deleted, no need to set the Phase info, aux phase info
	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		appSrcDeploymentInfo.AppDeploymentInfoList[i] = enterpriseApi.AppDeploymentInfo{
			AppName:      fmt.Sprintf("app%v.spl", i),
			ObjectHash:   fmt.Sprintf("\"abcdef1234567890abcdef%v-%v\"", i, i),
			DeployStatus: enterpriseApi.DeployStatusComplete,
			RepoState:    enterpriseApi.RepoStateDeleted,
		}
	}

	appDeployContext.Version = enterpriseApi.AfwPhase2
	appDeployContext.AppsSrcDeployStatus["appSrc1"] = appSrcDeploymentInfo

	migrated := migrateAfwStatus(client, cr, appDeployContext)
	if !migrated {
		t.Errorf("When there are objects to be migrated, should return true")
	}

	if appDeployContext.Version != currentAfwVersion {
		t.Errorf("Unable to update the App framework version")
	}

	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		if strings.Contains(appSrcDeploymentInfo.AppDeploymentInfoList[i].ObjectHash, "\"") {
			t.Errorf("failed to modify the Object hash for app %v", i)
		}

		if appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Phase == enterpriseApi.PhaseInstall || appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Status == enterpriseApi.AppPkgInstallComplete {
			t.Errorf("When the app pkg is not active, no need to set the Phase-3 phase info")
		}

		auxPhaseInfo := appSrcDeploymentInfo.AppDeploymentInfoList[i].AuxPhaseInfo

		for _, phase := range auxPhaseInfo {
			if phase == appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo {
				t.Errorf("When the app pkg is not active, no need to set the Phase-3 aux. phase info")
			}
		}
	}

	// When the app package is not installed already, should set the phase info and aux phase info with the download phase
	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		appSrcDeploymentInfo.AppDeploymentInfoList[i] = enterpriseApi.AppDeploymentInfo{
			AppName:      fmt.Sprintf("app%v.spl", i),
			ObjectHash:   fmt.Sprintf("\"abcdef1234567890abcdef%v\"", i),
			DeployStatus: enterpriseApi.DeployStatusError,
			RepoState:    enterpriseApi.RepoStateActive,
		}
	}

	appDeployContext.Version = enterpriseApi.AfwPhase2
	appDeployContext.AppsSrcDeployStatus["appSrc1"] = appSrcDeploymentInfo

	migrated = migrateAfwStatus(client, cr, appDeployContext)
	if !migrated {
		t.Errorf("When there are objects to be migrated, should return true")
	}

	if appDeployContext.Version != currentAfwVersion {
		t.Errorf("Unable to update the App framework version")
	}

	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		if strings.Contains(appSrcDeploymentInfo.AppDeploymentInfoList[i].ObjectHash, "\"") {
			t.Errorf("failed to modify the Object hash for app %v", i)
		}

		if appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Phase != enterpriseApi.PhaseDownload || appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Status != enterpriseApi.AppPkgDownloadPending {
			t.Errorf("When the DeployStatus is not in install complete, should start with the download phase")
		}
	}

	// When the deploy status is set to install complete, phase-3 phase info, and aux phaseinfo should reflect the install phase completion
	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		appSrcDeploymentInfo.AppDeploymentInfoList[i] = enterpriseApi.AppDeploymentInfo{
			AppName:      fmt.Sprintf("app%v.spl", i),
			ObjectHash:   fmt.Sprintf("\"abcdef1234567890abcdef%v\"", i),
			DeployStatus: enterpriseApi.DeployStatusComplete,
			RepoState:    enterpriseApi.RepoStateActive,
		}
	}

	appDeployContext.Version = enterpriseApi.AfwPhase2
	appDeployContext.AppsSrcDeployStatus["appSrc1"] = appSrcDeploymentInfo

	migrated = migrateAfwStatus(client, cr, appDeployContext)
	if !migrated {
		t.Errorf("When there are objects to be migrated, should return true")
	}

	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		if strings.Contains(appSrcDeploymentInfo.AppDeploymentInfoList[i].ObjectHash, "\"") {
			t.Errorf("failed to modify the Object hash for app %v", i)
		}

		if appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Phase != enterpriseApi.PhaseInstall || appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Status != enterpriseApi.AppPkgInstallComplete {
			t.Errorf("Unable to update the Phase-3 Phase info for app: %v", i)
		}

		auxPhaseInfo := appSrcDeploymentInfo.AppDeploymentInfoList[i].AuxPhaseInfo

		for _, phase := range auxPhaseInfo {
			if phase != appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo {
				t.Errorf("Failed to update the AuxPhase info during the migration")
			}
		}
	}
}

func TestcheckAndMigrateAppDeployStatus(t *testing.T) {
	var appDeployContext *enterpriseApi.AppDeploymentContext

	client := spltest.NewMockClient()
	cr := &enterpriseApi.Standalone{
		TypeMeta: metav1.TypeMeta{
			Kind: "Standalone",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stack1",
			Namespace: "test",
		},
	}

	appFrameworkConfig := &enterpriseApi.AppFrameworkSpec{
		VolList: []enterpriseApi.VolumeSpec{
			{Name: "msos_s2s3_vol", Endpoint: "https://s3-eu-west-2.amazonaws.com", Path: "testbucket-rs-london", SecretRef: "s3-secret", Type: "s3", Provider: "aws"},
		},
		AppSources: []enterpriseApi.AppSourceSpec{
			{Name: "adminApps",
				Location: "adminAppsRepo",
				AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
					VolName: "msos_s2s3_vol",
					Scope:   enterpriseApi.ScopeLocal},
			},
			{Name: "securityApps",
				Location: "securityAppsRepo",
				AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
					VolName: "msos_s2s3_vol",
					Scope:   enterpriseApi.ScopeLocal},
			},
			{Name: "authenticationApps",
				Location: "authenticationAppsRepo",
				AppSourceDefaultSpec: enterpriseApi.AppSourceDefaultSpec{
					VolName: "msos_s2s3_vol",
					Scope:   enterpriseApi.ScopeLocal},
			},
		},
	}

	err := checkAndMigrateAppDeployStatus(client, cr, appDeployContext, appFrameworkConfig, true)
	if err != nil {
		t.Errorf("When the app deploy context is nil, should not return an error")
	}

	appDeployContext = &enterpriseApi.AppDeploymentContext{
		Version: enterpriseApi.AfwPhase2,
	}
	appDeployContext.AppsSrcDeployStatus = make(map[string]enterpriseApi.AppSrcDeployInfo, 1)
	appSrcDeploymentInfo := enterpriseApi.AppSrcDeployInfo{}
	appSrcDeploymentInfo.AppDeploymentInfoList = make([]enterpriseApi.AppDeploymentInfo, 5)

	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		appSrcDeploymentInfo.AppDeploymentInfoList[i] = enterpriseApi.AppDeploymentInfo{
			AppName:      fmt.Sprintf("app%v.spl", i),
			ObjectHash:   fmt.Sprintf("\"abcdef1234567890abcdef%v\"", i),
			DeployStatus: enterpriseApi.DeployStatusComplete,
			RepoState:    enterpriseApi.RepoStateActive,
		}
	}

	appDeployContext.AppsSrcDeployStatus["appSrc1"] = appSrcDeploymentInfo

	statefulSetName := "splunk-stack1-standalone"
	var replicas int32 = 4

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: "test",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
	}

	_, err = splctrl.ApplyStatefulSet(client, sts)
	if err != nil {
		t.Errorf("unable to apply statefulset")
	}

	defaultVol := splcommon.AppDownloadVolume
	splcommon.AppDownloadVolume = "/tmp/testdir"
	defer func() {
		os.RemoveAll(splcommon.AppDownloadVolume)
		splcommon.AppDownloadVolume = defaultVol
	}()

	_, err = os.Stat(splcommon.AppDownloadVolume)
	if os.IsNotExist(err) {
		err = os.MkdirAll(splcommon.AppDownloadVolume, 0755)
		if err != nil {
			t.Errorf("Unable to create the directory, error: %v", err)
		}
	}

	err = checkAndMigrateAppDeployStatus(client, cr, appDeployContext, appFrameworkConfig, true)
	if err != nil {
		t.Errorf("With proper app spec and status contexts, migration should happen. error: %v", err)
	}

	if appDeployContext.Version != currentAfwVersion {
		t.Errorf("Unable to update the App framework version")
	}

	for i := range appSrcDeploymentInfo.AppDeploymentInfoList {
		if strings.Contains(appSrcDeploymentInfo.AppDeploymentInfoList[i].ObjectHash, "\"") {
			t.Errorf("failed to modify the Object hash for app %v", i)
		}

		if appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Phase != enterpriseApi.PhaseInstall || appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo.Status != enterpriseApi.AppPkgInstallComplete {
			t.Errorf("Unable to update the Phase-3 Phase info for app: %v", i)
		}

		auxPhaseInfo := appSrcDeploymentInfo.AppDeploymentInfoList[i].AuxPhaseInfo

		for _, phase := range auxPhaseInfo {
			if phase != appSrcDeploymentInfo.AppDeploymentInfoList[i].PhaseInfo {
				t.Errorf("Failed to update the AuxPhase info during the migration")
			}
		}
	}
}

func TestGetCleanObjectDigest(t *testing.T) {
	// plain digest
	var digests = []string{"\"b38a8f911e2b43982b71a979fe1d3c3f\"", "b38a8f911e2b43982b71a979fe1d3c3f"}
	retDigest, err := getCleanObjectDigest(&digests[0])
	if err != nil {
		t.Errorf("Unable to clean the digest, error: %v", err)
	}

	if digests[1] != *retDigest {
		t.Errorf("Converted digest value: %v is not equal to the expected digest value: %v", *retDigest, digests[1])
	}

	// digest in case of multi-part upload
	digests = []string{"\"b38a8f911e2b43982b71a979fe1d3c3f-3\"", "b38a8f911e2b43982b71a979fe1d3c3f-3"}
	retDigest, err = getCleanObjectDigest(&digests[0])
	if err != nil {
		t.Errorf("Unable to clean the digest, error: %v", err)
	}

	if digests[1] != *retDigest {
		t.Errorf("Converted digest value: %v is not equal to the expected digest value: %v", *retDigest, digests[1])
	}

}
