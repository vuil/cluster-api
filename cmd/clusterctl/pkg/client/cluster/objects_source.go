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
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv13 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeoutResourceReady = 15 * time.Minute
)

var deletePropagationPolicy = client.PropagationPolicy(metav1.DeletePropagationForeground)

type sourceClient interface {
	GetClusters(string) ([]*clusterv1.Cluster, error)
	ForceDeleteCluster(string, string) error

	GetUnstructuredObject(*unstructured.Unstructured) error
	ForceDeleteUnstructuredObject(*unstructured.Unstructured) error

	GetClusterSecrets(*clusterv1.Cluster) ([]*corev1.Secret, error)
	ForceDeleteSecret(string, string) error

	GetMachineDeployments(string) ([]*clusterv1.MachineDeployment, error)
	GetMachineDeploymentsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error)
	ForceDeleteMachineDeployment(string, string) error

	GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForCluster(*clusterv1.Cluster) ([]*clusterv1.MachineSet, error)
	GetMachineSetsForMachineDeployment(*clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error)
	ForceDeleteMachineSet(namespace, name string) error

	GetMachines(namespace string) ([]*clusterv1.Machine, error)
	GetMachinesForCluster(*clusterv1.Cluster) ([]*clusterv1.Machine, error)
	GetMachinesForMachineSet(*clusterv1.MachineSet) ([]*clusterv1.Machine, error)
	ForceDeleteMachine(string, string) error

	// Delete(string) error
	// ScaleDeployment(string, string, int32) error
	//
}

var _ sourceClient = &objectsClient{}

func (c *objectsClient) GetClusters(namespace string) ([]*clusterv1.Cluster, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	clusters := &clusterv1.ClusterList{}
	if err := cs.List(ctx, clusters); err != nil {
		return nil, errors.Wrapf(err, "error listing cluster objects in namespace %q", namespace)
	}

	var items []*clusterv1.Cluster
	for i := 0; i < len(clusters.Items); i++ {
		items = append(items, &clusters.Items[i])
	}
	return items, nil
}

func (c *objectsClient) ForceDeleteCluster(namespace, name string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	cluster := &clusterv1.Cluster{}
	if err := cs.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		return errors.Wrapf(err, "error getting cluster %s/%s", namespace, name)
	}

	cluster.ObjectMeta.SetFinalizers([]string{})
	if err := cs.Update(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error removing finalizer on cluster %s/%s", namespace, name)
	}

	if err := cs.Delete(ctx, cluster); err != nil {
		return errors.Wrapf(err, "error deleting cluster %s/%s", namespace, name)
	}

	return nil
}

func (c *objectsClient) GetUnstructuredObject(u *unstructured.Unstructured) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	key := client.ObjectKey{Namespace: u.GetNamespace(), Name: u.GetName()}
	if err := cs.Get(ctx, key, u); err != nil {
		return errors.Wrapf(err, "error fetching unstructured object %q %v", u.GroupVersionKind(), key)
	}
	return nil
}

func (c *objectsClient) ForceDeleteUnstructuredObject(u *unstructured.Unstructured) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	if err := cs.Get(ctx, client.ObjectKey{Namespace: u.GetNamespace(), Name: u.GetName()}, u); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "error retrieving unstructured object %q %s/%s",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	u.SetFinalizers([]string{})
	if err := cs.Update(ctx, u); err != nil {
		return errors.Wrapf(err, "error removing finalizer for unstructured object %q %s/%s",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	if err := cs.Delete(ctx, u); err != nil {
		return errors.Wrapf(err, "error deleting unstructured object %q %s/%s",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	return nil
}

func (c *objectsClient) GetClusterSecrets(cluster *clusterv1.Cluster) ([]*corev1.Secret, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	list := &corev1.SecretList{}
	if err := cs.List(ctx, list, client.InNamespace(cluster.Namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing Secrets for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	var res []*corev1.Secret
	for i, secret := range list.Items {
		if strings.HasPrefix(secret.Name, cluster.Name) {
			res = append(res, &list.Items[i])
		}
	}
	return res, nil
}

func (c *objectsClient) ForceDeleteSecret(namespace, name string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	secret := &corev1.Secret{}
	if err := cs.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "error getting Secret %s/%s", namespace, name)
	}
	if err := cs.Delete(ctx, secret); err != nil {
		return errors.Wrapf(err, "error deleting Secret %s/%s", secret.Namespace, secret.Name)
	}
	return nil
}

func (c *objectsClient) GetMachineDeployments(namespace string) ([]*clusterv1.MachineDeployment, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	if err := cs.List(ctx, machineDeploymentList, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing machine deployment objects in namespace %q", namespace)
	}
	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {
		machineDeployments = append(machineDeployments, &machineDeploymentList.Items[i])
	}
	return machineDeployments, nil
}

func (c *objectsClient) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	selectors := []client.ListOption{
		client.MatchingLabels{
			clusterv13.ClusterLabelName: cluster.Name,
		},
		client.InNamespace(cluster.Namespace),
	}
	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	if err := cs.List(ctx, machineDeploymentList, selectors...); err != nil {
		return nil, errors.Wrapf(err, "error listing MachineDeployments for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	var machineDeployments []*clusterv1.MachineDeployment
	for i := 0; i < len(machineDeploymentList.Items); i++ {

		md := machineDeploymentList.Items[i]
		for _, or := range md.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineDeployments = append(machineDeployments, &md)
			}
		}
	}
	return machineDeployments, nil
}

func (c *objectsClient) ForceDeleteMachineDeployment(namespace, name string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	md := &clusterv1.MachineDeployment{}
	if err := cs.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, md); err != nil {
		return errors.Wrapf(err, "error getting MachineDeployment %s/%s", namespace, name)
	}
	md.SetFinalizers([]string{})
	if err := cs.Update(ctx, md); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineDeployment %s/%s", namespace, name)
	}
	if err := cs.Delete(ctx, md, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting MachineDeployment %s/%s", namespace, name)
	}
	return nil
}

func (c *objectsClient) GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	machineSetList := &clusterv1.MachineSetList{}
	if err := cs.List(ctx, machineSetList, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets in namespace %q", namespace)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		machineSets = append(machineSets, &machineSetList.Items[i])
	}
	return machineSets, nil
}

func (c *objectsClient) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	selectors := []client.ListOption{
		client.MatchingLabels{
			clusterv13.ClusterLabelName: cluster.Name,
		},
		client.InNamespace(cluster.Namespace),
	}
	machineSetList := &clusterv1.MachineSetList{}
	if err := cs.List(ctx, machineSetList, selectors...); err != nil {
		return nil, errors.Wrapf(err, "error listing MachineSets for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machineSets []*clusterv1.MachineSet
	for i := 0; i < len(machineSetList.Items); i++ {
		ms := machineSetList.Items[i]
		for _, or := range ms.GetOwnerReferences() {
			if or.Kind == cluster.Kind && or.Name == cluster.Name {
				machineSets = append(machineSets, &ms)
				continue
			}
		}
	}
	return machineSets, nil
}

func (c *objectsClient) GetMachineSetsForMachineDeployment(md *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	machineSets, err := c.GetMachineSets(md.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachineSets []*clusterv1.MachineSet
	for _, ms := range machineSets {
		if metav1.GetControllerOf(ms) != nil && metav1.IsControlledBy(ms, md) {
			controlledMachineSets = append(controlledMachineSets, ms)
		}
	}
	return controlledMachineSets, nil
}

func (c *objectsClient) ForceDeleteMachineSet(namespace, name string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	ms := &clusterv1.MachineSet{}
	if err := cs.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ms); err != nil {
		return errors.Wrapf(err, "error getting MachineSet %s/%s", namespace, name)
	}
	ms.SetFinalizers([]string{})
	if err := cs.Update(ctx, ms); err != nil {
		return errors.Wrapf(err, "error removing finalizer for MachineSet %s/%s", namespace, name)
	}
	if err := cs.Delete(ctx, ms, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting MachineSet %s/%s", namespace, name)
	}
	return nil
}

func (c *objectsClient) GetMachines(namespace string) (machines []*clusterv1.Machine, _ error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	machineList := &clusterv1.MachineList{}
	if err := cs.List(ctx, machineList, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "error listing Machines in namespace %q", namespace)
	}
	for i := 0; i < len(machineList.Items); i++ {
		machines = append(machines, &machineList.Items[i])
	}
	return
}

func (c *objectsClient) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	selectors := []client.ListOption{
		client.MatchingLabels{
			clusterv13.ClusterLabelName: cluster.Name,
		},
		client.InNamespace(cluster.Namespace),
	}
	machineslist := &clusterv1.MachineList{}
	if err := cs.List(ctx, machineslist, selectors...); err != nil {
		return nil, errors.Wrapf(err, "error listing Machines for Cluster %s/%s", cluster.Namespace, cluster.Name)
	}
	var machines []*clusterv1.Machine
	for i := 0; i < len(machineslist.Items); i++ {
		machines = append(machines, &machineslist.Items[i])
	}
	return machines, nil
}

func (c *objectsClient) GetMachinesForMachineSet(ms *clusterv1.MachineSet) ([]*clusterv1.Machine, error) {
	machines, err := c.GetMachines(ms.Namespace)
	if err != nil {
		return nil, err
	}
	var controlledMachines []*clusterv1.Machine
	for _, m := range machines {
		if metav1.GetControllerOf(m) != nil && metav1.IsControlledBy(m, ms) {
			controlledMachines = append(controlledMachines, m)
		}
	}
	return controlledMachines, nil
}

func (c *objectsClient) ForceDeleteMachine(namespace, name string) error {
	cs, err := c.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	machine := &clusterv1.Machine{}
	if err := cs.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, machine); err != nil {
		return errors.Wrapf(err, "error getting Machine %s/%s", namespace, name)
	}
	machine.SetFinalizers([]string{})
	if err := cs.Update(ctx, machine); err != nil {
		return errors.Wrapf(err, "error removing finalizer for Machine %s/%s", namespace, name)
	}
	if err := cs.Delete(ctx, machine, deletePropagationPolicy); err != nil {
		return errors.Wrapf(err, "error deleting Machine %s/%s", namespace, name)
	}
	return nil
}
