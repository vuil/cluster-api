package client

import (
	"fmt"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
)

func Test_clusterctlClient_Init(t *testing.T) {
	type field struct {
		client *fakeClient
		hasCRD bool
	}

	type args struct {
		coreProvider           string
		bootstrapProvider      []string
		infrastructureProvider []string
		targetNameSpace        string
		watchingNamespace      string
		force                  bool
	}
	type want struct {
		provider          Provider
		version           string
		targetNamespace   string
		watchingNamespace string
	}

	tests := []struct {
		name    string
		field   field
		args    args
		want    []want
		wantErr bool
	}{
		{
			name: "Init (with an empty cluster) succeed when user installs default provider versions",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
				hasCRD: false,
			},
			args: args{
				coreProvider:           "", // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{"bootstrap"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.0.0",
					targetNamespace:   "ns1",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) succeed when user requires specific provider versions",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
				hasCRD: false,
			},
			args: args{
				coreProvider:           fmt.Sprintf("%s:v1.1.0", config.CAPI),
				bootstrapProvider:      []string{"bootstrap:v2.1.0"},
				infrastructureProvider: []string{"infra:v3.1.0"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.1.0",
					targetNamespace:   "ns1",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.1.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.1.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) succeed when user sets a target namespace",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
				hasCRD: false,
			},
			args: args{
				coreProvider:           "", // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{"bootstrap"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "nsx",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an NOT empty cluster) adds a provider",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
				hasCRD: true,
			},
			args: args{
				coreProvider:           "", // with a NOT empty cluster, a core provider should NOT be added automatically
				bootstrapProvider:      []string{"bootstrap"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Fails when coreProvider is a provider with the wrong type",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
			},
			args: args{
				coreProvider:           "infra",
				bootstrapProvider:      []string{"infra"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when bootstrapProvider list contains providers of the wrong type",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      []string{"infra"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when infrastructureProvider  list contains providers of the wrong type",
			field: field{
				client: fakeClient1(), // clusterctl client for an empty management cluster with capi, bootstrap and infra provider
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      []string{"bootstrap"},
				infrastructureProvider: []string{"bootstrap"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
		//TODO: test other failure conditions (unknown provider, unknown version)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.field.hasCRD {
				if _, err := tt.field.client.clusters["kubeconfig"].ProviderMetadata().EnsureMetadata(); err != nil {
					t.Fatalf("EnsureMetadata() error = %v", err)
					return
				}
			}

			got, _, err := tt.field.client.Init("kubeconfig", tt.args.coreProvider, tt.args.bootstrapProvider, tt.args.infrastructureProvider, tt.args.targetNameSpace, tt.args.watchingNamespace, tt.args.force)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("Init() got = %v items, want %v items", len(got), len(tt.want))
				return
			}

			for i, g := range got {
				w := tt.want[i]

				if g.Name() != w.provider.Name() {
					t.Errorf("Init(), Item[%d].Name() got = %v, want = %v ", i, g.Name(), w.provider.Name())
				}

				if g.Type() != w.provider.Type() {
					t.Errorf("Init(), Item[%d].Type() got = %v, want = %v ", i, g.Type(), w.provider.Type())
				}

				if g.Version() != w.version {
					t.Errorf("Init(), Item[%d].Version() got = %v, want = %v ", i, g.Version(), w.version)
				}

				if g.TargetNamespace() != w.targetNamespace {
					t.Errorf("Init(), Item[%d].TargetNamespace() got = %v, want = %v ", i, g.TargetNamespace(), w.targetNamespace)
				}

				if g.WatchingNamespace() != w.watchingNamespace {
					t.Errorf("Init(), Item[%d].WatchingNamespace() got = %v, want = %v ", i, g.WatchingNamespace(), w.watchingNamespace)
				}
			}
		})
	}
}

var (
	capiProviderConfig      = config.NewProvider(config.CAPI, "url", clusterctlv1.CoreProviderType)
	bootstrapProviderConfig = config.NewProvider("bootstrap", "url", clusterctlv1.BootstrapProviderType)
	infraProviderConfig     = config.NewProvider("infra", "url", clusterctlv1.InfrastructureProviderType)
)

func fakeClient1() *fakeClient {

	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(infraProviderConfig)
	cluster1 := newFakeCluster("kubeconfig").
		WithObjs()
	repository1 := newFakeRepository(capiProviderConfig, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1")).
		WithFile("v1.1.0", "components.yaml", componentsYAML("ns1"))
	repository2 := newFakeRepository(bootstrapProviderConfig, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns2")).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns2"))
	repository3 := newFakeRepository(infraProviderConfig, config1.Variables()).
		WithPaths("root", "components", "kustomize").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "components.yaml", componentsYAML("ns3")).
		WithFile("v3.1.0", "components.yaml", componentsYAML("ns3")).
		WithFile("v3.0.0", "config-kubeadm.yaml", templateYAML("ns3"))
	client := newFakeClient(config1).
		WithCluster(cluster1).
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3)

	return client
}

func componentsYAML(ns string) []byte {
	var namespaceYaml = []byte("apiVersion: v1\n" +
		"kind: Namespace\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s", ns))

	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Pod\n" +
		"metadata:\n" +
		"  name: manager")

	return util.JoinYaml(namespaceYaml, podYaml)
}

func templateYAML(ns string) []byte {
	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Pod\n" +
		"metadata:\n" +
		"  name: manager\n" +
		fmt.Sprintf("  namespace: %s", ns))

	return podYaml
}
