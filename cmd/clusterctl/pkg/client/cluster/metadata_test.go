package cluster

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_metadataClient_HasCRD(t *testing.T) {
	type fields struct {
		hasCRD bool
	}
	tests := []struct {
		name    string
		want    bool
		fields  fields
		wantErr bool
	}{
		{
			name: "Has not CRD",
			fields: fields{
				hasCRD: false,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Has CRD",
			fields: fields{
				hasCRD: true,
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newMetadataClient(test.NewFakeK8SProxy())
			if tt.fields.hasCRD {
				//forcing creation of metadata before test
				if _, err := p.EnsureMetadata(); err != nil {
					t.Errorf("EnsureMetadata() error = %v", err)
					return
				}
			}

			got, err := p.EnsureMetadata()
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EnsureMetadata() got = %v, want %v", got, tt.want)
			}

			//TODO: check metadatacrd exists
		})
	}
}

//TODO: add tests

var fooProvider = clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns1"}}

func Test_metadataClient_List(t *testing.T) {
	type fields struct {
		initObjs []runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		want    []clusterctlv1.Provider
		wantErr bool
	}{
		{
			name: "Get list",
			fields: fields{
				initObjs: []runtime.Object{
					&fooProvider,
				},
			},
			want: []clusterctlv1.Provider{
				fooProvider,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newMetadataClient(test.NewFakeK8SProxy().WithObjs(tt.fields.initObjs...))
			got, err := p.List()
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("List() got = %v, want %v", got, tt.want)
			}
		})
	}
}
