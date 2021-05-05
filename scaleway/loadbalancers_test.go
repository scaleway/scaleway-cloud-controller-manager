/*
Copyright 2021 Scaleway

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
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestGetValueForPort(t *testing.T) {
	testCase := []struct {
		name       string
		svc        *v1.Service
		nodePort   int32
		fullValue  string
		result     string
		errMessage string
	}{
		{
			name: "simple",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "foo",
			errMessage: "",
			result:     "foo",
		},
		{
			name: "split-1",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "33:foo;34:bar",
			errMessage: "",
			result:     "foo",
		},
		{
			name: "split-2",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33334,
			fullValue:  "33:foo;34:bar",
			errMessage: "",
			result:     "bar",
		},
		{
			name: "default-1",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "foo;34:bar",
			errMessage: "",
			result:     "foo",
		},
		{
			name: "default-2",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "34:bar;foo",
			errMessage: "",
			result:     "foo",
		},
		{
			name: "default-3",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33334,
			fullValue:  "34:bar;foo",
			errMessage: "",
			result:     "bar",
		},
		{
			name: "default-3",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33334,
			fullValue:  "foo;34:bar",
			errMessage: "",
			result:     "bar",
		},
		{
			name: "complex-1",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33334,
			fullValue:  "22:ssh;foo;34:bar;test",
			errMessage: "",
			result:     "bar",
		},
		{
			name: "multiple-1",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "34,35,35,33:bar;test",
			errMessage: "",
			result:     "bar",
		},

		// errors
		{
			name: "error-1",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  ":;34:::4;;:::44::",
			errMessage: "annotation with value :;34:::4;;:::44:: is wrongly formatted, should be `port1:value1;port2,port3:value2`",
			result:     "",
		},
		{
			name: "error-2",
			svc: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							NodePort: 33333,
							Port:     33,
						},
						{
							NodePort: 33334,
							Port:     34,
						},
					},
				},
			},
			nodePort:   33333,
			fullValue:  "foo:bar",
			errMessage: `strconv.ParseInt: parsing "foo": invalid syntax`,
			result:     "",
		},
	}

	for i := range testCase {
		t.Run(testCase[i].name, func(t *testing.T) {
			result, err := getValueForPort(testCase[i].svc, testCase[i].nodePort, testCase[i].fullValue)
			if result != testCase[i].result || err == nil && testCase[i].errMessage != "" || err != nil && testCase[i].errMessage == "" || err != nil && testCase[i].errMessage != err.Error() {
				t.Errorf("%s", err.Error())
				t.Errorf("getValueForPort: got %s, %v expected %s, %s", result, err, testCase[i].result, testCase[i].errMessage)
			}
		})
	}
}
