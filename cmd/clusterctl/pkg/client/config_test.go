package client

import (
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

//TODO: complete tests  with a fake client and assertions

//noinspection GoNilness,GoNilness,GoNilness,GoNilness,GoNilness,GoNilness,GoNilness,GoNilness,GoNilness,GoNilness
func Test_clusterctlClient_GetProviderConfig(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(infraProviderConfig, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "components.yaml", componentsYAML("ns1")).
		WithFile("v3.1.0", "components.yaml", componentsYAML("ns1"))

	client := newFakeClient(config1).
		WithRepository(repository1)

	type args struct {
		provider          string
		targetNameSpace   string
		watchingNamespace string
	}
	type want struct {
		provider          config.Provider
		version           string
		targetNamespace   string
		watchingNamespace string
		variables         []string
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "returns default provider version",
			args: args{
				provider:          "infra",
				targetNameSpace:   "",
				watchingNamespace: "",
			},
			want: want{
				provider:          infraProviderConfig,
				version:           "v3.0.0", // default version detected
				targetNamespace:   "ns1",    // default targetNamespace detected
				watchingNamespace: "",
				variables:         []string{}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "returns default specific version",
			args: args{
				provider:          "infra:v3.1.0",
				targetNameSpace:   "",
				watchingNamespace: "",
			},
			want: want{
				provider:          infraProviderConfig,
				version:           "v3.1.0", // default version detected
				targetNamespace:   "ns1",    // default targetNamespace detected
				watchingNamespace: "",
				variables:         []string{}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "allows namespace override",
			args: args{
				provider:          "infra",
				targetNameSpace:   "nsx",
				watchingNamespace: "",
			},
			want: want{
				provider:          infraProviderConfig,
				version:           "v3.0.0", // default version detected
				targetNamespace:   "nsx",
				watchingNamespace: "",
				variables:         []string{}, // variable detected
			},
			wantErr: false,
		},
		//TODO: test other failure conditions (unknown provider, unknown version)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetProviderConfig(tt.args.provider, tt.args.targetNameSpace, tt.args.watchingNamespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProviderConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.provider.Name() {
				t.Errorf("GetProviderConfig().Name() got = %v, want = %v ", got.Name(), tt.want.provider.Name())
			}

			if got.Type() != tt.want.provider.Type() {
				t.Errorf("GetProviderConfig().Type()  got = %v, want = %v ", got.Type(), tt.want.provider.Type())
			}

			if got.Version() != tt.want.version {
				t.Errorf("GetProviderConfig().Version() got = %v, want = %v ", got.Version(), tt.want.version)
			}

			if got.TargetNamespace() != tt.want.targetNamespace {
				t.Errorf("GetProviderConfig().TargetNamespace() got = %v, want = %v ", got.TargetNamespace(), tt.want.targetNamespace)
			}

			if got.WatchingNamespace() != tt.want.watchingNamespace {
				t.Errorf("GetProviderConfig().WatchingNamespace() got = %v, want = %v ", got.WatchingNamespace(), tt.want.watchingNamespace)
			}

		})
	}
}

func Test_clusterctlClient_GetProvidersConfig(t *testing.T) {
	type field struct {
		client Client
	}
	tests := []struct {
		name          string
		field         field
		wantProviders []string
		wantErr       bool
	}{
		{
			name: "Returns default providers",
			field: field{
				client: newFakeClient(newFakeConfig()),
			},
			wantProviders: []string{
				"aws",
				config.CAPI,
				"docker",
				"kubeadm",
				"vsphere",
			},
			wantErr: false,
		},
		{
			name: "Returns default providers and custom providers if defined",
			field: field{
				client: newFakeClient(newFakeConfig().WithProvider(bootstrapProviderConfig)),
			},
			wantProviders: []string{
				"aws",
				bootstrapProviderConfig.Name(),
				config.CAPI,
				"docker",
				"kubeadm",
				"vsphere",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.field.client.GetProvidersConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProvidersConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.wantProviders) {
				t.Errorf("Init() got = %v items, want %v items", len(got), len(tt.wantProviders))
				return
			}

			for i, g := range got {
				w := tt.wantProviders[i]

				if g.Name() != w {
					t.Errorf("GetProvidersConfig(), Item[%d].Name() got = %v, want = %v ", i, g.Name(), w)
				}
			}
		})
	}
}

func Test_clusterctlClient_GetClusterConfig(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(infraProviderConfig, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "config-kubeadm.yaml", templateYAML("ns3"))

	cluster1 := newFakeCluster("kubeconfig")

	client := newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1)

	type args struct {
		provider  string
		flavor    string
		bootstrap string
		options   TemplateOptions
	}

	type templateValues struct {
		name            string
		url             string
		ttype           clusterctlv1.ProviderType
		version         string
		flavor          string
		bootstrap       string
		path            string
		variables       []string
		targetNamespace string
		yaml            []byte
	}

	tests := []struct {
		name    string
		args    args
		want    templateValues
		wantErr bool
	}{
		{
			args: args{
				provider:  "infra:v3.0.0",
				flavor:    "",
				bootstrap: "kubeadm",
				options: TemplateOptions{
					ClusterName: "test",
					Namespace:   "ns1",
				},
			},
			want: templateValues{
				name:            "infra",
				url:             "url",
				ttype:           clusterctlv1.InfrastructureProviderType,
				version:         "v3.0.0",
				flavor:          "",
				bootstrap:       "kubeadm",
				path:            "config-kubeadm.yaml",
				variables:       nil,
				targetNamespace: "ns1",
				yaml:            templateYAML("ns1"),
			},
			//TODO: test default version and default namespace logic
			//TODO: test other failure conditions (unknown provider, unknown version. unknown flavor)
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetClusterTemplate("kubeconfig", tt.args.provider, tt.args.flavor, tt.args.bootstrap, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetClusterTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Name() != tt.want.name {
				t.Errorf("GetClusterTemplate().Name() got = %v, want %v", got.Name(), tt.want.name)
			}
			if got.URL() != tt.want.url {
				t.Errorf("GetClusterTemplate().URL() got = %v, want %v", got.URL(), tt.want.url)
			}
			if got.Type() != tt.want.ttype {
				t.Errorf("GetClusterTemplate().Type() got = %v, want %v", got.Type(), tt.want.ttype)
			}
			if got.Version() != tt.want.version {
				t.Errorf("GetClusterTemplate().Version() got = %v, want %v", got.Version(), tt.want.version)
			}
			if got.Flavor() != tt.want.flavor {
				t.Errorf("GetClusterTemplate().Flavor() got = %v, want %v", got.Flavor(), tt.want.flavor)
			}
			if got.Bootstrap() != tt.want.bootstrap {
				t.Errorf("GetClusterTemplate().Bootstrap() got = %v, want %v", got.Bootstrap(), tt.want.bootstrap)
			}
			if !reflect.DeepEqual(got.Variables(), tt.want.variables) {
				t.Errorf("GetClusterTemplate().Variables() got = %v, want %v", got.Variables(), tt.want.variables)
			}
			if got.TargetNamespace() != tt.want.targetNamespace {
				t.Errorf("GetClusterTemplate().TargetNamespace() got = %v, want %v", got.TargetNamespace(), tt.want.targetNamespace)
			}
			if !reflect.DeepEqual(got.Yaml(), tt.want.yaml) {
				t.Errorf("GetClusterTemplate().Yaml() got = %v, want %v", got.Yaml(), tt.want.yaml)
			}
		})
	}
}
