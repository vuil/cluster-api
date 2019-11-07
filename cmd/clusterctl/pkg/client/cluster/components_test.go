package cluster

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_providerComponents_ScaleDownControllers(t *testing.T) {
	type fields struct {
		initObjs []runtime.Object
	}
	type args struct {
		provider v1alpha3.Provider
	}
	var one int32 = 1
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Test1",
			fields: fields{
				initObjs: []runtime.Object{
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind: "Deployment",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "d1",
						},
						Spec: appsv1.DeploymentSpec{
							Replicas: &one,
						},
					},
				},
			},
			args: args{
				provider: clusterctlv1.Provider{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "ns1",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newComponentsClient(test.NewFakeK8SProxy().WithObjs(tt.fields.initObjs...))
			if err := p.ScaleDownControllers(tt.args.provider); (err != nil) != tt.wantErr {
				t.Errorf("ScaleDownControllers() error = %v, wantErr %v", err, tt.wantErr)
			}

			// TODO: review implementation in order to make possible to check results
		})
	}
}

func Test_providerComponents_Delete(t *testing.T) {
	labels := map[string]string{
		clusterctlv1.ClusterctlProviderLabelName: "aws",
	}

	crd := unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	crd.SetName("crd1")
	crd.SetLabels(labels)

	initObjs := []runtime.Object{
		// Namespace (should be deleted only if forceDeleteNamespace)
		&corev1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind: "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ns1",
				Labels: labels,
			},
		},
		// Component object in namespace (should always be deleted)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "pod1",
				Labels:    labels,
			},
		},
		// Other object in namespace without labels (should go away only when deleting ns)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns1",
				Name:      "pod2",
			},
		},
		// Other object out of namespace (should never be deleted)
		&corev1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind: "Pod",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns2",
				Name:      "pod3",
				Labels:    labels,
			},
		},
		// CRD (should be deleted only if forceDeleteCRD)
		&crd,
	}

	type args struct {
		provider             clusterctlv1.Provider
		forceDeleteNamespace bool
		forceDeleteCRD       bool
	}
	tests := []struct {
		name     string
		args     args
		wantDiff []string
		wantErr  bool
	}{
		{
			name: "",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: false,
				forceDeleteCRD:       false,
			},
			wantDiff: []string{
				//"=|-\\APIVersion\\Kind\\Namespace\\Name",
				"=\\v1\\Namespace\\\\ns1",
				"-\\v1\\Pod\\ns1\\pod1",
				"=\\v1\\Pod\\ns1\\pod2",
				"=\\v1\\Pod\\ns2\\pod3",
				"=\\apiextensions.k8s.io/v1beta1\\CustomResourceDefinition\\\\crd1",
			},
			wantErr: false,
		},
		{
			name: "",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: true,
				forceDeleteCRD:       false,
			},
			wantDiff: []string{
				//"=|-\\APIVersion\\Kind\\Namespace\\Name",
				"-\\v1\\Namespace\\\\ns1",
				"-\\v1\\Pod\\ns1\\pod1",
				"-\\v1\\Pod\\ns1\\pod2",
				"=\\v1\\Pod\\ns2\\pod3",
				"=\\apiextensions.k8s.io/v1beta1\\CustomResourceDefinition\\\\crd1",
			},
			wantErr: false,
		},

		{
			name: "",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: false,
				forceDeleteCRD:       true,
			},
			wantDiff: []string{
				//"=|-\\APIVersion\\Kind\\Namespace\\Name",
				"=\\v1\\Namespace\\\\ns1",
				"-\\v1\\Pod\\ns1\\pod1",
				"=\\v1\\Pod\\ns1\\pod2",
				"=\\v1\\Pod\\ns2\\pod3",
				"-\\apiextensions.k8s.io/v1beta1\\CustomResourceDefinition\\\\crd1",
			},
			wantErr: false,
		},

		{
			name: "",
			args: args{
				provider:             clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "ns1"}},
				forceDeleteNamespace: true,
				forceDeleteCRD:       true,
			},
			wantDiff: []string{
				//"=|-\\APIVersion\\Kind\\Namespace\\Name",
				"-\\v1\\Namespace\\\\ns1",
				"-\\v1\\Pod\\ns1\\pod1",
				"-\\v1\\Pod\\ns1\\pod2",
				"=\\v1\\Pod\\ns2\\pod3",
				"-\\apiextensions.k8s.io/v1beta1\\CustomResourceDefinition\\\\crd1",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sproxy := test.NewFakeK8SProxy().WithObjs(initObjs...)
			p := newComponentsClient(k8sproxy)
			if err := p.Delete(tt.args.provider, tt.args.forceDeleteNamespace, tt.args.forceDeleteCRD); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}

			cs, err := k8sproxy.NewClient()
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			for _, o := range tt.wantDiff {
				k := strings.Split(o, "\\")

				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion(k[1])
				obj.SetKind(k[2])

				key := client.ObjectKey{
					Namespace: k[3],
					Name:      k[4],
				}

				err := cs.Get(ctx, key, obj)
				if err != nil && !apierrors.IsNotFound(err) {
					t.Fatalf("Get() %v", err)
				}

				if k[0] == "=" {
					if apierrors.IsNotFound(err) {
						t.Errorf("Delete() %v deleted in source cluster (it should be skipped)", key)
					}
				}

				if k[0] == "-" {
					if !apierrors.IsNotFound(err) {
						if k[3] == "ns1" && tt.args.forceDeleteNamespace {
							continue
						}

						t.Errorf("Delete() %v not deleted in source cluster", key)
					}
				}
			}
		})
	}
}
