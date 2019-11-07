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
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ctx = context.Background()
)

// Client allow access to the clusterctl library.
type Client interface {
	// Kubeconfig return the path to kubeconfig used to access to a management cluster.
	Kubeconfig() string

	// Kubeconfig return the K8SProxy used for operating objects in the management cluster.
	K8SProxy() K8SProxy

	// ProviderComponents returns a ComponentsClient object that can be user for
	// operating provider components objects in the management cluster (e.g. the controllers).
	ProviderComponents() ComponentsClient

	// ProviderMetadata returns a MetadataClient object that can be user for
	// operating provider metadata stored in the management cluster (e.g. the list of installed providers/versions).
	ProviderMetadata() MetadataClient

	// ProviderMetadata returns a ObjectsClient object that can be user for
	// operating cluster API objects stored in the management cluster (e.g. clusters, machines).
	ProviderObjects() ObjectsClient

	// ProviderInstaller return an ProviderInstallerService that supports provider installation
	// into the management cluster.
	ProviderInstaller() ProviderInstallerService

	// ProviderMover return an ProviderMoverService that supports pivoting providers from one
	// management cluster into another.
	ProviderMover() ProviderMoverService
}

// configClient implements ConfigClient.
type clusterClient struct {
	kubeconfig string
	k8sProxy   K8SProxy
}

// ensure clusterClient implements Client.
var _ Client = &clusterClient{}

func (c *clusterClient) Kubeconfig() string {
	return c.kubeconfig
}

func (c *clusterClient) K8SProxy() K8SProxy {
	return c.k8sProxy
}

func (c *clusterClient) ProviderComponents() ComponentsClient {
	return newComponentsClient(c.k8sProxy)
}

func (c *clusterClient) ProviderMetadata() MetadataClient {
	return newMetadataClient(c.k8sProxy)
}

func (c *clusterClient) ProviderObjects() ObjectsClient {
	return newObjectsClient(c.k8sProxy)
}

func (c *clusterClient) ProviderInstaller() ProviderInstallerService {
	return newProviderInstaller(c.k8sProxy, c.ProviderMetadata(), c.ProviderComponents())
}

func (c *clusterClient) ProviderMover() ProviderMoverService {
	return newProviderMover(c.ProviderMetadata(), c.ProviderComponents(), c.ProviderObjects())
}

// New returns a configClient.
func New(kubeconfig string, options Options) Client {
	return newClusterClient(kubeconfig, options)
}

func newClusterClient(kubeconfig string, options Options) *clusterClient {
	k8sProxy := options.InjectK8SProxy
	if k8sProxy == nil {
		k8sProxy = newK8SProxy(kubeconfig)
	}
	return &clusterClient{
		kubeconfig: kubeconfig,
		k8sProxy:   k8sProxy,
	}
}

// Options allow to set ConfigClient options
type Options struct {
	InjectK8SProxy K8SProxy
}

type K8SProxy interface {
	CurrentNamespace() (string, error)
	NewClient() (client.Client, error)
	ListResources(namespace string, labels map[string]string) ([]unstructured.Unstructured, error)
	ScaleDeployment(*appsv1.Deployment, int32) error
	PollImmediate(interval, timeout time.Duration, condition func() (done bool, err error)) error
}

var _ K8SProxy = &test.FakeK8SProxy{}
