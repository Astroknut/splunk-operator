# Premium App Installation Guide

The Splunk Operator currently provides support for automating installation of Enterprise Security with support for other premium apps coming in the future. This page documents the prerequisites, installation steps, and limitations of deploying premium apps using the Splunk Operator. 


## Enterprise Security


### Prerequisites

Installing Enterprise Security in a Kubernetes cluster with the Splunk Operator requires the following:

* Ability to utilize the Splunk Operator [app framework](https://splunk.github.io/splunk-operator/AppFramework.html) method of installation.
* Access to the [Splunk Enterprise Security](https://splunkbase.splunk.com/app/263/) app package.
* Splunk Enterprise Security version 6.4.1 or 6.6.0 as Splunk Operator requires Splunk Enterprise 8.2.2 or later. For more information regarding Splunk Enterprise and Enterprise Security compatibility, see the [version compatibility matrix](https://docs.splunk.com/Documentation/VersionCompatibility/current/Matrix/CompatMatrix).
* If installing to an Indexer Cluster, access to the corresponding Splunk_TA_ForIndexers app from the Enterprise Security package. This app can be extracted using the procedure available at [Splunk ES app installation on Indexer Cluster](https://docs.splunk.com/Documentation/ES/7.0.2/Install/InstallTechnologyAdd-ons). After the Splunk_TA_ForIndexers is available, place it in a S3 bucket and use "cluster" scope in the YAML file to push the package via the cluster manager to the indexers. A Splunk_TA_ForIndexers package is also shipped with in the SplunkEnterpriseSecutitySuite package (can be found in the ES app package at SplunkEnterpriseSecuritySuite/install/splunkcloud/splunk_app_es/Splunk_TA_ForIndexers-\<version\>.spl).  
* Pod resource specs that meet the [Enterprise Security hardware requirements](https://docs.splunk.com/Documentation/ES/latest/Install/DeploymentPlanning#Hardware_requirements).
* In the following sections, aws s3 remote bucket is used for placing the splunk apps, but as given in the [app framework doc](https://splunk.github.io/splunk-operator/AppFramework.html), you can use Azure blob remote buckets also.

### Supported Deployment Types

Currently there are only a subset of architectures that support automated deployment of Enterprise Security through the Splunk Operator.

Supported Architectures Include:
* Standalone Splunk Instances 
* Standalone Search Head(s) which search any number of Indexer Clusters.
* Search Head Cluster(s) which search any number of Indexer Clusters. 

Notably, if deploying a distributed search environment, the use of indexer clustering is required to ensure that the necessary Enterprise Security specific configuration is pushed to the indexers via the Cluster Manager.

### What is and what is not automated by the Splunk Operator

The Splunk Operator will install the necessary Enterprise Security components depending on the architecture specified by the applied CRDs.

#### Standalone / Standalone Search Heads
For standalones and standalone search heads the Operator will install Splunk Enterprise Security and all associated domain add-ons (DAs), and supporting add-ons (SAs).

#### Search Head Cluster
When installing Enterprise Security in a Search Head Cluster, the Operator will perform the following tasks: 
1) Install the splunk enterprise app in Deployer's etc/apps directory
2) Run the ES post install command `essinstall` that stages the Splunk Enterprise Security and all associated domain add-ons (DAs), and supporting add-ons (SAs) to the etc/shcapps.
3) Push the shc cluster bundle from deployer to all the SHs.

#### Indexer Cluster
When installing ES in an indexer clustering environment through the Splunk Operator it is necessary to deploy the supplemental [Splunk_TA_ForIndexers](https://docs.splunk.com/Documentation/ES/latest/Install/InstallTechnologyAdd-ons#Create_the_Splunk_TA_ForIndexers_and_manage_deployment_manually) app from the ES package to the indexer cluster members. This can be achieved using the AppFramework app deployent steps using appSources scope of "cluster".


### How to Install Enterprise Security using the Splunk Operator


#### Necessary Configuration

When crafting your Custom Resource to create a Splunk Enterprise Deployment it is necessary to take the following configurations into account.

##### [appSources](https://splunk.github.io/splunk-operator/AppFramework.html#appsources) scope
   
   - When deploying ES to a Standalone or to a Search Head Cluster, it must be configured with an appSources scope of "premiumApps".
   - When deploying the Splunk_TA_ForIndexers app to an Indexer Cluster, it must be configured with an appSources scope of "cluster".

#####  SSL Enablement

When installing ES versions 6.3.0+ is necessary to supply a value for the parameter ssl_enablement that is required by the ES post installation command `essinstall`. By default the value of strict is used which requires Splunk to have SSL enabled in web.conf (refer to setting `enableSplunkWebSSL`). The below table can be used for reference of available values of ssl enablement. 

| SSL mode	| Description | 
| --------- | ----------- | 
|strict     |	Default mode. Ensure that SSL is enabled in the web.conf configuration file to use this mode. Otherwise, the installer exists with an error. Note that for the SHC, the ES post install command `essinstall` is run in deployer, and this command looks into web.conf files under etc/shcapps to validate that enableSplunkWebSSL is set to true or not.| 
| auto	   | Enables SSL in the etc/system/local/web.conf configuration file. This mode is not supported by SHC |
| ignore	   | Ignores whether SSL is enabled or disabled. |

The Operator passes the ssl_enablement parameter through `sslEnablement` parameter withn `premiumAppsProps` configuration. The following snippet of a YAML file shows using `ignore` mode for ssl_enablement

```yaml
  appSources:
        - name: esApp
          location: es_app/
          scope: premiumApps             <-------- setting scope as premiumApps to install ES
          premiumAppsProps:
            type: enterpriseSecurity
            esDefaults:
              sslEnablement: ignore       <--------- setting ssl_enablement to ignore
```

##### Search Head Cluster server.conf timeouts

It may be necessary to increase the value of the default Search Head Clustering network timeouts to ensure that the connections made from the deployer to the Search Heads while pushing apps do not timeout. 

These timeouts can be set through defaults.yaml
```yaml
  defaults: |-
    splunk:
      conf:
        - key: server
          value:
            directory: /opt/splunk/etc/system/local
            content:
              shclustering:         
                rcv_timeout: 300
                send_timeeout: 300
                cxn_timeeout: 300
```

##### splunkdConnectionTimeout
Increasing the value of splunkdConnectionTimeout in web.conf will help ensure that all API calls made by the installer script will not timeout and prevent installation from succeeding.
```yaml
  defaults: |-
    splunk:
      conf:
        - key: web
          value:
            directory: /opt/splunk/etc/system/local
            content:
              settings:
                splunkdConnectionTimeout: 300
```

### Example YAML

#### Install ES on a standalone splunk deployment

Here is an example of a standalone CR with the spec(scope=premiumApps, type=EnterpriseSecurity, sslEnablement=ignore)

Assuming the ES app tarball exists in an s3 bucket folder named "es_app" under the parent folder "security-team-apps" in your s3 bucket

```yaml
apiVersion: enterprise.splunk.com/v4
kind: Standalone
metadata:
  name: example
  finalizers:
  - enterprise.splunk.com/delete-pvc
spec:
  replicas: 1
  appRepo:
    appsRepoPollIntervalSeconds: 60
    defaults:
      volumeName: volume_app_repo
      scope: local
    appSources:
      - name: esApp
        location: es_app/
        scope: premiumApps
        premiumAppsProps:
          type: enterpriseSecurity
          esDefaults:
             sslEnablement: ignore
    volumes:
      - name: volume_app_repo
        storageType: s3
        provider: aws
        path: security-team-apps/
        endpoint: https://s3-us-west-2.amazonaws.com
        region: us-west-2
        secretRef: splunk-s3-secret
```

#### Install ES on a Search Head Cluster splunk deployment

The below yaml will configure ES on a Search Head Cluster which searches an Indexer Cluster. 

 Assumptions made are that:
 1. The ES app tarball exists in an s3 bucket folder named "es_app"
 2. Optional: The Splunk_TA_ForIndexers app exists in an s3 bucket folder named "es_app_indexer_ta".  
 
 If you choose to extract Splunk_TA_ForIndexers instead of using the pre-shipped Splunk_TA_ForIndexers package (SplunkEnterpriseSecuritySuite/install/splunkcloud/splunk_app_es/Splunk_TA_ForIndexers-\<version\>.spl), the SHC should be up and running with Splunk Enterprise Secutiry Suite first. Then, the steps given at https://docs.splunk.com/Documentation/ES/7.0.2/Install/InstallTechnologyAdd-ons can be used to extract and deploy the Splunk_TA_ForIndexers to indexers.

Steps:
1. Apply the following YAML file
2. Wait for the SHC, CM and Indexers and up and running
3. Login to a SH, and extract the Splunk_TA_ForIndexers (steps given here)[https://docs.splunk.com/Documentation/ES/7.0.2/Install/InstallTechnologyAdd-ons]
4. Place the extracted Splunk_TA_ForIndexers package in the s3 bucket folder named "es_app_indexer_ta"
The operator will poll this bucket after configured appsRepoPollIntervalSeconds and install the Splunk_TA_ForIndexers also.
 
In this example scope=premiumApps, type=EnterpriseSecurity, sslEnablement=ignore
 
```yaml
apiVersion: enterprise.splunk.com/v4
kind: SearchHeadCluster
metadata:
  name: shc-es
  finalizers:
  - enterprise.splunk.com/delete-pvc
spec:
  appRepo:
    appsRepoPollIntervalSeconds: 60
    defaults:
      volumeName: volume_app_repo
      scope: local
    appSources:
      - name: esApp
        location: es_app/
        scope: premiumApps
        premiumAppsProps:
          type: enterpriseSecurity
          esDefaults:
             sslEnablement: ignore
    volumes:
      - name: volume_app_repo
        storageType: s3
        provider: aws
        path: security-team-apps/
        endpoint: https://s3-us-west-2.amazonaws.com
        region: us-west-2
        secretRef: splunk-s3-secret
  clusterMasterRef:
    name: cm-es
---
apiVersion: enterprise.splunk.com/v4
kind: ClusterMaster
metadata:
  name: cm-es
  finalizers:
  - enterprise.splunk.com/delete-pvc
spec:
  appRepo:
    appsRepoPollIntervalSeconds: 60
    defaults:
      volumeName: volume_app_repo
      scope: local
    appSources:
      - name: esAppIndexer
        location: es_app_indexer_ta/
        scope: cluster
    volumes:
      - name: volume_app_repo
        storageType: s3
        provider: aws
        path: security-team-apps/
        endpoint: https://s3-us-west-2.amazonaws.com
        region: us-west-2
        secretRef: splunk-s3-secret
---
apiVersion: enterprise.splunk.com/v2
kind: IndexerCluster
metadata:
  name: idc-es
  finalizers:
  - enterprise.splunk.com/delete-pvc
spec:
  clusterMasterRef:
    name: cm-es
  replicas: 3
```

#### Special consideration while using ssl enabled mode of strict

In this example scope=premiumApps, type=EnterpriseSecurity, sslEnablement=strict

For using the strict mode, a SHC bootstrap step is required so the SHC has Splunk Web SSL enabled.
Steps:
1. Create a shc app that contain an app with its local/web.conf setting enableSplunkWebSSL=true
2. Deploy this app through app framework cluster scope. 
3. Use the extraEnv:SPLUNK_HTTP_ENABLESSL with value true

Following is an example to enable Splunk Web SSL through operator on SHC:

```yaml
apiVersion: enterprise.splunk.com/v4
kind: SearchHeadCluster
metadata:
  name: shcssl
  finalizers:
  - enterprise.splunk.com/delete-pvc
spec:
  extraEnv:
    - name: SPLUNK_HTTP_ENABLESSL
      value : "true"
  appRepo:
    appsRepoPollIntervalSeconds: 60
    defaults:
      volumeName: volume_app_repo
      scope: local
    appSources:
      - name: coreapps
        scope: cluster
        location: coreapps/
    volumes:
      - name: volume_app_repo
        storageType: s3
        provider: aws
        path: security-team-apps/
        endpoint: https://s3-us-west-2.amazonaws.com
        region: us-west-2
        secretRef: splunk-s3-secret
```


#### Installation steps

1. Ensure that the Enterprise Security app tarball is present in the specified AppFramework s3 location with the correct appSources scope. Additionally, if configuring an indexer cluster, ensure that the Splunk_TA_ForIndexers app is present in the ClusterManager AppFramework s3 location with the appSources "cluster" scope.
   
2. Apply the specified custom resource(s), the Splunk Operator will handle installation and the environment will be ready to use once all pods are in the "Ready" state.
   
**Important Considerations**
* Installation may take upwards of 30 minutes.

#### Post Installation Configuration

After installing Enterprise Security :

* [Deploy add-ons to Splunk Enterprise Security](https://docs.splunk.com/Documentation/ES/latest/Install/InstallTechnologyAdd-ons) - Technology add-ons (TAs) which need to be installed to indexers can be installed via AppFramework, while TAs that reside on forwarders will need to be installed manually or via third party configuration management.

* [Setup Integration with Splunk Stream](https://docs.splunk.com/Documentation/ES/latest/Install/IntegrateSplunkStream) (optional)

* [Configure and deploy indexes](https://docs.splunk.com/Documentation/ES/latest/Install/Indexes) - The indexes associated with the packaged DAs and SAs will automatically be pushed to indexers when using indexer clustering, this step is only necessary if it is desired to configure any [custom index configuration](https://docs.splunk.com/Documentation/ES/latest/Install/Indexes#Index_configuration). Additionally, any newly installed technical ad-ons which are not included with the ES package may require index deployment.

* [Configure Users and Roles as desired](https://docs.splunk.com/Documentation/ES/latest/Install/ConfigureUsersRoles)

* [Configure Datamodels](https://docs.splunk.com/Documentation/ES/latest/Install/Datamodels)


### Upgrade Steps

To upgrade ES, all that is required is to move the new ES package into the specified AppFramework bucket. This will initiate a pod reset and begin the process of upgrading the new version. In indexer clustering environments, it is also necessary to move the new Splunk_TA_ForIndexers app to the Cluster Manager's AppFramework bucket that deploys apps to cluster members.

* The upgrade process will preserve any knowledge objects that exist in app local directories.

* Be sure to check the [ES upgrade notes](https://docs.splunk.com/Documentation/ES/latest/Install/Upgradetonewerversion#Version-specific_upgrade_notes) for any version specific changes.

### Troubleshooting

Following logs can be useful to check the ES app installation progress:

Splunk operator log:

```
kubectl logs <operator_pod_name>
```

Logs of the respective pods:

```
kubectl logs <pod_name>
```


Common issues that may be encountered are : 
* ES installation failed as you used default sslEnablement mode ("strict") - enable Splunk Web SSL in web.conf.
* Ansible task timeouts - raise associated timeout (splunkdConnectionTimeout, rcvTimeout, etc.)
* Pod Recycles - raise livenessProbe value


### Current Limitations

* For indexer clustering environments, need to manually extract Splunk_TA_ForIndexers app and place in Cluster Manager AppFramework bucket to be deployed to indexers.

* Need to deploy add-ons to forwarders manually (or through your own methods).

* Need to deploy Stream App Manually
