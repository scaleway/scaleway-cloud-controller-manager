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

import "errors"

var (
	BadProviderID          = errors.New("provider ID wrong format: format should be scaleway://product/region/0788e6f4-55b0-42e2-936f-d0c5ecd49a13")
	InstanceDuplicated     = errors.New("duplicated instance results")
	IPAddressNotFound      = errors.New("ip address not found")
	IPAddressInUse         = errors.New("ip address already in use")
	LoadBalancerNotFound   = errors.New("loadbalancer not found")
	LoadBalancerDuplicated = errors.New("loadbalancer duplicated")
	LoadBalancerNotReady   = errors.New("loadbalancer is not ready")

	errLoadBalancerInvalidAnnotation     = errors.New("load balancer invalid annotation")
	errLoadBalancerInvalidLoadBalancerID = errors.New("load balancer invalid loadbalancer-id annotation")
)

type AnnotationError struct {
	Key   string
	Value string
}

func NewAnnorationError(key, value string) AnnotationError {
	return AnnotationError{
		Key:   key,
		Value: value,
	}
}

func (a AnnotationError) Error() string {
	return "annotation '" + a.Key + "' is missing or has invalid value '" + a.Value
}
