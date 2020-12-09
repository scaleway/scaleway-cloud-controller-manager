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
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type kubePatcher struct {
	kclient kubernetes.Interface
	origin  interface{}
	final   interface{}
}

func NewServicePatcher(kclient kubernetes.Interface, service *v1.Service) kubePatcher {
	return kubePatcher{
		kclient: kclient,
		origin:  service.DeepCopy(),
		final:   service,
	}
}

func NewNodePatcher(kclient kubernetes.Interface, node *v1.Node) kubePatcher {
	return kubePatcher{
		kclient: kclient,
		origin:  node.DeepCopy(),
		final:   node,
	}
}

func (kp *kubePatcher) Patch() error {
	if kp.kclient == nil {
		klog.Infof("clientset is nil!")
	}

	originJSON, err := json.Marshal(kp.origin)
	if err != nil {
		return fmt.Errorf("failed to serialize current original object: %s", err)
	}

	updatedJSON, err := json.Marshal(kp.final)
	if err != nil {
		return fmt.Errorf("failed to serialize modified updated object: %s", err)
	}

	switch typedUpdated := kp.origin.(type) {
	case *v1.Node:
		patch, err := strategicpatch.CreateTwoWayMergePatch(originJSON, updatedJSON, v1.Node{})
		if err != nil {
			return fmt.Errorf("failed to create 2-way merge patch: %s", err)
		}

		if len(patch) == 0 || string(patch) == "{}" {
			return nil
		}

		_, err = kp.kclient.CoreV1().Nodes().Patch(context.Background(), typedUpdated.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("failed to patch node object %s: %s", typedUpdated.Name, err)
		}
	case *v1.Service:
		patch, err := strategicpatch.CreateTwoWayMergePatch(originJSON, updatedJSON, v1.Service{})
		if err != nil {
			return fmt.Errorf("failed to create 2-way merge patch: %s", err)
		}

		if len(patch) == 0 || string(patch) == "{}" {
			return nil
		}

		_, err = kp.kclient.CoreV1().Services(typedUpdated.Namespace).Patch(context.Background(), typedUpdated.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("failed to patch service object %s/%s: %s", typedUpdated.Namespace, typedUpdated.Name, err)
		}
	default:
		panic("unsupported type!")
	}

	return nil
}
