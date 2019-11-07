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

package config

import (
	"net/url"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

const (
	CAPI               = "cluster-api"
	ProvidersConfigKey = "providers"
)

// ProvidersClient has methods to work with provider configurations.
type ProvidersClient interface {
	Defaults() []Provider
	List() ([]Provider, error)
	Get(name string) (*Provider, error)
}

// providersClient implements ProvidersClient.
type providersClient struct {
	providers []Provider
	reader    Reader
}

// ensure providersClient implements ProvidersClient.
var _ ProvidersClient = &providersClient{}

// newProvidersClient returns a providersClient.
func newProvidersClient(reader Reader) *providersClient {
	return &providersClient{
		reader: reader,
	}
}

// Defaults returns the list of clusterctl default provider configurations.
func (p *providersClient) Defaults() []Provider {

	defaults := []Provider{
		// cluster API core provider
		&provider{
			name:  CAPI,
			url:   "https://github.com/kubernetes-sigs/cluster-api/releases/latest/cluster-api-components.yaml",
			ttype: clusterctlv1.CoreProviderType,
		},

		// Infrastructure providersClient
		&provider{
			name:  "aws",
			url:   "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/latest/infrastructure-components.yaml",
			ttype: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:  "docker",
			url:   "https://github.com/kubernetes-sigs/cluster-api-provider-docker/releases/latest/provider_components.yaml",
			ttype: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:  "vsphere",
			url:   "https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/infrastructure-components.yaml",
			ttype: clusterctlv1.InfrastructureProviderType,
		},

		// Bootstrap providersClient
		&provider{
			name:  "kubeadm",
			url:   "https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases/latest/bootstrap-components.yaml",
			ttype: clusterctlv1.BootstrapProviderType,
		},
	}

	// ensure defaults are consistently sorted
	sort.Slice(defaults, func(i, j int) bool {
		return defaults[i].Name() < defaults[j].Name()
	})

	return defaults
}

// List returns all the provider configurations, including Defaults and user defined provider configurations.
// In case of conflict, user defined provider configurations override provider configurations.
func (p *providersClient) List() ([]Provider, error) {
	// Use cached list if available
	if p.providers != nil {
		return p.providers, nil
	}

	// Creates a maps with all the defaults provider configurations
	rMap := map[string]Provider{}
	for _, r := range p.Defaults() {
		rMap[r.Name()] = r
	}

	// Gets user defined provider configurations, validate them, and merges with
	// defaults provider configurations handling conflicts
	type configProvider struct {
		Name string                    `json:"name,omitempty"`
		URL  string                    `json:"url,omitempty"`
		Type clusterctlv1.ProviderType `json:"type,omitempty"`
	}

	var ur []configProvider //nolint
	err := p.reader.UnmarshalKey(ProvidersConfigKey, &ur)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal providers from the clusterctl configuration file")
	}

	for _, u := range ur {
		r := NewProvider(u.Name, u.URL, u.Type)

		err := validateProviderRepository(r)
		if err != nil {
			return nil, errors.Wrapf(err, "error validating configuration from %q. Please fix the providers value in clusterctl configuration file", r.Name())
		}

		if _, ok := rMap[r.Name()]; ok {
			klog.V(3).Infof("The clusterctl configuration file overrides default configuration for provider %s", r.Name())
		}
		rMap[r.Name()] = r
	}

	// Converts provider configurations maps to a slice
	p.providers = nil //nolint
	for _, r := range rMap {
		p.providers = append(p.providers, r)
	}

	// ensure provider configurations are consistently sorted
	sort.Slice(p.providers, func(i, j int) bool {
		return p.providers[i].Name() < p.providers[j].Name()
	})

	return p.providers, nil
}

// Get return the provider configuration with the given key. In case the key does not exists, an error is returned.
func (p *providersClient) Get(name string) (*Provider, error) {
	l, err := p.List()
	if err != nil {
		return nil, err
	}

	for _, r := range l {
		if name == r.Name() {
			return &r, nil
		}
	}

	return nil, errors.Errorf("failed to get configuration for %q provider. Please check the provider name and/or add configuration for new providers using the .clusterctl config file", name)
}

// validateProviderRepository validate a provider configuration.
func validateProviderRepository(r Provider) error {
	if r.Name() == "" {
		return errors.New("name value cannot be empty")
	}
	errMsgs := validation.IsDNS1123Subdomain(r.Name())
	if len(errMsgs) != 0 {
		return errors.Errorf("invalid name value: %s", strings.Join(errMsgs, "; "))
	}
	if r.URL() == "" {
		return errors.New("url value cannot be empty")
	}

	_, err := url.Parse(r.URL())
	if err != nil {
		return errors.Wrap(err, "error parsing URL")
	}

	if r.Type() == "" {
		return errors.New("type value cannot be empty")
	}

	switch r.Type() {
	case clusterctlv1.CoreProviderType,
		clusterctlv1.BootstrapProviderType,
		clusterctlv1.InfrastructureProviderType:
		break
	default:
		return errors.Errorf("invalid type value. Allowed values are [%s, %s, %s]",
			clusterctlv1.CoreProviderType,
			clusterctlv1.BootstrapProviderType,
			clusterctlv1.InfrastructureProviderType)
	}
	return nil
}
