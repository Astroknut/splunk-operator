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
	"path/filepath"
	"sync"
	"time"

	enterpriseApi "github.com/splunk/splunk-operator/pkg/apis/enterprise/v2"
	splcommon "github.com/splunk/splunk-operator/pkg/splunk/common"
	splctrl "github.com/splunk/splunk-operator/pkg/splunk/controller"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// handle to hold the pipeline
var afwPipeline *AppInstallPipeline

// createPipelineWorker creates a pipeline worker for an app package
func createPipelineWorker(appDeployInfo *enterpriseApi.AppDeploymentInfo, appSrcName string, podName string,
	appFrameworkConfig *enterpriseApi.AppFrameworkSpec, appDeployContext *enterpriseApi.AppDeploymentContext,
	client *splcommon.ControllerClient, cr splcommon.MetaObject, statefulSet *appsv1.StatefulSet) *PipelineWorker {
	return &PipelineWorker{
		appDeployInfo:    appDeployInfo,
		appSrcName:       appSrcName,
		targetPodName:    podName,
		afwConfig:        appFrameworkConfig,
		afwDeployContext: appDeployContext,
		client:           client,
		cr:               cr,
		sts:              statefulSet,
	}
}

// createAndAddAWorker used to add a worker to the pipeline on reconcile re-entry
func (ppln *AppInstallPipeline) createAndAddPipelineWorker(phase enterpriseApi.AppPhaseType, appDeployInfo *enterpriseApi.AppDeploymentInfo,
	appSrcName string, podName string, appFrameworkConfig *enterpriseApi.AppFrameworkSpec, appDeployContext *enterpriseApi.AppDeploymentContext,
	client splcommon.ControllerClient, cr splcommon.MetaObject, statefulSet *appsv1.StatefulSet) {

	scopedLog := log.WithName("createAndAddPipelineWorker").WithValues("name", cr.GetName(), "namespace", cr.GetNamespace())
	worker := createPipelineWorker(appDeployInfo, appSrcName, podName, appFrameworkConfig, appDeployContext, &client, cr, statefulSet)

	if worker != nil {
		scopedLog.Info("Created new worker", "Pod name", worker.targetPodName, "App name", appDeployInfo.AppName, "digest", appDeployInfo.ObjectHash, "phase", appDeployInfo.PhaseInfo.Phase)
		ppln.addWorkersToPipelinePhase(phase, worker)
	}
}

// getApplicablePodNameForWorker gets the Pod name relevant for the CR under work
func getApplicablePodNameForWorker(cr splcommon.MetaObject, podID int) string {
	var podType string

	switch cr.GetObjectKind().GroupVersionKind().Kind {
	case "Standalone":
		podType = "standalone"
	case "LicenseMaster":
		podType = "license-master"
	case "SearchHeadCluster":
		podType = "deployer"
	case "IndexerCluster":
		// error?
	case "ClusterMaster":
		podType = "cluster-master"
	case "MonitoringConsole":
		podType = "monitoring-console"
	}

	return fmt.Sprintf("splunk-%s-%s-%d", cr.GetName(), podType, podID)
}

// addWorkersToPipelinePhase adds a worker to a given pipeline phase
func (ppln *AppInstallPipeline) addWorkersToPipelinePhase(phaseID enterpriseApi.AppPhaseType, workers ...*PipelineWorker) {
	scopedLog := log.WithName("addWorkersToPipelinePhase").WithValues("phase", phaseID)

	for _, worker := range workers {
		scopedLog.Info("Adding worker", "name", worker.cr.GetName(), "namespace", worker.cr.GetNamespace(), "Pod name", worker.targetPodName, "App name", worker.appDeployInfo.AppName, "digest", worker.appDeployInfo.ObjectHash)
	}
	ppln.pplnPhases[phaseID].mutex.Lock()
	ppln.pplnPhases[phaseID].q = append(ppln.pplnPhases[phaseID].q, workers...)
	ppln.pplnPhases[phaseID].mutex.Unlock()
}

// deleteWorkerFromPipelinePhase deletes a given worker from a pipeline phase
func (ppln *AppInstallPipeline) deleteWorkerFromPipelinePhase(phaseID enterpriseApi.AppPhaseType, worker *PipelineWorker) bool {
	scopedLog := log.WithName("deleteWorkerFromPipelinePhase").WithValues("phase", phaseID)
	ppln.pplnPhases[phaseID].mutex.Lock()
	defer ppln.pplnPhases[phaseID].mutex.Unlock()

	phaseQ := ppln.pplnPhases[phaseID].q
	for i, qWorker := range phaseQ {
		if worker == qWorker {
			if i == 0 {
				phaseQ = phaseQ[1:]
			} else {
				if i != len(phaseQ)-1 {
					phaseQ[i] = phaseQ[len(phaseQ)-1]
				}
				phaseQ = phaseQ[:len(phaseQ)-1]
			}
			ppln.pplnPhases[phaseID].q = phaseQ
			scopedLog.Info("Deleted worker", "name", worker.cr.GetName(), "namespace", worker.cr.GetNamespace(), "Pod name", worker.targetPodName, "App name", worker.appDeployInfo.AppName, "digest", worker.appDeployInfo.ObjectHash)
			return true
		}
	}
	return false
}

// TransitionWorkerPhase transition a worker to new phase, and deletes from the current phase
// In the case of Standalone CR with multiple replicas, Fan-out `replicas` number of new workers
func (ppln *AppInstallPipeline) TransitionWorkerPhase(worker *PipelineWorker, currentPhase, nextPhase enterpriseApi.AppPhaseType) {
	kind := worker.cr.GetObjectKind().GroupVersionKind().Kind

	scopedLog := log.WithName("TransitionWorkerPhase").WithValues("name", worker.cr.GetName(), "namespace", worker.cr.GetNamespace(), "App name", worker.appDeployInfo.AppName, "digest", worker.appDeployInfo.ObjectHash, "pod name", worker.targetPodName, "current Phase", currentPhase, "next phase", nextPhase)

	appDeployInfo := worker.appDeployInfo
	var replicaCount int32
	if worker.sts != nil {
		replicaCount = *worker.sts.Spec.Replicas
	} else {
		replicaCount = 1
	}

	// For now Standalone is the only CR unique with multiple replicas that is applicable for the AFW
	// If the replica count is more than 1, and if it is Standalone, when transitioning from
	// download phase, create a separate worker for the Pod copy(which also transition to install worker)

	// Also, for whatever reason(say, standalone reset, and that way it lost the app package), if the Standalone
	// switches to download phase, once the download phase is complete, it will safely schedule a new pod copy worker,
	// without affecting other pods.
	if replicaCount == 1 || currentPhase != enterpriseApi.PhaseDownload {
		scopedLog.Info("Simple transition")
		appDeployInfo.PhaseInfo = enterpriseApi.PhaseInfo{
			Phase:      nextPhase,
			RetryCount: 0,
		}

		if nextPhase == enterpriseApi.PhaseDownload {
			appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgDownloadPending
		} else if nextPhase == enterpriseApi.PhasePodCopy {
			appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgPodCopyPending
		} else if nextPhase == enterpriseApi.PhaseInstall {
			appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgInstallPending
		}
		//Re-use the same worker
		worker.isActive = false
		worker.waiter = nil
		ppln.addWorkersToPipelinePhase(nextPhase, worker)
	} else if currentPhase == enterpriseApi.PhaseDownload && kind == "Standalone" {
		scopedLog.Info("Fan-out transition")
		var copyWorkers, installWorkers []*PipelineWorker

		// TBD, @Gaurav, As part of CSPL-1169, plz make sure that we are dealing with the right replica count in case of the scale-up scenario
		// Seems like the download just finished. Allocate Phase info
		if len(appDeployInfo.AuxPhaseInfo) == 0 {
			scopedLog.Info("Just finished the download phase")
			// Create Phase info for all the statefulset Pods.
			appDeployInfo.AuxPhaseInfo = make([]enterpriseApi.PhaseInfo, replicaCount)

			// Create a slice of corresponding worker nodes
			copyWorkers = make([]*PipelineWorker, replicaCount)

			//Create the Aux PhaseInfo for tracking all the Standalone Pods
			for i := range appDeployInfo.AuxPhaseInfo {
				appDeployInfo.AuxPhaseInfo[i] = enterpriseApi.PhaseInfo{
					Phase:  enterpriseApi.PhasePodCopy,
					Status: enterpriseApi.AppPkgPodCopyPending,
				}

				// Create a new copy
				copyWorkers[i] = &PipelineWorker{}
				*copyWorkers[i] = *worker
				copyWorkers[i].targetPodName = getApplicablePodNameForWorker(worker.cr, i)
				copyWorkers[i].isActive = false
				copyWorkers[i].waiter = nil
				scopedLog.Info("Created a new fan-out pod copy worker", "pod name", worker.targetPodName)
			}
		} else {
			scopedLog.Info("Installation is already in progress for replica members")
			for i, phaseInfo := range appDeployInfo.AuxPhaseInfo {
				newWorker := *worker
				newWorker.targetPodName = getApplicablePodNameForWorker(worker.cr, i)
				newWorker.isActive = false
				newWorker.waiter = nil

				if phaseInfo.Phase == enterpriseApi.PhaseInstall && phaseInfo.Status != enterpriseApi.AppPkgInstallComplete {
					// If the install is already complete for that app, nothing to be done
					scopedLog.Info("Created a new fan-out pod copy worker", "pod name", worker.targetPodName)
					installWorkers = append(installWorkers, &newWorker)
				} else {
					//sgontla: check here
					scopedLog.Info("Created a new fan-out install worker", "pod name", worker.targetPodName)
					copyWorkers = append(copyWorkers, &newWorker)
				}
			}

		}

		ppln.addWorkersToPipelinePhase(enterpriseApi.PhasePodCopy, copyWorkers...)
		ppln.addWorkersToPipelinePhase(enterpriseApi.PhaseInstall, installWorkers...)
	}

	// We have already moved the worker(s) to the required queue.
	// Now, safely delete the worker from the current phase queue
	scopedLog.Info("Deleted worker", "phase", currentPhase)
	ppln.deleteWorkerFromPipelinePhase(currentPhase, worker)
}

// updatePplnWorkerPhaseInfo updates the in-memory PhaseInfo
func updatePplnWorkerPhaseInfo(appDeployInfo *enterpriseApi.AppDeploymentInfo, phaseType enterpriseApi.AppPhaseType, retryCount int32, statusType enterpriseApi.AppPhaseStatusType) {
	scopedLog := log.WithName("updatePplnWorkerPhaseInfo").WithValues("appName", appDeployInfo.AppName)

	scopedLog.Info("changing the status", "old status", appPhaseStatusAsStr(appDeployInfo.PhaseInfo.Status), "new status", appPhaseStatusAsStr(statusType))
	appDeployInfo.PhaseInfo.Phase = phaseType
	appDeployInfo.PhaseInfo.RetryCount = retryCount
	appDeployInfo.PhaseInfo.Status = statusType
}

// Download API will do the actual work of downloading apps from remote storage
func (pplnWorker *PipelineWorker) Download(activeWorkers *uint64, localPath string) {

	defer func() {
		// decrement the number of active workers
		pplnWorker.workerMutex.Lock()
		*activeWorkers--
		pplnWorker.workerMutex.Unlock()

		// decrement the waiter count
		pplnWorker.waiter.Done()
	}()

	splunkCR := pplnWorker.cr
	appSrcName := pplnWorker.appSrcName

	scopedLog := log.WithName("PipelineWorker.Download()").WithValues("name", splunkCR.GetName(), "namespace", splunkCR.GetNamespace(), "App name", pplnWorker.appDeployInfo.AppName, "objectHash", pplnWorker.appDeployInfo.ObjectHash)

	appDeployInfo := pplnWorker.appDeployInfo

	s3ClientMgr, _ := getS3ClientMgr(*pplnWorker.client, pplnWorker.cr, pplnWorker.afwConfig, appSrcName)
	appName := appDeployInfo.AppName

	localFile := getLocalAppFileName(localPath, appName, appDeployInfo.ObjectHash)
	remoteFile, err := getRemoteObjectKey(splunkCR, pplnWorker.afwConfig, appSrcName, appName)
	if err != nil {
		scopedLog.Error(err, "unable to get remote object key", "appName", appName)
		// increment the retry count and mark this app as download pending
		updatePplnWorkerPhaseInfo(appDeployInfo, enterpriseApi.PhaseDownload, appDeployInfo.PhaseInfo.RetryCount+1, enterpriseApi.AppPkgDownloadPending)
		return
	}

	// download the app from remote storage
	err = s3ClientMgr.DownloadApp(remoteFile, localFile, appDeployInfo.ObjectHash)
	if err != nil {
		scopedLog.Error(err, "unable to download app", "appName", appName)
		// increment the retry count and mark this app as download pending
		updatePplnWorkerPhaseInfo(appDeployInfo, enterpriseApi.PhaseDownload, appDeployInfo.PhaseInfo.RetryCount+1, enterpriseApi.AppPkgDownloadPending)
		return
	} else {
		// download is successfull, update the state and reset the retry count
		updatePplnWorkerPhaseInfo(appDeployInfo, enterpriseApi.PhaseDownload, 0, enterpriseApi.AppPkgDownloadComplete)
	}

	scopedLog.Info("Finished downloading app")
}

// scheduleDownloads schedules the download workers to download app/s
func (ppln *AppInstallPipeline) scheduleDownloads(pplnPhase *PipelinePhase, maxWorkers uint64, sigTerm <-chan bool, downloadPhaseWaiter *sync.WaitGroup) {
	var activeWorkers uint64

	scopedLog := log.WithName("scheduleDownloads")

downloadWork:
	for {
		if activeWorkers <= maxWorkers {
			select {
			case <-sigTerm:
				break downloadWork

			case downloadWorker := <-pplnPhase.msgChannel:

				// do not redownload the app if it is already downloaded
				if isAppAlreadyDownloaded(downloadWorker) {
					scopedLog.Info("app is already downloaded on operator pod, hence skipping it.", "appSrcName", downloadWorker.appSrcName, "appName", downloadWorker.appDeployInfo.AppName)
				}

				// do not proceed if we dont have enough disk space to download this app
				if ppln.availableDiskSpace-downloadWorker.appDeployInfo.Size <= 0 {
					// put this worker back in the channel to be picked up later
					pplnPhase.msgChannel <- downloadWorker
					continue
				}
				// increment the number of active workers
				activeWorkers++

				// increment the count in worker waitgroup
				downloadWorker.waiter.Add(1)

				// update the available disk space
				ppln.availableDiskSpace = ppln.availableDiskSpace - downloadWorker.appDeployInfo.Size

				// update the download state of app to be DownloadInProgress
				updatePplnWorkerPhaseInfo(downloadWorker.appDeployInfo, enterpriseApi.PhaseDownload, downloadWorker.appDeployInfo.PhaseInfo.RetryCount, enterpriseApi.AppPkgDownloadInProgress)

				scope := getAppSrcScope(downloadWorker.afwConfig, downloadWorker.appSrcName)

				kind := downloadWorker.cr.GetObjectKind().GroupVersionKind().Kind

				// This is how the path to download apps looks like -
				// /opt/splunk/appframework/downloadedApps/<namespace>/<CR_Kind>/<CR_Name>/<scope>/<appSrc_Name>/
				// For e.g., if the we are trying to download app app1.tgz under "admin" app source name, for a Standalone CR with name "stand1"
				// in default namespace, then it will be downloaded at the path -
				// /opt/splunk/appframework/downloadedApps/default/Standalone/stand1/local/admin/app1.tgz_<hash>
				localPath := filepath.Join(splcommon.AppDownloadVolume, "downloadedApps", downloadWorker.cr.GetNamespace(), kind, downloadWorker.cr.GetName(), scope, downloadWorker.appSrcName) + "/"

				appDeployInfo := downloadWorker.appDeployInfo

				// create the sub-directories on the volume for downloading scoped apps
				err := createAppDownloadDir(localPath)
				if err != nil {
					// increment the retry count and mark this app as download pending
					appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgDownloadError
					updatePplnWorkerPhaseInfo(appDeployInfo, enterpriseApi.PhaseDownload, appDeployInfo.PhaseInfo.RetryCount+1, enterpriseApi.AppPkgDownloadPending)
					continue
				}

				// start the actual download
				go downloadWorker.Download(&activeWorkers, localPath)

			default:
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	// wait for all the download threads to finish
	pplnPhase.workerWaiter.Wait()

	// we are done processing download jobs
	downloadPhaseWaiter.Done()
}

//downloadPhaseManager creates download phase manager for the install pipeline
func (ppln *AppInstallPipeline) downloadPhaseManager(phaseWaiter *sync.WaitGroup, sigTerm <-chan bool, appDeployContext *enterpriseApi.AppDeploymentContext) {
	scopedLog := log.WithName("downloadPhaseManager")
	scopedLog.Info("Starting Download phase manager")
	pplnPhase := ppln.pplnPhases[enterpriseApi.PhaseDownload]

	maxWorkers := appDeployContext.AppsStatusMaxConcurrentAppDownloads

	downloadPhaseWaiter := new(sync.WaitGroup)

	downloadPhaseWaiter.Add(1)
	// schedule the download threads to do actual download work
	go ppln.scheduleDownloads(pplnPhase, maxWorkers, sigTerm, downloadPhaseWaiter)

downloadPhase:
	for {
		select {
		case <-sigTerm:
			scopedLog.Info("Received the termination request from the scheduler")
			break downloadPhase

		default:
			for _, downloadWorker := range pplnPhase.q {
				phaseInfo := downloadWorker.appDeployInfo.PhaseInfo
				if !downloadWorker.isActive && phaseInfo.RetryCount < pipelinePhaseMaxRetryCount && phaseInfo.Status != enterpriseApi.AppPkgDownloadComplete {
					downloadWorker.waiter = &pplnPhase.workerWaiter
					pplnPhase.msgChannel <- downloadWorker

					scopedLog.Info("Download worker got a run slot", "name", downloadWorker.cr.GetName(), "namespace", downloadWorker.cr.GetNamespace(), "App name", downloadWorker.appDeployInfo.AppName, "digest", downloadWorker.appDeployInfo.ObjectHash)
					downloadWorker.isActive = true
				} else if downloadWorker.appDeployInfo.PhaseInfo.Status == enterpriseApi.AppPkgDownloadComplete {
					ppln.TransitionWorkerPhase(downloadWorker, enterpriseApi.PhaseDownload, enterpriseApi.PhasePodCopy)
				} else if phaseInfo.RetryCount >= pipelinePhaseMaxRetryCount {
					downloadWorker.appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgDownloadError
					ppln.deleteWorkerFromPipelinePhase(phaseInfo.Phase, downloadWorker)
				}
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	// First wait for my all download workers to finish
	scopedLog.Info("Wating for the download workers to finish")
	downloadPhaseWaiter.Wait()

	scopedLog.Info("All the download workers finished")
	// Signal that the download phase is complete
	phaseWaiter.Done()
}

// podCopyPhaseManager creates pod copy phase manager for the install pipeline
func (ppln *AppInstallPipeline) podCopyPhaseManager(phaseWaiter *sync.WaitGroup, sigTerm <-chan bool, appDeployContext *enterpriseApi.AppDeploymentContext) {
	scopedLog := log.WithName("podCopyPhaseManager")
	scopedLog.Info("Starting Pod copy phase manager")
	pplnPhase := ppln.pplnPhases[enterpriseApi.PhasePodCopy]

podCopyPhase:
	for {
		select {
		case <-sigTerm:
			scopedLog.Info("Received the termination request from the scheduler")
			break podCopyPhase

		default:
		schedulePodCopy:
			for _, podCopyWorker := range pplnPhase.q {
				phaseInfo := &podCopyWorker.appDeployInfo.PhaseInfo
				if phaseInfo.RetryCount < pipelinePhaseMaxRetryCount && !podCopyWorker.isActive && phaseInfo.Status != enterpriseApi.AppPkgPodCopyComplete {
					podCopyWorker.waiter = &pplnPhase.workerWaiter
					select {
					case pplnPhase.msgChannel <- podCopyWorker:
						scopedLog.Info("Pod copy worker got a run slot", "name", podCopyWorker.cr.GetName(), "namespace", podCopyWorker.cr.GetNamespace(), "pod name", podCopyWorker.targetPodName, "App name", podCopyWorker.appDeployInfo.AppName, "digest", podCopyWorker.appDeployInfo.ObjectHash)
						//pplnPhase.workerWaiter.Add(1)
						podCopyWorker.isActive = true
					default:
						podCopyWorker.waiter = nil
						break schedulePodCopy
					}
				} else if phaseInfo.Status == enterpriseApi.AppPkgPodCopyComplete {
					appSrc, err := getAppSrcSpec(podCopyWorker.afwConfig.AppSources, podCopyWorker.appSrcName)
					if err != nil {
						// Error, should never happen
						scopedLog.Error(err, "Unable to find the App source", "app src name", appSrc)
						continue
					}

					// If cluster scoped apps, don't do any thing, just delete the worker. Yield logic knows when to push the bundle
					if appSrc.Scope != enterpriseApi.ScopeCluster {
						ppln.TransitionWorkerPhase(podCopyWorker, enterpriseApi.PhasePodCopy, enterpriseApi.PhaseInstall)
					}
				} else if phaseInfo.RetryCount >= pipelinePhaseMaxRetryCount {
					podCopyWorker.appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgPodCopyError
					ppln.deleteWorkerFromPipelinePhase(phaseInfo.Phase, podCopyWorker)
				}
			}

		}

		time.Sleep(200 * time.Millisecond)
	}
	scopedLog.Info("Wating for the download workers to finish")
	// First wait for my all pod copy workers to finish
	pplnPhase.workerWaiter.Wait()

	//Singal that the Pod copy manager is done
	scopedLog.Info("All the download workers finished")
	phaseWaiter.Done()
}

// installPhaseManager creates install phase manager for the afw installation pipeline
func (ppln *AppInstallPipeline) installPhaseManager(phaseWaiter *sync.WaitGroup, sigTerm <-chan bool, appDeployContext *enterpriseApi.AppDeploymentContext) {
	scopedLog := log.WithName("installPhaseManager")
	scopedLog.Info("Starting Install phase manager")
	pplnPhase := ppln.pplnPhases[enterpriseApi.PhaseInstall]
installPhase:
	for {
		select {
		case <-sigTerm:
			scopedLog.Info("Received the termination request from the scheduler")
			break installPhase

		default:
		scheduleInstalls:
			for _, installWorker := range pplnPhase.q {
				phaseInfo := installWorker.appDeployInfo.PhaseInfo
				if phaseInfo.RetryCount < pipelinePhaseMaxRetryCount && !installWorker.isActive && phaseInfo.Status != enterpriseApi.AppPkgInstallComplete {
					installWorker.waiter = &pplnPhase.workerWaiter
					select {
					case pplnPhase.msgChannel <- installWorker:
						scopedLog.Info("Install worker got a run slot", "name", installWorker.cr.GetName(), "namespace", installWorker.cr.GetNamespace(), "pod name", installWorker.targetPodName, "App name", installWorker.appDeployInfo.AppName, "digest", installWorker.appDeployInfo.ObjectHash)
						//pplnPhase.workerWaiter.Add(1)
						installWorker.isActive = true
					default:
						installWorker.waiter = nil
						break scheduleInstalls
					}
				} else if phaseInfo.Status == enterpriseApi.AppPkgInstallComplete {
					if installWorker.cr.GetObjectKind().GroupVersionKind().Kind == "Standalone" && isAppInstallationCompleteOnStandaloneReplicas(installWorker.appDeployInfo.AuxPhaseInfo) {
						installWorker.appDeployInfo.PhaseInfo.Phase = enterpriseApi.PhaseInstall
						installWorker.appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgInstallComplete
					}
					//For now, set the deploy status as complete. Eventually, we can phase it out
					installWorker.appDeployInfo.DeployStatus = enterpriseApi.DeployStatusComplete
					ppln.deleteWorkerFromPipelinePhase(phaseInfo.Phase, installWorker)
				} else if phaseInfo.RetryCount >= pipelinePhaseMaxRetryCount {
					installWorker.appDeployInfo.PhaseInfo.Status = enterpriseApi.AppPkgInstallError
					ppln.deleteWorkerFromPipelinePhase(phaseInfo.Phase, installWorker)
				}
			}

		}
		time.Sleep(200 * time.Millisecond)
	}
	scopedLog.Info("Wating for the download workers to finish")
	// wait for all the install workers to finish
	pplnPhase.workerWaiter.Wait()

	// Signal that the Install phase manager is complete
	scopedLog.Info("All the download workers finished")
	phaseWaiter.Done()
}

// isPipelineEmpty checks if the pipeline is empty or not
func (ppln *AppInstallPipeline) isPipelineEmpty() bool {
	if ppln.pplnPhases == nil {
		return false
	}

	for _, phase := range ppln.pplnPhases {
		if len(phase.q) > 0 {
			return false
		}
	}
	return true
}

// isAppInstallationCompleteOnStandaloneReplicas confirms if an app package is installed on all the Standalone Pods or not
func isAppInstallationCompleteOnStandaloneReplicas(auxPhaseInfo []enterpriseApi.PhaseInfo) bool {
	for _, phaseInfo := range auxPhaseInfo {
		if phaseInfo.Phase != enterpriseApi.PhaseInstall || phaseInfo.Status != enterpriseApi.AppPkgInstallComplete {
			return false
		}
	}

	return true
}

// checkIfBundlePushNeeded confirms if the bundle push is needed or not
func checkIfBundlePushNeeded(clusterScopedApps []*enterpriseApi.AppDeploymentInfo) bool {
	for _, appDeployInfo := range clusterScopedApps {
		if appDeployInfo.PhaseInfo.Phase != enterpriseApi.PhasePodCopy || appDeployInfo.PhaseInfo.Status != enterpriseApi.AppPkgPodCopyComplete {
			return false
		}
	}

	return true
}

// initAppInstallPipeline creates the AFW scheduler pipeline
// TBD: Do we need to make it singleton? For now leave it till we have the clarity on
func initAppInstallPipeline() *AppInstallPipeline {
	if afwPipeline != nil {
		return afwPipeline
	}

	afwPipeline = &AppInstallPipeline{}
	afwPipeline.pplnPhases = make(map[enterpriseApi.AppPhaseType]*PipelinePhase, 3)
	afwPipeline.sigTerm = make(chan bool)
	// Allocate the Download phase
	afwPipeline.pplnPhases[enterpriseApi.PhaseDownload] = &PipelinePhase{
		q:          []*PipelineWorker{},
		msgChannel: make(chan *PipelineWorker),
	}

	// Allocate the Pod Copy phase
	afwPipeline.pplnPhases[enterpriseApi.PhasePodCopy] = &PipelinePhase{
		q:          []*PipelineWorker{},
		msgChannel: make(chan *PipelineWorker),
	}

	// Allocate the install phase
	afwPipeline.pplnPhases[enterpriseApi.PhaseInstall] = &PipelinePhase{
		q:          []*PipelineWorker{},
		msgChannel: make(chan *PipelineWorker),
	}

	return afwPipeline
}

func afwGetReleventStatefulsetByKind(cr splcommon.MetaObject, client splcommon.ControllerClient) *appsv1.StatefulSet {
	scopedLog := log.WithName("getReleventStatefulsetByKind").WithValues("name", cr.GetName(), "namespace", cr.GetNamespace())
	var instanceID InstanceType

	switch cr.GetObjectKind().GroupVersionKind().Kind {
	case "Standalone":
		instanceID = SplunkStandalone
	case "LicenseMaster":
		instanceID = SplunkLicenseMaster
	case "SearchHeadCluster":
		instanceID = SplunkDeployer
	case "ClusterMaster":
		instanceID = SplunkClusterMaster
	case "MonitoringConsole":
		instanceID = SplunkMonitoringConsole
	default:
		return nil
	}

	statefulsetName := GetSplunkStatefulsetName(instanceID, cr.GetName())
	namespacedName := types.NamespacedName{Namespace: cr.GetNamespace(), Name: statefulsetName}
	sts, err := splctrl.GetStatefulSetByName(client, namespacedName)
	if err != nil {
		scopedLog.Error(err, "Unable to get the stateful set")
	}

	return sts
}

// afwSchedulerEntry Starts the scheduler Pipeline with the required phases
func afwSchedulerEntry(client splcommon.ControllerClient, cr splcommon.MetaObject, appDeployContext *enterpriseApi.AppDeploymentContext,
	appFrameworkConfig *enterpriseApi.AppFrameworkSpec) error {
	scopedLog := log.WithName("afwSchedulerEntry").WithValues("name", cr.GetName(), "namespace", cr.GetNamespace(), "App name")
	afwPipeline = initAppInstallPipeline()
	var yieldTime int64 = 90 //seconds

	var clusterScopedApps []*enterpriseApi.AppDeploymentInfo

	// get the current available disk space for downloading apps on operator pod
	availableDiskSpace, err := getAvailableDiskSpace()
	if err != nil {
		return err
	}
	afwPipeline.availableDiskSpace = availableDiskSpace

	// Start the download phase manager
	afwPipeline.phaseWaiter.Add(1)
	go afwPipeline.downloadPhaseManager(&afwPipeline.phaseWaiter, afwPipeline.sigTerm, appDeployContext)

	// Start the pod copy phase manager
	afwPipeline.phaseWaiter.Add(1)
	go afwPipeline.podCopyPhaseManager(&afwPipeline.phaseWaiter, afwPipeline.sigTerm, appDeployContext)

	// Start the install phase manager
	afwPipeline.phaseWaiter.Add(1)
	go afwPipeline.installPhaseManager(&afwPipeline.phaseWaiter, afwPipeline.sigTerm, appDeployContext)

	scopedLog.Info("Creating pipeline workers for pending app packages")
	for appSrcName, appSrcDeployInfo := range appDeployContext.AppsSrcDeployStatus {
		for index := 0; index < len(appSrcDeployInfo.AppDeploymentInfoList); index++ {
			deployInfo := appSrcDeployInfo.AppDeploymentInfoList[index]
			var pplnPhase enterpriseApi.AppPhaseType
			var podName string
			var podID int

			appSrc, err := getAppSrcSpec(appFrameworkConfig.AppSources, appSrcName)
			if err != nil {
				// Error, should never happen
				continue
			}

			// Track the cluster scoped apps to track the bundle push
			if appSrc.Scope == enterpriseApi.ScopeCluster && deployInfo.DeployStatus != enterpriseApi.DeployStatusComplete {
				clusterScopedApps = append(clusterScopedApps, &deployInfo)
			}

			pplnPhase = ""
			//Push All the Intermediatory work to the Pipeline phases and let the corresponding phase manager take care of them
			if deployInfo.PhaseInfo.Phase == enterpriseApi.PhaseDownload && deployInfo.PhaseInfo.RetryCount < pipelinePhaseMaxRetryCount {
				pplnPhase = enterpriseApi.PhaseDownload
			} else if deployInfo.PhaseInfo.Phase == enterpriseApi.PhasePodCopy && deployInfo.PhaseInfo.RetryCount < pipelinePhaseMaxRetryCount {
				pplnPhase = enterpriseApi.PhasePodCopy
			} else if deployInfo.PhaseInfo.Phase == enterpriseApi.PhaseInstall && deployInfo.PhaseInfo.Status != enterpriseApi.AppPkgInstallComplete && deployInfo.PhaseInfo.RetryCount < pipelinePhaseMaxRetryCount {
				// Cluster scopes do not get to this phase. Only local scoped apps fall into this pipeline.
				// For cluster scoped apps, all that we need is bundle push. So, a one-shot worker(outside of this logic) serves the purpose
				pplnPhase = enterpriseApi.PhaseInstall
			}

			// Ignore any other apps that are not in progress
			if pplnPhase != "" {
				podName = getApplicablePodNameForWorker(cr, podID)
				sts := afwGetReleventStatefulsetByKind(cr, client)
				afwPipeline.createAndAddPipelineWorker(pplnPhase, &deployInfo, appSrcName, podName, appFrameworkConfig, appDeployContext, client, cr, sts)
			}
		}
	}

	var clusterAppsList string
	for _, appDeployInfo := range clusterScopedApps {
		clusterAppsList = fmt.Sprintf("%s:%s, %s", appDeployInfo.AppName, appDeployInfo.ObjectHash, clusterAppsList)
	}
	scopedLog.Info("List of cluster scoped apps(appName:digest) for this reconcile entry", "apps", clusterAppsList)

	// Wait for the yield function to finish
	afwPipeline.phaseWaiter.Add(1)
	go func() {
		afwEntryTime := time.Now().Unix()
		for {
			if afwEntryTime+yieldTime < time.Now().Unix() || afwPipeline.isPipelineEmpty() {
				scopedLog.Info("Yielding from AFW scheduler", "time elapsed", time.Now().Unix()-afwEntryTime)
				// terminate download Phase
				afwPipeline.sigTerm <- true

				// terminate podCopy Phase
				afwPipeline.sigTerm <- true

				// terminate install Phase
				afwPipeline.sigTerm <- true

				close(afwPipeline.sigTerm)
				afwPipeline.phaseWaiter.Done()
				break
			} else {
				if len(clusterScopedApps) > 0 && appDeployContext.BundlePushStatus.BudlePushStage < enterpriseApi.BundlePushPending {
					if checkIfBundlePushNeeded(clusterScopedApps) {
						// Trigger the bundle push playbook: CSPL-1332, CSPL-1333
					}
				}
				// sleep for one second
				time.Sleep(1 * time.Second)
			}
		}
	}()

	// ToDo: sgontla: for now, just make the UT happy, until we get the glue logic
	// check if this needs to be pure singleton for the entire reconcile span, considering CSPL-1169. CC: @Gaurav
	// Finally delete the pipeline
	defer func() {
		afwPipeline = nil
	}()
	scopedLog.Info("Waiting for the phase managers to finish")

	// Wait for all the pipeline managers to finish
	afwPipeline.phaseWaiter.Wait()
	scopedLog.Info("All the phase managers finished")

	return nil
}
