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
	"github.com/scaleway/scaleway-sdk-go/scw"
	clientset "k8s.io/client-go/kubernetes"
)

type client struct {
	scaleway   *scw.Client
	kubernetes clientset.Interface
}

func newClient(scaleway *scw.Client) *client {
	return &client{
		scaleway: scaleway,
	}
}
