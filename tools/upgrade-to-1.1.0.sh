#!/usr/bin/env bash

#
#Copyright (c) 2018-2022 Splunk Inc. All rights reserved.
#
#Licensed under the Apache License, Version 2.0 (the "License");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an "AS IS" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.

# This is an upgrade script for splunk operator 
# Version 1.1.0 is a new installation rather than upgarde for current operator 
# Due to this user should cleanup the older script and install version 1.1.0
# The script will help customer in doing these steps
# This script ask current namespace where operator is installed , and 
# * it first takes backup of all the operator resources within the namespace
# * like serviceaccount, deployment, role, rolebinding, clusterrole, clusterrolebinding 
# * it then deletes all the resources and installs the opeartor in splunk-operator namespace
# * by default splunk-opeartor 1.1.0 will be installed to watch clusterwide, 

help() {
  echo ""
  echo "USAGE: ${PROGRAM_NAME} --help [ --current_namespace=<namespacename> ]"
  echo ""
  echo "OPTIONS:"
  echo ""
  echo "   --current_namespace specifiy the current namespace where operator is installed, " \
       " script will delete existing serviceaccount, deployment, role and rolebinding and install the operator in splunk-operator namespace"
  echo ""
  echo "   --help  Show this help message."
  echo ""
}

parse_options() {
  local count="$#"

  for i in $(seq "${count}"); do
    eval arg="\$$i"
    param="$(echo "${arg}" | awk -F '=' '{print $1}' | sed -e 's|--||')"
    val="$(echo "${arg}" | awk -F '=' '{print $2}')"

    case "${param}" in
      current_namespace)
        eval "${param}"="${val}"
        ;;
      help)
        help && exit 0
        ;;
      *)
        echo "Parameter not found: '$param'"
        help && exit 1
        ;;
    esac
  done
}


backup() {
    echo "taking backup of existing operator installation manifest files"
    echo "---" > backup.yaml
    echo "kubectl get namespace ${current_namespace} -o yaml >> backup.yaml"
    echo "" > backup.yaml
    kubectl get namespace ${current_namespace} -o yaml >> backup.yaml
    echo "---" >> backup.yaml
    echo "kubectl get serviceaccount splunk-operator -n ${current_namespace} -o yaml >> backup.yaml"
    kubectl get serviceaccount ${current_namespace} -n splunk-operator -o yaml >> backup.yaml
    echo "" > backup.yaml
    echo "---" >> backup.yaml
    echo "kubectl get deployment splunk-operator -n ${current_namespace} -o yaml >> backup.yaml"
    kubectl get deployment splunk-operator -n ${current_namespace} -o yaml >> backup.yaml
    echo "" > backup.yaml
    echo "---" >> backup.yaml
    echo "kubectl get role splunk:operator:namespace-manager -n ${current_namespace} -o yaml >> backup.yaml"
    kubectl get role splunk:operator:namespace-manager -n ${current_namespace} -o yaml >> backup.yaml
    echo "---" >> backup.yaml
    echo "kubectl get rolebinding splunk:operator:namespace-manager  -n ${current_namespace} -o yaml >> backup.yaml"
    echo "" > backup.yaml
    kubectl get rolebinding splunk:operator:namespace-manager -n ${current_namespace}  -o yaml >> backup.yaml
    echo "---" >> backup.yaml

    echo "kubectl get clusterrole splunk:operator:resource-manager -o yaml  >> backup.yaml"
    kubectl get clusterrole splunk:operator:resource-manager -o yaml  >> backup.yaml
    echo "" > backup.yaml
    echo "---" >> backup.yaml
    echo "kubectl get clusterrolebinding splunk:operator:resource-manager -o yaml >> backup.yaml"
    kubectl get clusterrolebinding splunk:operator:resource-manager -o yaml >> backup.yaml
    echo "" > backup.yaml
    echo "---" >> backup.yaml
}

delete_operator() {
    echo "deleting clusterrole"
    kubectl delete clusterrole splunk:operator:resource-manager -n ${current_namespace}
    echo "deleting cluster rolebinding"
    kubectl delete clusterrolebinding splunk:operator:resource-manager -n ${current_namespace}
    echo "deletign deployment"
    kubectl delete deployment splunk-operator -n ${current_namespace}
    echo "deleting serviceaccount"
    kubectl delete serviceaccount splunk-operator -n ${current_namespace}
    echo "deleting role"
    kubectl delete role splunk:operator:namespace-manager -n ${current_namespace}
    echo "deleting rolebinding"
    kubectl delete rolebinding splunk:operator:namespace-manager -n ${current_namespace}
}

deploy_operator() {
    echo "installing operator 1.1.0" 
    kubectl apply -f release-v1.1.0/splunk-operator-install.yaml
}

parse_options "$@"
backup
delete_operator
deploy_operator
