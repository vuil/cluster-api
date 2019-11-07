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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
)

func moveClusters(from sourceClient, to targetClient, clusters []*clusterv1.Cluster) error {
	clusterNames := make([]string, 0, len(clusters))
	for _, c := range clusters {
		clusterNames = append(clusterNames, c.Name)
	}
	klog.V(3).Infof("Preparing to move Clusters: %v", clusterNames)

	for _, c := range clusters {
		if err := moveCluster(from, to, c); err != nil {
			return errors.Wrapf(err, "Failed to move cluster: %s/%s", c.Namespace, c.Name)
		}
	}
	return nil
}

func moveCluster(from sourceClient, to targetClient, cluster *clusterv1.Cluster) error {
	klog.V(3).Infof("Moving Cluster %s/%s", cluster.Namespace, cluster.Name)

	klog.V(3).Infof("Ensuring namespace %q exists on target cluster", cluster.Namespace)
	if err := to.EnsureNamespace(cluster.Namespace); err != nil {
		return errors.Wrapf(err, "unable to ensure namespace %q in target cluster", cluster.Namespace)
	}

	// New objects cannot have a specified resource version. Clear it out.
	cluster.SetResourceVersion("")
	if err := to.CreateCluster(cluster); err != nil {
		return errors.Wrapf(err, "error copying Cluster %s/%s to target cluster", cluster.Namespace, cluster.Name)
	}

	// Move infrastructure reference, if any.
	if cluster.Spec.InfrastructureRef != nil {
		if err := moveReference(from, to, cluster.Spec.InfrastructureRef, cluster.Namespace); err != nil {
			return errors.Wrapf(err, "error copying Cluster %s/%s infrastructure reference to target cluster",
				cluster.Namespace, cluster.Name)
		}
	}

	// Move the cluster's secrets only after the target cluster resource is created
	// since we have to update the Secret's OwnerRef
	if err := moveSecrets(from, to, cluster); err != nil {
		return errors.Wrapf(err, "failed to move Secrets for Cluster %s/%s to target cluster", cluster.Namespace, cluster.Name)
	}

	klog.V(3).Infof("Retrieving list of MachineDeployments to move for Cluster %s/%s", cluster.Namespace, cluster.Name)
	machineDeployments, err := from.GetMachineDeploymentsForCluster(cluster)
	if err != nil {
		return err
	}
	if err := moveMachineDeployments(from, to, machineDeployments); err != nil {
		return err
	}

	klog.V(3).Infof("Retrieving list of MachineSets not associated with a MachineDeployment to move for Cluster %s/%s", cluster.Namespace, cluster.Name)
	machineSets, err := from.GetMachineSetsForCluster(cluster)
	if err != nil {
		return err
	}
	if err := moveMachineSets(from, to, machineSets); err != nil {
		return err
	}

	klog.V(3).Infof("Retrieving list of Machines not associated with a MachineSet to move for Cluster %s/%s", cluster.Namespace, cluster.Name)
	machines, err := from.GetMachinesForCluster(cluster)
	if err != nil {
		return err
	}
	if err := moveMachines(from, to, machines); err != nil {
		return err
	}

	if err := from.ForceDeleteCluster(cluster.Namespace, cluster.Name); err != nil {
		return errors.Wrapf(err, "error force deleting cluster %s/%s", cluster.Namespace, cluster.Name)
	}

	klog.V(3).Infof("Successfully moved Cluster %s/%s", cluster.Namespace, cluster.Name)
	return nil
}

func moveReference(from sourceClient, to targetClient, ref *corev1.ObjectReference, namespace string) error {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(ref.APIVersion)
	u.SetKind(ref.Kind)
	u.SetNamespace(ref.Namespace)
	if u.GetNamespace() == "" {
		u.SetNamespace(namespace)
	}

	u.SetName(ref.Name)
	if err := from.GetUnstructuredObject(u); err != nil {
		return errors.Wrapf(err, "error fetching unstructured object %q %s/%s",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	return moveUnstructured(from, to, u)
}

func moveUnstructured(from sourceClient, to targetClient, u *unstructured.Unstructured) error {
	klog.V(3).Infof("Moving unstructured object %q %s/%s", u.GroupVersionKind(), u.GetNamespace(), u.GetName())

	targetObject := u.DeepCopy()

	// New objects cannot have a specified resource version. Clear it out.
	targetObject.SetResourceVersion("")

	// Remove owner reference.
	targetObject.SetOwnerReferences(nil)

	if err := to.CreateUnstructuredObject(targetObject); err != nil {
		return errors.Wrapf(err, "error copying unstructured object %q %s/%s to target cluster",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}

	if err := from.ForceDeleteUnstructuredObject(u); err != nil {
		return errors.Wrapf(err, "error force deleting unstructured object %q %s/%s to target cluster",
			u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	}
	klog.V(3).Infof("Successfully moved unstructured object %q %s/%s", u.GroupVersionKind(), u.GetNamespace(), u.GetName())
	return nil
}

func moveSecrets(from sourceClient, to targetClient, cluster *clusterv1.Cluster) error {
	klog.V(3).Infof("Moving Secrets for Cluster %s/%s", cluster.Namespace, cluster.Name)
	secrets, err := from.GetClusterSecrets(cluster)
	if err != nil {
		return err
	}

	toCluster, err := to.GetCluster(cluster.Name, cluster.Namespace)
	if err != nil {
		return err
	}

	for _, secret := range secrets {
		if err := moveSecret(from, to, secret, toCluster); err != nil {
			return errors.Wrapf(err, "failed to move Secret %s/%s", secret.Namespace, secret.Name)
		}
	}
	return nil
}

func moveSecret(from sourceClient, to targetClient, secret *corev1.Secret, toCluster *clusterv1.Cluster) error {
	klog.V(3).Infof("Moving secret %s/%s", secret.Namespace, secret.Name)

	// New objects cannot have a specified resource version. Clear it out.
	secret.SetResourceVersion("")

	// Set the cluster owner ref based on target cluster's Cluster resource
	if err := to.SetClusterOwnerRef(secret, toCluster); err != nil {
		return errors.Wrap(err, "failed to set ownerref to secret")
	}

	if err := to.CreateSecret(secret); err != nil {
		return errors.Wrapf(err, "error copying Secret %s/%s to target cluster", secret.Namespace, secret.Name)
	}

	if err := from.ForceDeleteSecret(secret.Namespace, secret.Name); err != nil {
		return errors.Wrapf(err, "error force deleting Secret %s/%s from source cluster", secret.Namespace, secret.Name)
	}
	klog.V(3).Infof("Successfully moved Secret %s/%s", secret.Namespace, secret.Name)
	return nil
}

func moveMachineDeployments(from sourceClient, to targetClient, machineDeployments []*clusterv1.MachineDeployment) error {
	machineDeploymentNames := make([]string, 0, len(machineDeployments))
	for _, md := range machineDeployments {
		machineDeploymentNames = append(machineDeploymentNames, md.Name)
	}
	klog.V(3).Infof("Preparing to move MachineDeployments: %v", machineDeploymentNames)

	for _, md := range machineDeployments {
		if err := moveMachineDeployment(from, to, md); err != nil {
			return errors.Wrapf(err, "failed to move MachineDeployment %s/%s", md.Namespace, md.Name)
		}
	}
	return nil
}

func moveMachineDeployment(from sourceClient, to targetClient, md *clusterv1.MachineDeployment) error {
	klog.V(3).Infof("Moving MachineDeployment %s/%s", md.Namespace, md.Name)

	klog.V(3).Infof("Retrieving list of MachineSets for MachineDeployment %s/%s", md.Namespace, md.Name)
	machineSets, err := from.GetMachineSetsForMachineDeployment(md)
	if err != nil {
		return err
	}
	if err := moveMachineSets(from, to, machineSets); err != nil {
		return err
	}

	// Move infrastructure reference.
	if err := moveReference(from, to, &md.Spec.Template.Spec.InfrastructureRef, md.Namespace); err != nil {
		return errors.Wrapf(err, "error copying MachineSet %s/%s infrastructure reference to target cluster",
			md.Namespace, md.Name)
	}

	// New objects cannot have a specified resource version. Clear it out.
	md.SetResourceVersion("")

	// Remove owner reference. This currently assumes that the only owner reference would be a Cluster.
	md.SetOwnerReferences(nil)

	if err := to.CreateMachineDeployments([]*clusterv1.MachineDeployment{md}, md.Namespace); err != nil {
		return errors.Wrapf(err, "error copying MachineDeployment %s/%s to target cluster", md.Namespace, md.Name)
	}

	if err := from.ForceDeleteMachineDeployment(md.Namespace, md.Name); err != nil {
		return errors.Wrapf(err, "error force deleting MachineDeployment %s/%s from source cluster", md.Namespace, md.Name)
	}
	klog.V(3).Infof("Successfully moved MachineDeployment %s/%s", md.Namespace, md.Name)
	return nil
}

func moveMachineSets(from sourceClient, to targetClient, machineSets []*clusterv1.MachineSet) error {
	machineSetNames := make([]string, 0, len(machineSets))
	for _, ms := range machineSets {
		machineSetNames = append(machineSetNames, ms.Name)
	}
	klog.V(3).Infof("Preparing to move MachineSets: %v", machineSetNames)

	for _, ms := range machineSets {
		if err := moveMachineSet(from, to, ms); err != nil {
			return errors.Wrapf(err, "failed to move MachineSet %s:%s", ms.Namespace, ms.Name)
		}
	}
	return nil
}

func moveMachineSet(from sourceClient, to targetClient, ms *clusterv1.MachineSet) error {
	klog.V(3).Infof("Moving MachineSet %s/%s", ms.Namespace, ms.Name)

	klog.V(3).Infof("Retrieving list of Machines for MachineSet %s/%s", ms.Namespace, ms.Name)
	machines, err := from.GetMachinesForMachineSet(ms)
	if err != nil {
		return err
	}
	if err := moveMachines(from, to, machines); err != nil {
		return err
	}

	// Move infrastructure reference if the MachineSet isn't linked to a MachineDeployment.
	// When a MachineSet is owned by a MachineDeployment, the referenced template has already been moved
	// by the time this function is called.
	if metav1.GetControllerOf(ms) == nil {
		if err := moveReference(from, to, &ms.Spec.Template.Spec.InfrastructureRef, ms.Namespace); err != nil {
			return errors.Wrapf(err, "error copying MachineSet %s/%s infrastructure reference to target cluster",
				ms.Namespace, ms.Name)
		}
	}

	// New objects cannot have a specified resource version. Clear it out.
	ms.SetResourceVersion("")

	// Remove owner reference. This currently assumes that the only owner references would be a MachineDeployment and/or a Cluster.
	ms.SetOwnerReferences(nil)

	if err := to.CreateMachineSets([]*clusterv1.MachineSet{ms}, ms.Namespace); err != nil {
		return errors.Wrapf(err, "error copying MachineSet %s/%s to target cluster", ms.Namespace, ms.Name)
	}
	if err := from.ForceDeleteMachineSet(ms.Namespace, ms.Name); err != nil {
		return errors.Wrapf(err, "error force deleting MachineSet %s/%s from source cluster", ms.Namespace, ms.Name)
	}
	klog.V(3).Infof("Successfully moved MachineSet %s/%s", ms.Namespace, ms.Name)
	return nil
}

func moveMachines(from sourceClient, to targetClient, machines []*clusterv1.Machine) error {
	machineNames := make([]string, 0, len(machines))
	for _, m := range machines {
		if m.DeletionTimestamp != nil {
			klog.V(3).Infof("Skipping to move deleted machine: %q", m.Name)
			continue
		}
		machineNames = append(machineNames, m.Name)
	}
	klog.V(3).Infof("Preparing to move Machines: %v", machineNames)

	for _, m := range machines {
		if !m.DeletionTimestamp.IsZero() {
			continue
		}
		if err := moveMachine(from, to, m); err != nil {
			return errors.Wrapf(err, "failed to move Machine %s:%s", m.Namespace, m.Name)
		}
	}
	return nil
}

func moveMachine(from sourceClient, to targetClient, m *clusterv1.Machine) error {
	klog.V(3).Infof("Moving Machine %s/%s", m.Namespace, m.Name)

	// Move bootstrap reference, if any.
	if m.Spec.Bootstrap.ConfigRef != nil {
		if err := moveReference(from, to, m.Spec.Bootstrap.ConfigRef, m.Namespace); err != nil {
			return errors.Wrapf(err, "error copying Machine %s/%s bootstrap reference to target cluster",
				m.Namespace, m.Name)
		}
	}

	// Move infrastructure reference.
	if err := moveReference(from, to, &m.Spec.InfrastructureRef, m.Namespace); err != nil {
		return errors.Wrapf(err, "error copying Machine %s/%s infrastructure reference to target cluster",
			m.Namespace, m.Name)
	}

	// New objects cannot have a specified resource version. Clear it out.
	m.SetResourceVersion("")

	// Remove owner reference. This currently assumes that the only owner references would be a MachineSet and/or a Cluster.
	m.SetOwnerReferences(nil)

	if err := to.CreateMachines([]*clusterv1.Machine{m}, m.Namespace); err != nil {
		return errors.Wrapf(err, "error copying Machine %s/%s to target cluster", m.Namespace, m.Name)
	}
	if err := from.ForceDeleteMachine(m.Namespace, m.Name); err != nil {
		return errors.Wrapf(err, "error force deleting Machine %s/%s from source cluster", m.Namespace, m.Name)
	}

	klog.V(3).Infof("Successfully moved Machine %s/%s", m.Namespace, m.Name)
	return nil
}
