package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	clusterv13 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_objectsClient_Pivot(t *testing.T) {
	type fields struct {
		obj []runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Pivot a cluster",
			fields: fields{
				obj: sourceClusterObjects(),
			},
			wantErr: false,
		},
		{
			name: "Pivot a stand alone MachineDeployment",
			fields: fields{
				obj: sourceStandaloneMachineDeploymentObjects(),
			},
			wantErr: false,
		},
		{
			name: "Pivot a stand alone MachineSet",
			fields: fields{
				obj: sourceStandaloneMachineSetObjects(),
			},
			wantErr: false,
		},
		{
			name: "Pivot a stand alone Machine",
			fields: fields{
				obj: sourceStandaloneMachineObjects(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fromk8sproxy := test.NewFakeK8SProxy().WithObjs(tt.fields.obj...)
			from := newObjectsClient(fromk8sproxy)

			tok8sproxy := test.NewFakeK8SProxy()
			to := New("", Options{
				InjectK8SProxy: tok8sproxy,
			})
			if err := from.Pivot(to.ProviderObjects()); (err != nil) != tt.wantErr {
				t.Errorf("Pivot() error = %v, wantErr %v", err, tt.wantErr)
			}

			csFrom, _ := fromk8sproxy.NewClient()
			csTo, _ := tok8sproxy.NewClient()
			for _, o := range tt.fields.obj {
				key, _ := client.ObjectKeyFromObject(o)

				// objects are created in the target cluster
				oTo := o.DeepCopyObject()
				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("Pivot() error = %v when checking for %v created in target cluster", err, key)
					continue
				}

				// objects are deleted from the source cluster
				oFrom := o.DeepCopyObject()
				err := csFrom.Get(ctx, key, oFrom)
				if err == nil {
					t.Errorf("Pivot() %v not deleted in source cluster", key)
					continue
				}
				if !apierrors.IsNotFound(err) {
					t.Errorf("Pivot() error = %v when checking for %v deleted in source cluster", err, key)
					continue
				}

				t.Logf("check %s, Namespace=%s, Name=%s moved", o.GetObjectKind().GroupVersionKind().String(), key.Namespace, key.Name)
			}
		})
	}
}

const ns1 = "ns1"

func sourceClusterObjects() []runtime.Object {
	b := newFakeclusterBuilder().
		// A cluster With
		WithCluster(ns1, "cluster1").
		// - MachineDeployment with two MachineSet with two and one Machine each
		WithMachineDeployment(ns1, "cluster1", "deployment1").
		WithMachineSet(ns1, "cluster1", "deployment1", "machineset1").
		WithMachine(ns1, "cluster1", "machineset1", "machine1").
		WithMachine(ns1, "cluster1", "machineset1", "machine2").
		WithMachineSet(ns1, "cluster1", "machinedeployment1", "machineset2").
		WithMachine(ns1, "cluster1", "machineset2", "machine3").

		// - MachineSet with two machines
		WithMachineSet(ns1, "cluster1", "", "machineset3").
		WithMachine(ns1, "cluster1", "machineset3", "machine4").
		WithMachine(ns1, "cluster1", "", "machine5").

		// - three Machines
		WithMachine(ns1, "cluster1", "", "machine6").
		WithMachine(ns1, "cluster1", "", "machine7").
		WithMachine(ns1, "cluster1", "", "machine")

	return b.obj
}

func sourceStandaloneMachineDeploymentObjects() []runtime.Object {
	b := newFakeclusterBuilder().
		WithMachineDeployment(ns1, "", "deployment1").
		WithMachineSet(ns1, "", "deployment1", "machineset1").
		WithMachine(ns1, "", "machineset1", "machine1")
	return b.obj
}

func sourceStandaloneMachineSetObjects() []runtime.Object {
	b := newFakeclusterBuilder().
		WithMachineSet(ns1, "", "", "machineset1").
		WithMachine(ns1, "", "machineset1", "machine1")
	return b.obj
}

func sourceStandaloneMachineObjects() []runtime.Object {
	b := newFakeclusterBuilder().
		WithMachine(ns1, "", "", "machine1")

	return b.obj
}

type fakeclusterBuilder struct {
	obj []runtime.Object
}

const (
	InfrastructureAPIVersion    = "infrastructure.cluster.x-k8s.io/v1alpha3"
	KindProviderMachineTemplate = "ProviderMachineTemplate"
	KindProviderMachine         = "ProviderMachine"
	KindProviderCluster         = "ProviderCluster"
)

func (b *fakeclusterBuilder) WithCluster(ns, name string) *fakeclusterBuilder {
	b.obj = append(b.obj, &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: InfrastructureAPIVersion,
				Kind:       KindProviderCluster,
				Name:       name,
				Namespace:  ns,
			},
		},
	})

	b.obj = append(b.obj, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderCluster,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})

	b.obj = append(b.obj, &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-kubeconfig",
			Namespace: ns,
		},
	})

	return b
}

func (b *fakeclusterBuilder) WithMachineDeployment(ns, cluster, name string) *fakeclusterBuilder {
	md := clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: InfrastructureAPIVersion,
						Kind:       KindProviderMachineTemplate,
						Name:       name,
						Namespace:  ns,
					},
				},
			},
		},
	}

	if cluster != "" {
		md.Labels = map[string]string{clusterv13.ClusterLabelName: cluster}
		blockOwnerDeletion := true
		md.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}

	b.obj = append(b.obj, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderMachineTemplate,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})
	b.obj = append(b.obj, &md)
	return b
}

func (b *fakeclusterBuilder) WithMachineSet(ns, cluster, md, name string) *fakeclusterBuilder {
	ms := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineSet",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	if cluster != "" {
		ms.Labels = map[string]string{clusterv13.ClusterLabelName: cluster}
		blockOwnerDeletion := true
		ms.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}

	if md != "" {
		isController := true
		ms.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.Version,
				Kind:       "MachineDeployment",
				Name:       md,
				Controller: &isController,
			},
		}
	} else {
		ms.Spec.Template.Spec.InfrastructureRef = corev1.ObjectReference{
			APIVersion: InfrastructureAPIVersion,
			Kind:       KindProviderMachineTemplate,
			Name:       name,
			Namespace:  ns,
		}
		b.obj = append(b.obj, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": InfrastructureAPIVersion,
				"kind":       KindProviderMachineTemplate,
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": ns,
				},
			},
		})
	}
	b.obj = append(b.obj, &ms)
	return b
}

func (b *fakeclusterBuilder) WithMachine(ns, cluster, ms, name string) *fakeclusterBuilder {
	m := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: InfrastructureAPIVersion,
				Kind:       KindProviderMachine,
				Name:       name,
				Namespace:  ns,
			},
		},
	}

	if cluster != "" {
		m.Labels = map[string]string{clusterv13.ClusterLabelName: cluster}
		blockOwnerDeletion := true
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         clusterv1.GroupVersion.Version,
				Kind:               "Cluster",
				Name:               cluster,
				BlockOwnerDeletion: &blockOwnerDeletion,
			},
		}
	}
	if ms != "" {
		isController := true
		m.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: clusterv1.GroupVersion.Version,
				Kind:       "MachineSet",
				Name:       ms,
				Controller: &isController,
			},
		}
	}

	b.obj = append(b.obj, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": InfrastructureAPIVersion,
			"kind":       KindProviderMachine,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	})
	b.obj = append(b.obj, &m)
	return b
}

func newFakeclusterBuilder() *fakeclusterBuilder {
	return &fakeclusterBuilder{}
}
