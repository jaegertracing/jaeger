#!/usr/bin/env bash

set -e

make docker

PUBLIC_HOST=$(hostname -s)
ES_OPERATOR_NAMESPACE=openshift-logging
TEST_NAMESPACE=e2e-token-test
MAX_WAIT_ATTEMPTS=100

function is_pod_ready() {
   containers_ready=$(oc get pod $1 | tail -n 1 | awk '{print $2}')
  [[ "${containers_ready}" == "2/2" ]]
}

function pods_ready() {
    pods=($(oc get pod -n ${TEST_NAMESPACE} -o 'jsonpath={.items[*].metadata.name}'))
    if [[ "${#pods[@]}" < "2" ]]; then
        return 1
    fi
    for pod in "${pods[@]}"; do
        is_pod_ready ${pod} || return 1
    done
    return 0
}

function wait_pods_ready() {
 local attempts=MAX_WAIT_ATTEMPTS
 for ((i=0; i<$attempts; i+=1)); do
    if pods_ready
    then
        return 0
    fi
    sleep 1
   done
   return 1
}

if [ ! -z "${TRAVIS}" ]
then
    OPENSHIFT_BIN_PATH=${HOME}/bin
    OPENSHIFT_EXECUTABLE=${OPENSHIFT_BIN_PATH}/oc
   # download and copy openshift oc command
    curl -SL https://github.com/openshift/origin/releases/download/v3.9.0/openshift-origin-client-tools-v3.9.0-191fece-linux-64bit.tar.gz | tar -xz -C ${OPENSHIFT_BIN_PATH} --strip=1
    YAML_RESOURCES_PATH=./scripts/travis/es-token-propagation.yaml
else
    OPENSHIFT_BIN_PATH=/usr/bin/
    OPENSHIFT_EXECUTABLE=${OPENSHIFT_BIN_PATH}/oc
    YAML_RESOURCES_PATH=./es-token-propagation.yaml
fi

# create cluster
${OPENSHIFT_EXECUTABLE} cluster up --public-hostname=${PUBLIC_HOST}

# login with privileges
${OPENSHIFT_EXECUTABLE} login -u system:admin

# install elasticsearch operator
${OPENSHIFT_EXECUTABLE} create namespace ${TEST_NAMESPACE}
${OPENSHIFT_EXECUTABLE} project ${TEST_NAMESPACE}
${OPENSHIFT_EXECUTABLE} create namespace ${ES_OPERATOR_NAMESPACE} 2>&1 | grep -v "already exists" || true
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/master/example/prometheus-operator-crd/prometheusrule.crd.yaml
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/master/example/prometheus-operator-crd/servicemonitor.crd.yaml
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/openshift/elasticsearch-operator/master/manifests/01-service-account.yaml -n ${ES_OPERATOR_NAMESPACE}
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/openshift/elasticsearch-operator/master/manifests/02-role.yaml
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/openshift/elasticsearch-operator/master/manifests/03-role-bindings.yaml
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/openshift/elasticsearch-operator/master/manifests/04-crd.yaml -n ${ES_OPERATOR_NAMESPACE}
${OPENSHIFT_EXECUTABLE} apply -f https://raw.githubusercontent.com/openshift/elasticsearch-operator/master/manifests/05-deployment.yaml -n ${ES_OPERATOR_NAMESPACE}
${OPENSHIFT_EXECUTABLE} label node localhost kubernetes.io/os=linux
sudo sysctl -w vm.max_map_count=262144

# deploy query component with token-propagation setup
${OPENSHIFT_EXECUTABLE} apply -f ${YAML_RESOURCES_PATH}

echo "Waiting for pods to be ready..."
# Wait for pods ready or timeout..
wait_pods_ready

# Give developer user permissions to see projects (see projects means read to ES)
${OPENSHIFT_EXECUTABLE} adm policy add-cluster-role-to-user admin developer

${OPENSHIFT_EXECUTABLE} get pods -n ${TEST_NAMESPACE}

make token-propagation-test