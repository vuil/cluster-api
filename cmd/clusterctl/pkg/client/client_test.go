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

package client

import (
	"testing"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// dummy test to document Fake client syntax
func TestNewFakeClient(t *testing.T) {
	repository1Config := config.NewProvider("p1", "url", clusterctlv1.CoreProviderType)

	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(repository1Config)

	cluster1 := newFakeCluster("kubeconfig").
		WithObjs()

	repository1 := newFakeRepository(repository1Config, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v1.0").
		WithFile("v1.0", "components.yaml", []byte("content"))

	newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1)
}

type fakeClient struct {
	configClient   config.Client
	clusters       map[string]cluster.Client
	repositories   map[string]repository.Client
	internalclient *clusterctlClient
}

var _ Client = &fakeClient{}

func (f fakeClient) GetProvidersConfig() ([]Provider, error) {
	return f.internalclient.GetProvidersConfig()
}

func (f fakeClient) GetProviderConfig(provider, targetNameSpace, watchingNamespace string) (Components, error) {
	return f.internalclient.GetProviderConfig(provider, targetNameSpace, watchingNamespace)
}

func (f fakeClient) GetClusterTemplate(kubeconfig, provider, flavor, bootstrap string, options TemplateOptions) (Template, error) {
	return f.internalclient.GetClusterTemplate(kubeconfig, provider, flavor, bootstrap, options)
}

func (f fakeClient) Init(kubeconfig string, coreProvider string, bootstrapProvider, infrastructureProvider []string, targetNameSpace, watchingNamespace string, force bool) ([]Components, bool, error) {
	return f.internalclient.Init(kubeconfig, coreProvider, bootstrapProvider, infrastructureProvider, targetNameSpace, watchingNamespace, force)
}

func (f fakeClient) Pivot(fromKubeconfig, toKubeconfig string) error {
	return f.internalclient.Pivot(fromKubeconfig, toKubeconfig)
}

func (f fakeClient) Delete(kubeconfig string, forceDeleteNamespace, forceDeleteCRD bool, args ...string) error {
	return f.internalclient.Delete(kubeconfig, forceDeleteNamespace, forceDeleteCRD, args...)
}

func newFakeClient(configClient config.Client) *fakeClient {

	fake := &fakeClient{
		clusters:     map[string]cluster.Client{},
		repositories: map[string]repository.Client{},
	}

	fake.configClient = configClient
	if fake.configClient == nil {
		fake.configClient = newFakeConfig()
	}

	options := Options{
		InjectConfig: fake.configClient,
		InjectClusterFactory: func(kubeconfig string) (cluster.Client, error) {
			if _, ok := fake.clusters[kubeconfig]; !ok {
				return nil, errors.Errorf("Cluster for kubeconfig %q does not exists.", kubeconfig)
			}
			return fake.clusters[kubeconfig], nil
		},
		InjectRepositoryFactory: func(provider config.Provider) (repository.Client, error) {
			if _, ok := fake.repositories[provider.Name()]; !ok {
				return nil, errors.Errorf("Repository for kubeconfig %q does not exists.", provider.Name())
			}
			return fake.repositories[provider.Name()], nil
		},
	}

	fake.internalclient, _ = newClusterctlClient("fake-config", options)

	return fake
}

func (f *fakeClient) WithCluster(clusterClient cluster.Client) *fakeClient {
	f.clusters[clusterClient.Kubeconfig()] = clusterClient
	return f
}

func (f *fakeClient) WithRepository(repositoryClient repository.Client) *fakeClient {
	if fc, ok := f.configClient.(fakeConfigClient); ok {
		fc.WithProvider(repositoryClient)
	}
	f.repositories[repositoryClient.Name()] = repositoryClient
	return f
}

type fakeClusterClient struct {
	kubeconfig     string
	fakeK8SProxy   *test.FakeK8SProxy
	internalclient cluster.Client
}

var _ cluster.Client = &fakeClusterClient{}

func (f fakeClusterClient) Kubeconfig() string {
	return f.kubeconfig
}

func (f fakeClusterClient) K8SProxy() cluster.K8SProxy {
	return f.fakeK8SProxy
}

func (f fakeClusterClient) ProviderComponents() cluster.ComponentsClient {
	return f.internalclient.ProviderComponents()
}

func (f fakeClusterClient) ProviderMetadata() cluster.MetadataClient {
	return f.internalclient.ProviderMetadata()
}

func (f fakeClusterClient) ProviderObjects() cluster.ObjectsClient {
	return f.internalclient.ProviderObjects()
}

func (f fakeClusterClient) ProviderInstaller() cluster.ProviderInstallerService {
	return f.internalclient.ProviderInstaller()
}

func (f fakeClusterClient) ProviderMover() cluster.ProviderMoverService {
	return f.internalclient.ProviderMover()
}

func newFakeCluster(kubeconfig string) *fakeClusterClient {
	fakeK8SProxy := test.NewFakeK8SProxy()

	options := cluster.Options{
		InjectK8SProxy: fakeK8SProxy,
	}

	client := cluster.New("", options)

	return &fakeClusterClient{
		kubeconfig:     kubeconfig,
		fakeK8SProxy:   fakeK8SProxy,
		internalclient: client,
	}
}

func (f *fakeClusterClient) WithObjs(objs ...runtime.Object) *fakeClusterClient {
	f.fakeK8SProxy.WithObjs(objs...)
	return f
}

type fakeConfigClient struct {
	fakeReader     *test.FakeReader
	internalclient config.Client
}

var _ config.Client = &fakeConfigClient{}

func (f fakeConfigClient) Providers() config.ProvidersClient {
	return f.internalclient.Providers()
}

func (f fakeConfigClient) Variables() config.VariablesClient {
	return f.internalclient.Variables()
}

func newFakeConfig() *fakeConfigClient {
	fakeReader := test.NewFakeReader()

	client, _ := config.New("fake-config", config.InjectReader(fakeReader))

	return &fakeConfigClient{
		fakeReader:     fakeReader,
		internalclient: client,
	}
}

func (f *fakeConfigClient) WithVar(key, value string) *fakeConfigClient {
	f.fakeReader.WithVar(key, value)
	return f
}

func (f *fakeConfigClient) WithProvider(provider config.Provider) *fakeConfigClient {
	f.fakeReader.WithProvider(provider.Name(), provider.Type(), provider.URL())
	return f
}

type fakeRepositoryClient struct {
	config.Provider
	fakeRepository *test.FakeRepository
	client         repository.Client
}

var _ repository.Client = &fakeRepositoryClient{}

func (f fakeRepositoryClient) DefaultVersion() string {
	return f.fakeRepository.DefaultVersion()
}

func (f fakeRepositoryClient) Components() repository.ComponentsClient {
	return f.client.Components()
}
func (f fakeRepositoryClient) Templates(version string) repository.TemplatesClient {
	return f.client.Templates(version)
}

func newFakeRepository(provider config.Provider, configVariablesClient config.VariablesClient) *fakeRepositoryClient {
	fakeRepository := test.NewFakeRepository()
	options := repository.Options{
		InjectRepository: fakeRepository,
	}

	if configVariablesClient == nil {
		configVariablesClient = newFakeConfig().Variables()
	}

	client, _ := repository.New(provider, configVariablesClient, options)

	return &fakeRepositoryClient{
		Provider:       provider,
		fakeRepository: fakeRepository,
		client:         client,
	}
}

func (f *fakeRepositoryClient) WithPaths(rootPath, componentsPath, kustomizeDir string) *fakeRepositoryClient {
	f.fakeRepository.WithPaths(rootPath, componentsPath, kustomizeDir)
	return f
}

func (f *fakeRepositoryClient) WithDefaultVersion(version string) *fakeRepositoryClient {
	f.fakeRepository.WithDefaultVersion(version)
	return f
}

func (f *fakeRepositoryClient) WithFile(version, path string, content []byte) *fakeRepositoryClient {
	f.fakeRepository.WithFile(version, path, content)
	return f
}

func Test_parseProviderName(t *testing.T) {
	type args struct {
		provider string
	}
	tests := []struct {
		name          string
		args          args
		wantNamespace string
		wantName      string
		wantVersion   string
		wantErr       bool
	}{
		{
			name: "simple name",
			args: args{
				provider: "provider",
			},
			wantNamespace: "",
			wantName:      "provider",
			wantVersion:   "",
			wantErr:       false,
		},
		{
			name: "name & version",
			args: args{
				provider: "provider:version",
			},
			wantNamespace: "",
			wantName:      "provider",
			wantVersion:   "version",
			wantErr:       false,
		},
		{
			name: "namespace & name",
			args: args{
				provider: "namespace/provider",
			},
			wantNamespace: "namespace",
			wantName:      "provider",
			wantVersion:   "",
			wantErr:       false,
		},
		{
			name: "namespace, name & version",
			args: args{
				provider: "namespace/provider:version",
			},
			wantNamespace: "namespace",
			wantName:      "provider",
			wantVersion:   "version",
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotName, gotVersion, err := parseProviderName(tt.args.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProviderName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNamespace != tt.wantNamespace {
				t.Errorf("parseProviderName() gotNamespace = %v, want %v", gotNamespace, tt.wantNamespace)
			}
			if gotName != tt.wantName {
				t.Errorf("parseProviderName() gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("parseProviderName() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}
