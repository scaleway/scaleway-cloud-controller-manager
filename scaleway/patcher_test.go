/*
Copyright 2018 Scaleway

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

package scaleway

import (
	"context"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeService() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "service-name",
			Namespace:   metav1.NamespaceDefault,
			Annotations: map[string]string{},
		},
	}
}

func newFakeNode() *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "node-name",
			Annotations: map[string]string{},
		},
	}
}

func TestNewNodePatcher(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	node := newFakeNode()
	patcher := NewNodePatcher(clientset, node)

	if !reflect.DeepEqual(patcher.origin, patcher.final) {
		t.Errorf("NewServicePatcher() origin and final are not equal")
	}

	node.ObjectMeta.Annotations["test-annotation"] = "works"

	if reflect.DeepEqual(patcher.origin, patcher.final) {
		t.Errorf("NewServicePatcher() origin and final are still equal")
	}
}

func TestNewServicePatcher(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	service := newFakeService()
	patcher := NewServicePatcher(clientset, service)

	if !reflect.DeepEqual(patcher.origin, patcher.final) {
		t.Errorf("NewServicePatcher() origin and final are not equal")
	}

	service.ObjectMeta.Annotations["test-annotation"] = "works"

	if reflect.DeepEqual(patcher.origin, patcher.final) {
		t.Errorf("NewServicePatcher() origin and final are still equal")
	}
}

func TestKubePatcher_Patch(t *testing.T) {
	t.Run("Service", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		service := newFakeService()
		serviceAdded, _ := clientset.CoreV1().Services(metav1.NamespaceDefault).Create(context.Background(), service, metav1.CreateOptions{})
		if val, ok := serviceAdded.ObjectMeta.Annotations["test-annotation"]; ok || val == "works" {
			t.Errorf("Patch() bad test, service is already annotated")
		}

		patcher := NewServicePatcher(clientset, service)
		service.ObjectMeta.Annotations["test-annotation"] = "works"
		err := patcher.Patch()
		if err != nil {
			t.Errorf("Patch() got error while patching service")
		}

		serviceFinal, _ := clientset.CoreV1().Services(metav1.NamespaceDefault).Get(context.Background(), service.Name, metav1.GetOptions{})
		if val, ok := serviceFinal.ObjectMeta.Annotations["test-annotation"]; !ok || val != "works" {
			t.Errorf("Patch() service do not have annotation")
		}
	})

	t.Run("Node", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		node := newFakeNode()
		nodeAdded, _ := clientset.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		if val, ok := nodeAdded.ObjectMeta.Annotations["test-annotation"]; ok || val == "works" {
			t.Errorf("Patch() bad test, node is already annotated")
		}

		patcher := NewNodePatcher(clientset, node)
		node.ObjectMeta.Annotations["test-annotation"] = "works"
		err := patcher.Patch()
		if err != nil {
			t.Errorf("Patch() got error while patching node")
		}

		nodeFinal, _ := clientset.CoreV1().Nodes().Get(context.Background(), node.Name, metav1.GetOptions{})
		if val, ok := nodeFinal.ObjectMeta.Annotations["test-annotation"]; !ok || val != "works" {
			t.Errorf("Patch() node do not have annotation")
		}
	})
}
