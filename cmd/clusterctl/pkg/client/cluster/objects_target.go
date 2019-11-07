/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	retryAcquireClient = 10 * time.Second
	//retryIntervalKubectlApply  = 10 * time.Second
	retryIntervalResourceReady = 10 * time.Second

	timeoutAcquireClient = 10 * time.Minute
	//timeoutKubectlApply        = 15 * time.Minute
	timeoutMachineReady = 30 * time.Minute
)

const (
	timeoutMachineReadyVariableName = "CLUSTER_API_MACHINE_READY_TIMEOUT"
)

var _ sourceClient = &objectsClient{}

type targetClient interface {
	EnsureNamespace(string) error

	CreateCluster(*clusterv1.Cluster) error
	CreateUnstructuredObject(*unstructured.Unstructured) error
	CreateSecret(*corev1.Secret) error
	CreateMachineDeployments([]*clusterv1.MachineDeployment, string) error
	CreateMachineSets([]*clusterv1.MachineSet, string) error
	CreateMachines([]*clusterv1.Machine, string) error

	GetCluster(string, string) (*clusterv1.Cluster, error)

	SetClusterOwnerRef(runtime.Object, *clusterv1.Cluster) error

	WaitForMachineReady(cs client.Client, machine *clusterv1.Machine) error
	// Apply(string) error
	// GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error)
	// GetMachineSet(string, string) (*clusterv1.MachineSet, error)
}

var _ targetClient = &objectsClient{}

func (c *objectsClient) EnsureNamespace(namespace string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	ns := &corev1.Namespace{}
	key := client.ObjectKey{
		Name: namespace,
	}

	if err = cs.Get(ctx, key, ns); err == nil {
		return nil
	}
	if apierrors.IsForbidden(err) {
		namespaces := &corev1.NamespaceList{}
		if err := cs.List(ctx, namespaces); err != nil {
			return err
		}

		for _, ns := range namespaces.Items {
			if ns.Name == namespace {
				return nil
			}
		}
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	ns = &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if err = cs.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (c *objectsClient) CreateCluster(cluster *clusterv1.Cluster) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	//TODO: check original clusterclient.GetContextNamespace() logic

	if err := cs.Create(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error creating cluster in namespace %v", cluster.Namespace)
	}

	// It seems that Create cleanups the value into the Kind field; this is not a problem usually,
	// but in this case it is necessary to restoring it because it is used in subsequent steps of the move sequence
	// when checking object ownership e.g. GetMachineDeploymentsForCluster
	if cluster.Kind == "" {
		cluster.Kind = "Cluster"
	}

	return nil
}

func (c *objectsClient) CreateUnstructuredObject(u *unstructured.Unstructured) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	if err := cs.Create(ctx, u); err != nil {
		return errors.Wrapf(err, "error creating unstructured object %q %s/%s",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	return nil
}

func (c *objectsClient) CreateSecret(s *corev1.Secret) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	if err := cs.Create(ctx, s); err != nil {
		return errors.Wrapf(err, "error creating Secret %s/%s", s.Namespace, s.Name)
	}
	return nil
}

func (c *objectsClient) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, namespace string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	for _, deploy := range deployments {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err := cs.Create(ctx, deploy); err != nil {
			return errors.Wrapf(err, "error creating a machine deployment object in namespace %q", namespace)
		}
	}
	return nil
}

func (c *objectsClient) CreateMachineSets(machineSets []*clusterv1.MachineSet, namespace string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	for _, ms := range machineSets {
		// TODO: Run in parallel https://github.com/kubernetes-sigs/cluster-api/issues/258
		if err := cs.Create(ctx, ms); err != nil {
			return errors.Wrapf(err, "error creating a machine set object in namespace %q", namespace)
		}
	}
	return nil
}

func (c *objectsClient) CreateMachines(machines []*clusterv1.Machine, namespace string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		gerr    error
	)
	// The approach to concurrency here comes from golang.org/x/sync/errgroup.
	for _, machine := range machines {
		wg.Add(1)

		go func(machine *clusterv1.Machine) {
			defer wg.Done()

			if err := cs.Create(ctx, machine); err != nil {
				errOnce.Do(func() {
					gerr = errors.Wrapf(err, "error creating a machine object in namespace %v", namespace)
				})
				return
			}

			if err := c.WaitForMachineReady(cs, machine); err != nil {
				errOnce.Do(func() { gerr = err })
			}
		}(machine)
	}
	wg.Wait()
	return gerr
}

func (c *objectsClient) WaitForMachineReady(cs client.Client, machine *clusterv1.Machine) error {
	timeout := timeoutMachineReady

	//TODO: use config
	if p := os.Getenv(timeoutMachineReadyVariableName); p != "" {
		t, err := strconv.Atoi(p)
		if err == nil {
			// only valid value will be used
			timeout = time.Duration(t) * time.Minute
			klog.V(4).Info("Setting wait for machine timeout value to ", timeout)
		}
	}

	err := c.k8sproxy.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		klog.V(2).Infof("Waiting for Machine %v to become ready...", machine.Name)
		if err := cs.Get(ctx, client.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}, machine); err != nil {
			return false, nil
		}

		// Return true if the Machine has a reference to a Node.
		return machine.Status.NodeRef != nil, nil
	})

	return err
}

func (c *objectsClient) GetCluster(name string, namespace string) (*clusterv1.Cluster, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	cluster := &clusterv1.Cluster{}

	if err := cs.Get(ctx, key, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

func (c *objectsClient) SetClusterOwnerRef(obj runtime.Object, cluster *clusterv1.Cluster) error {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	meta.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		},
	})

	return nil
}
