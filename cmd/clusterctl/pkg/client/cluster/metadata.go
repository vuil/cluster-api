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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetadataClient has methods to work with provider metadata in the cluster.
type MetadataClient interface {
	EnsureMetadata() (bool, error)
	Validate(clusterctlv1.Provider) error
	Create(clusterctlv1.Provider) error
	List() ([]clusterctlv1.Provider, error)
	Get(namespace, name string) (*clusterctlv1.Provider, error)
	GetDefaultProvider(providerType clusterctlv1.ProviderType) (string, error)
	GetDefaultVersion(provider string) (string, error)
	GetDefaultNamespace(provider string) (string, error)
}

type listOptions struct {
	Namespace string
	Name      string
	Type      clusterctlv1.ProviderType
}

// metadataClient implements MetadataClient.
type metadataClient struct {
	k8sproxy K8SProxy
}

// ensure metadataClient implements MetadataClient.
var _ MetadataClient = &metadataClient{}

// neProviderMetadata returns a metadataClient.
func newMetadataClient(k8sproxy K8SProxy) *metadataClient {
	return &metadataClient{
		k8sproxy: k8sproxy,
	}
}

// EnsureMetadata
func (p *metadataClient) EnsureMetadata() (bool, error) {
	// Important! when a new release of clusterctl API is created, this is the point where to detect that metadata should be migrated and
	// to implement conversion (if a cluster has old CRD version, install new CRD version, migrate objects, delete old CRD version)

	yaml, err := config.Asset("manifest/clusterctl-api.yaml")
	if err != nil {
		return false, err
	}

	// transform the yaml in a list of objects
	objs, err := util.ToUnstructured(yaml)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse yaml for clusterctl metadata")
	}

	c, err := p.k8sproxy.NewClient()
	if err != nil {
		return false, err
	}

	for _, o := range objs {
		klog.V(3).Infof("Creating: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		if err = c.Create(ctx, &o); err != nil { //nolint
			if apierrors.IsAlreadyExists(err) {
				return true, nil
			}
			return false, errors.Wrapf(err, "failed to create clusterctl metadata")
		}
	}

	return false, errors.Wrapf(err, "invalid yaml for clusterctl metadata")
}

func (p *metadataClient) Validate(m clusterctlv1.Provider) error {
	instances, err := p.list(listOptions{
		Name: m.Name,
	})
	if err != nil {
		return err
	}

	if len(instances) == 0 {
		return nil
	}

	//TODO: return all the errors at once

	// Target Namespace check
	// Installing two instances of the same provider in the same namespace won't be supported
	for _, i := range instances {
		if i.Namespace == m.Namespace {
			return errors.Errorf("There is already an instance of the %q provider installed in the %q namespace", m.Name, m.Namespace)
		}
	}

	// Version check:
	// If we are going to install an instance of a provider with version X, and there are already other instances of the same provider with
	// different versions there is the risk of creating problems to all the version different than X because we are going to override
	// all the existing non-namespaced objects (e.g. CRDs) with the ones from version X
	sameVersion := false
	for _, i := range instances {
		if i.Version == m.Version {
			sameVersion = true
		}
	}
	if !sameVersion {
		return errors.Errorf("The new instance of the %q provider has a version different than other instances of the same provider", m.Name)
	}

	// Watching Namespace check:
	// If we are going to install an instance of a provider watching objects in namespaces already controlled by other providers
	// then there will be providers fighting for objects...
	if m.WatchedNamespace == "" {
		return errors.Errorf("The new instance of the %q provider is going to watch for objects in namespaces already controlled by other providers", m.Name)
	}

	sameNamespace := false
	for _, i := range instances {
		if i.WatchedNamespace == "" || m.WatchedNamespace == i.WatchedNamespace {
			sameNamespace = true
		}
	}
	if sameNamespace {
		return errors.Errorf("The new instance of the %q provider is going to watch for objects in the namespace %q that is already controlled by other providers", m.Name, m.WatchedNamespace)
	}

	return nil
}

// Create metadata for a provider instance installed in the cluster.
func (p *metadataClient) Create(m clusterctlv1.Provider) error {
	cl, err := p.k8sproxy.NewClient()
	if err != nil {
		return err
	}

	currentProvider := &clusterctlv1.Provider{}
	key := client.ObjectKey{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
	if err = cl.Get(ctx, key, currentProvider); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get current provider object")
		}
		currentProvider = nil
	}

	c := m.DeepCopyObject()
	if currentProvider == nil {
		if err = cl.Create(ctx, c); err != nil {
			return errors.Wrapf(err, "failed to create provider object")
		}
	} else {
		m.ResourceVersion = currentProvider.ResourceVersion
		if err = cl.Update(ctx, c); err != nil {
			return errors.Wrapf(err, "failed to update provider object")
		}
	}

	return nil
}

// Return metadata for all the provider instances installed in the cluster.
func (p *metadataClient) List() ([]clusterctlv1.Provider, error) {
	return p.list(listOptions{})
}

func (p *metadataClient) list(options listOptions) ([]clusterctlv1.Provider, error) {
	cl, err := p.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	l := &clusterctlv1.ProviderList{}
	if err := cl.List(ctx, l); err != nil {
		return nil, errors.Wrap(err, "failed get providers")
	}

	var ret []clusterctlv1.Provider //nolint
	for _, i := range l.Items {
		if options.Name != "" && i.Name != options.Name {
			continue
		}
		if options.Namespace != "" && i.Namespace != options.Namespace {
			continue
		}
		if options.Type != "" && i.Type != string(options.Type) {
			continue
		}
		ret = append(ret, i)
	}
	return ret, nil
}

func (p *metadataClient) Get(namespace, name string) (*clusterctlv1.Provider, error) {
	cl, err := p.k8sproxy.NewClient()
	if err != nil {
		return nil, err
	}

	provider := &clusterctlv1.Provider{}
	key := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err = cl.Get(ctx, key, provider); err != nil {
		return nil, errors.Wrapf(err, "failed to get provider %s/%s", namespace, name)
	}

	return provider, nil
}

func (p *metadataClient) GetDefaultProvider(providerType clusterctlv1.ProviderType) (string, error) {
	l, err := p.list(listOptions{
		Type: providerType,
	})
	if err != nil {
		return "", err
	}

	names := sets.NewString()
	for _, p := range l {
		names.Insert(p.Name)
	}

	if names.Len() == 1 {
		return names.List()[0], nil
	}

	// there no provider/more than one of this type; in both cases, it is not possible to get a default provider name
	return "", nil
}

func (p *metadataClient) GetDefaultVersion(provider string) (string, error) {
	l, err := p.list(listOptions{
		Name: provider,
	})
	if err != nil {
		return "", err
	}

	versions := sets.NewString()
	for _, p := range l {
		versions.Insert(p.Version)
	}

	if versions.Len() == 1 {
		return versions.List()[0], nil
	}

	// there no provider/more than one with this name; in both cases, it is not possible to get a default provider version
	return "", nil
}

func (p *metadataClient) GetDefaultNamespace(provider string) (string, error) {
	l, err := p.list(listOptions{
		Name: provider,
	})
	if err != nil {
		return "", err
	}

	namespaces := sets.NewString()
	for _, p := range l {
		namespaces.Insert(p.Namespace)
	}

	if namespaces.Len() == 1 {
		return namespaces.List()[0], nil
	}

	// there no provider/more than one with this name; in both cases, it is not possible to get a default provider namespace
	return "", nil
}
