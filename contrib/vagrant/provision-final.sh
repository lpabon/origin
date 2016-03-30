#!/bin/bash

pull_images() {
    echo "Pull images"
    docker pull openshift/origin-docker-registry
    sleep 1
    docker pull openshift/origin-sti-builder
    sleep 1
    docker pull openshift/origin-deployer
    sleep 1
}

setup_kube() {
    echo "Setup kubeconfig"
    chmod a+rwX openshift.local.config/master/admin.kubeconfig 
    chmod +r openshift.local.config/master/openshift-registry.kubeconfig
    oadm registry --create \
        --credentials=openshift.local.config/master/openshift-registry.kubeconfig \
        --config=openshift.local.config/master/admin.kubeconfig
    oc describe service docker-registry --config=openshift.local.config/master/admin.kubeconfig
    oadm policy add-cluster-role-to-user cluster-admin test-admin \
        --config=openshift.local.config/master/admin.kubeconfig
}

setup_admin() {
    echo "Create user test-admin"
    oc login --certificate-authority=openshift.local.config/master/ca.crt \
        -u test-admin -p admin
}

setup_project() {
    echo "Create test project" 
    oc new-project test \
        --display-name="OpenShift 3 Sample" \
        --description="This is an example project to demonstrate OpenShift v3"
}

setup_router() {
    echo "Create router"
    docker pull openshift/origin-haproxy-router
    sleep 1
    echo '{"kind":"ServiceAccount","apiVersion":"v1","metadata":{"name":"router"}}' | oc create -f -

    # Wait until it is running
    local msg="waiting for router"
    local condition="oc get pods | grep router | grep Running"
    os::provision::wait-for-condition "${msg}" "${condition}"

    oadm policy add-scc-to-user hostnetwork -z router
    oadm policy add-cluster-role-to-user system:router system:serviceaccount:default:router
    chmod +r openshift.local.config/master/openshift-router.kubeconfig
    oadm router --credentials=openshift.local.config/master/openshift-router.kubeconfig \
                --service-account=router
}

setup_app() {
    echo "Setup sample application"
    oc new-app centos/ruby-22-centos7~https://github.com/openshift/ruby-hello-world.git
    oc expose svc/ruby-hello-world
}

final() {
    pull_images
    setup_kube
    setup_admin
    setup_project
    setup_route
}

final

