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
	"time"

	"github.com/pkg/errors"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
)

// ObjectsClient has methods to work with provider objects in the cluster.
type ObjectsClient interface {
	Pivot(to ObjectsClient) error

	WaitForClusterV1alpha2Ready() error
	sourceClient
	targetClient
}

// objectsClient implements ObjectsClient.
type objectsClient struct {
	k8sproxy K8SProxy
}

// ensure objectsClient implements ObjectsClient.
var _ ObjectsClient = &objectsClient{}

// assumptions:
// - provider components scaled down to 0 replicas in the from cluster (controller shut down)
// - provider components installed in the to cluster (ready to install)
func (c *objectsClient) Pivot(to ObjectsClient) error {
	from := c

	klog.V(3).Info("Ensuring cluster v1alpha2 resources are available on the source cluster")
	if err := from.WaitForClusterV1alpha2Ready(); err != nil {
		return errors.New("cluster v1alpha2 resource not ready on source cluster")
	}

	klog.V(3).Info("Ensuring cluster v1alpha2 resources are available on the target cluster")
	if err := to.WaitForClusterV1alpha2Ready(); err != nil {
		return errors.New("cluster v1alpha2 resource not ready on target cluster")
	}

	klog.V(3).Info("Retrieving list of Clusters to move")
	clusters, err := from.GetClusters("")
	if err != nil {
		return err
	}

	if err := moveClusters(from, to, clusters); err != nil {
		return err
	}

	klog.V(3).Info("Retrieving list of MachineDeployments not associated with a Cluster to move")
	machineDeployments, err := from.GetMachineDeployments("")
	if err != nil {
		return err
	}
	if err := moveMachineDeployments(from, to, machineDeployments); err != nil {
		return err
	}

	klog.V(3).Info("Retrieving list of MachineSets not associated with a MachineDeployment or a Cluster to move")
	machineSets, err := from.GetMachineSets("")
	if err != nil {
		return err
	}
	if err := moveMachineSets(from, to, machineSets); err != nil {
		return err
	}

	klog.V(3).Infof("Retrieving list of Machines not associated with a MachineSet or a Cluster to move")
	machines, err := from.GetMachines("")
	if err != nil {
		return err
	}
	if err := moveMachines(from, to, machines); err != nil {
		return err
	}

	return nil
}

func (c *objectsClient) WaitForClusterV1alpha2Ready() error {
	deadline := time.Now().Add(timeoutResourceReady)
	timeout := time.Until(deadline)
	return c.k8sproxy.PollImmediate(retryIntervalResourceReady, timeout, func() (bool, error) {
		cs, err := c.k8sproxy.NewClient()
		if err != nil {
			return false, err
		}

		klog.V(2).Infof("Waiting for Cluster resources to be listable...")
		clusterList := &clusterv1.ClusterList{}
		if err := cs.List(ctx, clusterList); err == nil {
			return true, nil
		}
		return false, nil
	})
}

// newProviderObjects returns a objectsClient.
func newObjectsClient(k8sproxy K8SProxy) *objectsClient {
	return &objectsClient{
		k8sproxy: k8sproxy,
	}
}
