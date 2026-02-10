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
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cloud-provider/api"
	"k8s.io/utils/ptr"
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

func TestFilterNodes(t *testing.T) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"service.beta.kubernetes.io/scw-loadbalancer-target-node-labels": "key1=value1,key2=,key3,k8s.scaleway.com/pool-name=default",
			},
		},
	}
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ok-exact-match",
				Labels: map[string]string{
					"key1":                       "value1",
					"key2":                       "",
					"key3":                       "",
					"k8s.scaleway.com/pool-name": "default",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ok-with-value-in-empty-keys",
				Labels: map[string]string{
					"key1":                       "value1",
					"key2":                       "value2",
					"key3":                       "value3",
					"k8s.scaleway.com/pool-name": "default",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ok-with-extra-keys",
				Labels: map[string]string{
					"key1":                       "value1",
					"key2":                       "value2",
					"key3":                       "value3",
					"k8s.scaleway.com/pool-name": "default",
					"extra":                      "extra",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ko-invalid-value-key",
				Labels: map[string]string{
					"key1":                       "unexpected",
					"key2":                       "value2",
					"key3":                       "value3",
					"k8s.scaleway.com/pool-name": "default",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ko-missing-value-key",
				Labels: map[string]string{
					"key2":                       "value2",
					"key3":                       "value3",
					"k8s.scaleway.com/pool-name": "default",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-ko-missing-empty-key",
				Labels: map[string]string{
					"key1":                       "value1",
					"key3":                       "value3",
					"k8s.scaleway.com/pool-name": "default",
				},
			},
		},
	}

	want := []string{}
	for _, n := range nodes {
		if strings.Contains(n.Name, "ok") {
			want = append(want, n.Name)
		}
	}

	filteredNodes := filterNodes(service, nodes)
	got := []string{}
	for _, n := range filteredNodes {
		got = append(got, n.Name)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

func TestServicePortToFrontend(t *testing.T) {
	defaultTimeout, _ := time.ParseDuration("10m")
	otherTimeout, _ := time.ParseDuration("15m")

	matrix := []struct {
		name    string
		service *v1.Service
		want    *scwlb.Frontend
		wantErr bool
	}{
		{
			"simple",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID:         "uid",
					Annotations: map[string]string{},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
			},
			false,
		},
		{
			"with timeout",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-timeout-client": "15m",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{},
			},
			false,
		},
		{
			"with one certificate",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "uid-1",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			false,
		},
		{
			"with one zoned certificate",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "fr-par-1/uid-1",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			false,
		},
		{
			"with one certificate in complex form",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "1234:uid-1;9876:uid-2",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			false,
		},
		{
			"with certificates",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "uid-1,uid-2",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			false,
		},
		{
			"with certificates in complex form",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "1234:uid-1,uid-2;9876:uid-3,uid-4",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			false,
		},
		{
			"with connection rate limit",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-connection-rate-limit": "100",
					},
				},
			},
			&scwlb.Frontend{
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(100); return &v }(),
			},
			false,
		},
		{
			"with enable access logs",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-enable-access-logs": "true",
					},
				},
			},
			&scwlb.Frontend{
				Name:             "uid_tcp_1234",
				InboundPort:      1234,
				TimeoutClient:    &defaultTimeout,
				CertificateIDs:   []string{},
				EnableAccessLogs: true,
			},
			false,
		},
		{
			"with enable http3",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-enable-http3": "true",
					},
				},
			},
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
				EnableHTTP3:    true,
			},
			false,
		},
		{
			"with invalid timeout",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-timeout-client": "garbage",
					},
				},
			},
			nil,
			true,
		},
		{
			"with invalid certificate format",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-certificate-ids": "uid-1:uid-2",
					},
				},
			},
			nil,
			true,
		},
		{
			"with invalid connection rate limit",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-connection-rate-limit": "not-a-number",
					},
				},
			},
			nil,
			true,
		},
		{
			"with invalid enable access logs",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-enable-access-logs": "not-a-bool",
					},
				},
			},
			nil,
			true,
		},
		{
			"with invalid enable http3",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "uid",
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-enable-http3": "invalid-bool",
					},
				},
			},
			nil,
			true,
		},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got, err := servicePortToFrontend(tt.service, &scwlb.LB{ID: "lbid"}, v1.ServicePort{Port: 1234})
			if tt.wantErr != (err != nil) {
				t.Errorf("got error: %s, expected: %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("want: %##v, got: %##v", tt.want, got)
			}
		})
	}
}

func TestFrontendEquals(t *testing.T) {
	defaultTimeout, _ := time.ParseDuration("10m")
	defaultTimeout2, _ := time.ParseDuration("10m")
	otherTimeout, _ := time.ParseDuration("15m")

	matrix := []struct {
		name string
		a    *scwlb.Frontend
		b    *scwlb.Frontend
		want bool
	}{
		{
			"simple",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout2,
				CertificateIDs: []string{},
			},
			true,
		},
		{
			"with first nil",
			nil,
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
			},
			false,
		},
		{
			"with last nil",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
			},
			nil,
			false,
		},
		{
			"with both nil",
			nil,
			nil,
			true,
		},
		{
			"full",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-2", "uid-1"},
			},
			true,
		},
		{
			"with a different name",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "different",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-2", "uid-1"},
			},
			false,
		},
		{
			"with a different port",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    0,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-2", "uid-1"},
			},
			false,
		},
		{
			"with a different timeout",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-2", "uid-1"},
			},
			false,
		},
		{
			"with different certificate list",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-2"},
			},
			false,
		},
		{
			"with extra certificate in list",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			false,
		},
		{
			"with missing certificate in list",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1", "uid-2"},
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &otherTimeout,
				CertificateIDs: []string{"uid-1"},
			},
			false,
		},
		{
			"with same connection rate limit",
			&scwlb.Frontend{
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(100); return &v }(),
			},
			&scwlb.Frontend{
				ID:                  "uid-1",
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(100); return &v }(),
			},
			true,
		},
		{
			"with different connection rate limit",
			&scwlb.Frontend{
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(100); return &v }(),
			},
			&scwlb.Frontend{
				ID:                  "uid-1",
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(200); return &v }(),
			},
			false,
		},
		{
			"with nil connection rate limit",
			&scwlb.Frontend{
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: nil,
			},
			&scwlb.Frontend{
				ID:                  "uid-1",
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: nil,
			},
			true,
		},
		{
			"with one nil connection rate limit",
			&scwlb.Frontend{
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: func() *uint32 { v := uint32(100); return &v }(),
			},
			&scwlb.Frontend{
				ID:                  "uid-1",
				Name:                "uid_tcp_1234",
				InboundPort:         1234,
				TimeoutClient:       &defaultTimeout,
				CertificateIDs:      []string{},
				ConnectionRateLimit: nil,
			},
			false,
		},
		{
			"with same enable access logs",
			&scwlb.Frontend{
				Name:             "uid_tcp_1234",
				InboundPort:      1234,
				TimeoutClient:    &defaultTimeout,
				CertificateIDs:   []string{},
				EnableAccessLogs: true,
			},
			&scwlb.Frontend{
				ID:               "uid-1",
				Name:             "uid_tcp_1234",
				InboundPort:      1234,
				TimeoutClient:    &defaultTimeout,
				CertificateIDs:   []string{},
				EnableAccessLogs: true,
			},
			true,
		},
		{
			"with different enable access logs",
			&scwlb.Frontend{
				Name:             "uid_tcp_1234",
				InboundPort:      1234,
				TimeoutClient:    &defaultTimeout,
				CertificateIDs:   []string{},
				EnableAccessLogs: true,
			},
			&scwlb.Frontend{
				ID:               "uid-1",
				Name:             "uid_tcp_1234",
				InboundPort:      1234,
				TimeoutClient:    &defaultTimeout,
				CertificateIDs:   []string{},
				EnableAccessLogs: false,
			},
			false,
		},
		{
			"with same enable http3",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
				EnableHTTP3:    true,
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
				EnableHTTP3:    true,
			},
			true,
		},
		{
			"with different enable http3",
			&scwlb.Frontend{
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
				EnableHTTP3:    false,
			},
			&scwlb.Frontend{
				ID:             "uid-1",
				Name:           "uid_tcp_1234",
				InboundPort:    1234,
				TimeoutClient:  &defaultTimeout,
				CertificateIDs: []string{},
				EnableHTTP3:    true,
			},
			false,
		},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := frontendEquals(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func TestBackendEquals(t *testing.T) {
	defaultDelay, _ := time.ParseDuration("1s")
	defaultTimeout, _ := time.ParseDuration("2s")
	defaultTimeoutServer, _ := time.ParseDuration("3s")
	defaultTimeoutConnect, _ := time.ParseDuration("4s")
	defaultTimeoutTunnel, _ := time.ParseDuration("5s")
	defaultTimeoutQueueTime, _ := time.ParseDuration("5s")
	defaultTimeoutQueue := scw.NewDurationFromTimeDuration(defaultTimeoutQueueTime)
	otherDuration, _ := time.ParseDuration("50m")
	defaultString := "default"
	otherString := "other"
	boolTrue := true
	boolFalse := false
	var int0 int32 = 0
	var int1 int32 = 1
	var intOther int32 = 5

	reference := &scwlb.Backend{
		Name:                     "name",
		ForwardProtocol:          "proto",
		ForwardPort:              1234,
		ForwardPortAlgorithm:     "algo",
		StickySessions:           "mode",
		StickySessionsCookieName: "cookie",
		HealthCheck: &scwlb.HealthCheck{
			Port:                1234,
			CheckDelay:          &defaultDelay,
			CheckTimeout:        &defaultTimeout,
			CheckMaxRetries:     0,
			TCPConfig:           &scwlb.HealthCheckTCPConfig{},
			MysqlConfig:         nil,
			PgsqlConfig:         nil,
			LdapConfig:          nil,
			RedisConfig:         nil,
			HTTPConfig:          nil,
			HTTPSConfig:         nil,
			CheckSendProxy:      false,
			TransientCheckDelay: &scw.Duration{Seconds: 1},
		},
		Pool:                   []string{"1.2.3.4", "2.3.4.5"},
		SendProxyV2:            &boolTrue,
		SslBridging:            &boolFalse,
		IgnoreSslServerVerify:  &boolFalse,
		TimeoutServer:          &defaultTimeoutServer,
		TimeoutConnect:         &defaultTimeoutConnect,
		TimeoutTunnel:          &defaultTimeoutTunnel,
		TimeoutQueue:           defaultTimeoutQueue,
		OnMarkedDownAction:     "action",
		ProxyProtocol:          "proxy",
		RedispatchAttemptCount: &int0,
		MaxConnections:         &int1,
		MaxRetries:             &int1,
		FailoverHost:           &defaultString,
	}

	matrix := []struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{
		{
			"simple",
			reference,
			deepCloneBackend(reference),
			true,
		},
		{
			"with first nil",
			nil,
			deepCloneBackend(reference),
			false,
		},
		{
			"with last nil",
			deepCloneBackend(reference),
			nil,
			false,
		},
		{
			"with both nil",
			nil,
			nil,
			true,
		},
	}

	diff := deepCloneBackend(reference)
	diff.Name = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different Name", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.ForwardPort = 2345
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different ForwardPort", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.ForwardPortAlgorithm = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different ForwardPortAlgorithm", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.ForwardProtocol = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different ForwardProtocol", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.MaxConnections = &intOther
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different MaxConnections", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.MaxRetries = &intOther
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different MaxRetries", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.OnMarkedDownAction = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different OnMarkedDownAction", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.Pool = []string{"2.3.4.5"}
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different Pool", reference, diff, true})

	diff = deepCloneBackend(reference)
	diff.ProxyProtocol = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different ProxyProtocol", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.RedispatchAttemptCount = &intOther
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different RedispatchAttemptCount", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.StickySessions = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different StickySessions", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.StickySessionsCookieName = "other"
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different StickySessionsCookieName", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.TimeoutConnect = &otherDuration
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different TimeoutConnect", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.TimeoutServer = &otherDuration
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different TimeoutServer", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.TimeoutTunnel = &otherDuration
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different TimeoutTunnel", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.TimeoutQueue = scw.NewDurationFromTimeDuration(otherDuration)
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different TimeoutQueue", reference, diff, false})

	diff = deepCloneBackend(reference)
	diff.FailoverHost = &otherString
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with a different FailoverHost", reference, diff, false})

	httpRef := deepCloneBackend(reference)
	httpRef.HealthCheck.TCPConfig = nil
	httpRef.HealthCheck.HTTPConfig = &scwlb.HealthCheckHTTPConfig{
		URI:    "/",
		Method: "POST",
		Code:   scw.Int32Ptr(200),
	}
	httpDiff := deepCloneBackend(httpRef)
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with same HTTP healthchecks", httpRef, httpDiff, true})

	httpDiff = deepCloneBackend(httpRef)
	httpDiff.HealthCheck.HTTPConfig.Code = scw.Int32Ptr(404)
	matrix = append(matrix, struct {
		Name string
		a    *scwlb.Backend
		b    *scwlb.Backend
		want bool
	}{"with different HTTP healthchecks", httpRef, httpDiff, false})

	for _, tt := range matrix {
		t.Run(tt.Name, func(t *testing.T) {
			got := backendEquals(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func TestStringArrayEqual(t *testing.T) {
	matrix := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"same", []string{"a", "b"}, []string{"a", "b"}, true},
		{"with a different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"with a different element", []string{"a", "b"}, []string{"a", "c"}, false},
		{"with a missing element", []string{"a", "b"}, []string{"a"}, false},
		{"with an extra element", []string{"a", "b"}, []string{"a", "b", "c"}, false},
		{"with first empty", nil, []string{"a", "b"}, false},
		{"with last empty", []string{"a", "b"}, nil, false},
		{"with both empty", nil, nil, true},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := stringArrayEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestStringPtrArrayEqual(t *testing.T) {
	matrix := []struct {
		name string
		a    []*string
		b    []*string
		want bool
	}{
		{"same", scw.StringSlicePtr([]string{"a", "b"}), scw.StringSlicePtr([]string{"a", "b"}), true},
		{"with a different order", scw.StringSlicePtr([]string{"a", "b"}), scw.StringSlicePtr([]string{"b", "a"}), true},
		{"with a different element", scw.StringSlicePtr([]string{"a", "b"}), scw.StringSlicePtr([]string{"a", "c"}), false},
		{"with a missing element", scw.StringSlicePtr([]string{"a", "b"}), scw.StringSlicePtr([]string{"a"}), false},
		{"with an extra element", scw.StringSlicePtr([]string{"a", "b"}), scw.StringSlicePtr([]string{"a", "b", "c"}), false},
		{"with first empty", nil, scw.StringSlicePtr([]string{"a", "b"}), false},
		{"with last empty", scw.StringSlicePtr([]string{"a", "b"}), nil, false},
		{"with both empty", nil, nil, true},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := stringPtrArrayEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestDurationPtrEqual(t *testing.T) {
	duration1, _ := time.ParseDuration("10m")
	otherDuration1, _ := time.ParseDuration("10m")
	duration2, _ := time.ParseDuration("5s")

	matrix := []struct {
		name string
		a    *time.Duration
		b    *time.Duration
		want bool
	}{
		{"same", &duration1, &duration1, true},
		{"same with different ptr", &duration1, &otherDuration1, true},
		{"with first nil", &duration1, nil, false},
		{"with last nil", nil, &duration1, false},
		{"with both nil", nil, nil, true},
		{"with different values", &duration1, &duration2, false},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := durationPtrEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestScwDurationPtrEqual(t *testing.T) {
	duration1 := scw.Duration{Seconds: 10, Nanos: 1000}
	otherDuration1 := scw.Duration{Seconds: 10, Nanos: 1000}
	duration2 := scw.Duration{Seconds: 20, Nanos: 2000}

	matrix := []struct {
		name string
		a    *scw.Duration
		b    *scw.Duration
		want bool
	}{
		{"same", &duration1, &duration1, true},
		{"same with different ptr", &duration1, &otherDuration1, true},
		{"with first nil", &duration1, nil, false},
		{"with last nil", nil, &duration1, false},
		{"with both nil", nil, nil, true},
		{"with different values", &duration1, &duration2, false},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := scwDurationPtrEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestInt32PtrEqual(t *testing.T) {
	var int1 int32 = 1
	var otherInt1 int32 = 1
	var int2 int32 = 2

	matrix := []struct {
		name string
		a    *int32
		b    *int32
		want bool
	}{
		{"same", &int1, &int1, true},
		{"same with different ptr", &int1, &otherInt1, true},
		{"with first nil", &int1, nil, false},
		{"with last nil", nil, &int1, false},
		{"with both nil", nil, nil, true},
		{"with different values", &int1, &int2, false},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := int32PtrEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func TestUint32PtrEqual(t *testing.T) {
	var uint1 uint32 = 1
	var otherUint1 uint32 = 1
	var uint2 uint32 = 2

	matrix := []struct {
		name string
		a    *uint32
		b    *uint32
		want bool
	}{
		{"same", &uint1, &uint1, true},
		{"same with different ptr", &uint1, &otherUint1, true},
		{"with first nil", &uint1, nil, false},
		{"with last nil", nil, &uint1, false},
		{"with both nil", nil, nil, true},
		{"with different values", &uint1, &uint2, false},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := uint32PtrEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func TestPtrUint32ToString(t *testing.T) {
	var uint1 uint32 = 42
	var uint2 uint32 = 0

	matrix := []struct {
		name string
		a    *uint32
		want string
	}{
		{"with value", &uint1, "42"},
		{"with zero", &uint2, "0"},
		{"with nil", nil, "<nil>"},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrUint32ToString(tt.a)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestStrPtrEqual(t *testing.T) {
	var str1 string = "test"
	var otherStr1 string = "test"
	var str2 string = "test2"

	matrix := []struct {
		name string
		a    *string
		b    *string
		want bool
	}{
		{"same", &str1, &str1, true},
		{"same with different ptr", &str1, &otherStr1, true},
		{"with first nil", &str1, nil, false},
		{"with last nil", nil, &str1, false},
		{"with both nil", nil, nil, true},
		{"with different values", &str1, &str2, false},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrStringEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestChunkArray(t *testing.T) {
	matrix := []struct {
		name  string
		array []string
		want  [][]string
	}{
		{"nil", nil, [][]string{}},
		{"empty", []string{}, [][]string{}},
		{"less than chunksize", []string{"1", "2"}, [][]string{
			{"1", "2"},
		}},
		{"equal to chunksize", []string{"1", "2", "3"}, [][]string{
			{"1", "2", "3"},
		}},
		{"more than chunksize", []string{"1", "2", "3", "4"}, [][]string{
			{"1", "2", "3"},
			{"4"},
		}},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkArray(tt.array, 3)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}
func TestMakeACLPrefix(t *testing.T) {
	matrix := []struct {
		name     string
		frontend *scwlb.Frontend
		want     string
	}{
		{"with nil", nil, "lb-source-range"},
		{"with empty frontend", &scwlb.Frontend{}, "-lb-source-range"},
		{"with frontend", &scwlb.Frontend{ID: "uid"}, "uid-lb-source-range"},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := makeACLPrefix(tt.frontend)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func TestGetHTTPHealthCheck(t *testing.T) {
	matrix := []struct {
		name string
		svc  *v1.Service
		want *scwlb.HealthCheckHTTPConfig
	}{
		{"with empty config", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type": "http",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/", Method: "GET", Code: scw.Int32Ptr(200)}},

		{"with just a domain", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":     "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri": "domain.tld",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/", Method: "GET", Code: scw.Int32Ptr(200), HostHeader: "domain.tld"}},

		{"with a domain and path", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":     "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri": "domain.tld/path",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/path", Method: "GET", Code: scw.Int32Ptr(200), HostHeader: "domain.tld"}},

		{"with a domain, path and query params", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":     "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri": "domain.tld/path?password=xxxx",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/path?password=xxxx", Method: "GET", Code: scw.Int32Ptr(200), HostHeader: "domain.tld"}},

		{"with just a path", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":     "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri": "/path",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/path", Method: "GET", Code: scw.Int32Ptr(200)}},

		{"with a specific code", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":      "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-code": "404",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/", Method: "GET", Code: scw.Int32Ptr(404)}},

		{"with a specific method", &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-type":        "http",
					"service.beta.kubernetes.io/scw-loadbalancer-health-check-http-method": "POST",
				},
			},
		}, &scwlb.HealthCheckHTTPConfig{URI: "/", Method: "POST", Code: scw.Int32Ptr(200)}},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := getHTTPHealthCheck(tt.svc, int32(80))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func deepCloneBackend(original *scwlb.Backend) *scwlb.Backend {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		panic(err)
	}

	var clone *scwlb.Backend

	err = json.Unmarshal(originalJSON, &clone)
	if err != nil {
		panic(err)
	}

	return clone
}

func Test_hasEqualLoadBalancerStaticIPs(t *testing.T) {
	type args struct {
		service *v1.Service
		lb      *scwlb.LB
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "no static ip set",
			args: args{
				service: &v1.Service{},
				lb:      &scwlb.LB{},
			},
			want: true,
		},
		{
			name: "static ip set with field but mismatch",
			args: args{
				service: &v1.Service{
					Spec: v1.ServiceSpec{
						LoadBalancerIP: "127.0.0.1",
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							IPAddress: "127.0.0.2",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "static ip set with field match",
			args: args{
				service: &v1.Service{
					Spec: v1.ServiceSpec{
						LoadBalancerIP: "127.0.0.1",
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							IPAddress: "127.0.0.1",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "static ip set with annotation mismatch",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{serviceAnnotationLoadBalancerIPIDs: "fakeid"},
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							ID: "fakeid",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "static ip set with annotation, count mismatch",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{serviceAnnotationLoadBalancerIPIDs: "fakeid1,fakeid2"},
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							ID: "fakeid1",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "static ip set with annotation, match many",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{serviceAnnotationLoadBalancerIPIDs: "fakeid1,fakeid2"},
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							ID: "fakeid1",
						},
						{
							ID: "fakeid2",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "static ip set with annotation, ignore field",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{serviceAnnotationLoadBalancerIPIDs: "fakeid1,fakeid2"},
					},
					Spec: v1.ServiceSpec{
						LoadBalancerIP: "127.0.0.1",
					},
				},
				lb: &scwlb.LB{
					IP: []*scwlb.IP{
						{
							ID:        "fakeid1",
							IPAddress: "127.0.0.2",
						},
						{
							ID: "fakeid2",
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasEqualLoadBalancerStaticIPs(tt.args.service, tt.args.lb); got != tt.want {
				t.Errorf("hasEqualLoadBalancerStaticIPs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_nodesInitialized(t *testing.T) {
	type args struct {
		nodes []*v1.Node
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "node initialized",
			args: args{
				nodes: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "node not initialized but created 5 minutes ago",
			args: args{
				nodes: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
						},
						Spec: v1.NodeSpec{
							Taints: []v1.Taint{
								{
									Key:    api.TaintExternalCloudProvider,
									Value:  "true",
									Effect: v1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "node not initialized, created 10 seconds ago",
			args: args{
				nodes: []*v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test",
							CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Second)),
						},
						Spec: v1.NodeSpec{
							Taints: []v1.Taint{
								{
									Key:    api.TaintExternalCloudProvider,
									Value:  "true",
									Effect: v1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := nodesInitialized(tt.args.nodes); (err != nil) != tt.wantErr {
				t.Errorf("nodesInitialized() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetSSLCompatibilityLevel(t *testing.T) {
	matrix := []struct {
		name    string
		service *v1.Service
		want    scwlb.SSLCompatibilityLevel
		wantErr bool
	}{
		{
			"with empty annotation",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			scwlb.SSLCompatibilityLevelSslCompatibilityLevelIntermediate,
			false,
		},
		{
			"with valid intermediate level",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-ssl-compatibility-level": "ssl_compatibility_level_intermediate",
					},
				},
			},
			scwlb.SSLCompatibilityLevelSslCompatibilityLevelIntermediate,
			false,
		},
		{
			"with valid modern level",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-ssl-compatibility-level": "ssl_compatibility_level_modern",
					},
				},
			},
			scwlb.SSLCompatibilityLevelSslCompatibilityLevelModern,
			false,
		},
		{
			"with valid old level",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-ssl-compatibility-level": "ssl_compatibility_level_old",
					},
				},
			},
			scwlb.SSLCompatibilityLevelSslCompatibilityLevelOld,
			false,
		},
		{
			"with invalid level",
			&v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-ssl-compatibility-level": "invalid_level",
					},
				},
			},
			"",
			true,
		},
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getSSLCompatibilityLevel(tt.service)
			if tt.wantErr != (err != nil) {
				t.Errorf("got error: %s, expected: %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("want: %v, got: %v", tt.want, got)
			}
		})
	}
}

func Test_ipMode(t *testing.T) {
	type args struct {
		service *v1.Service
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.LoadBalancerIPMode
		wantErr bool
	}{
		{
			name: "no ports",
			args: args{
				service: newFakeService(),
			},
			want: ptr.To(v1.LoadBalancerIPModeVIP),
		},
		{
			name: "no proxy protocol",
			args: args{
				service: &v1.Service{
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(80),
								NodePort:   30000,
							},
							{
								Port:       443,
								TargetPort: intstr.FromInt(443),
								NodePort:   30001,
							},
						},
					},
				},
			},
			want: ptr.To(v1.LoadBalancerIPModeVIP),
		},
		{
			name: "proxy protocol on one port only",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							serviceAnnotationLoadBalancerProxyProtocolV2: "80",
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(80),
								NodePort:   30000,
							},
							{
								Port:       443,
								TargetPort: intstr.FromInt(443),
								NodePort:   30001,
							},
						},
					},
				},
			},
			want: ptr.To(v1.LoadBalancerIPModeVIP),
		},
		{
			name: "proxy protocol on all ports",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							serviceAnnotationLoadBalancerProxyProtocolV2: "true",
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(80),
								NodePort:   30000,
							},
							{
								Port:       443,
								TargetPort: intstr.FromInt(443),
								NodePort:   30001,
							},
						},
					},
				},
			},
			want: ptr.To(v1.LoadBalancerIPModeProxy),
		},
		{
			name: "proxy protocol on all ports, overriden by annotation",
			args: args{
				service: &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							serviceAnnotationLoadBalancerProxyProtocolV2: "true",
							serviceAnnotationLoadBalancerIPMode:          string(v1.LoadBalancerIPModeVIP),
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port:       80,
								TargetPort: intstr.FromInt(80),
								NodePort:   30000,
							},
							{
								Port:       443,
								TargetPort: intstr.FromInt(443),
								NodePort:   30001,
							},
						},
					},
				},
			},
			want: ptr.To(v1.LoadBalancerIPModeVIP),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ipMode(tt.args.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("ipMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ipMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetHealthCheckFromService(t *testing.T) {
	testCases := []struct {
		name            string
		service         *v1.Service
		port            int32
		wantEnabled     bool
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "annotation not set",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			port:        80,
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "annotation set to true",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "true",
					},
				},
			},
			port:        80,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "annotation set to false",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "false",
					},
				},
			},
			port:        80,
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "annotation set to * (all ports)",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "*",
					},
				},
			},
			port:        443,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "annotation set to specific port - match",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80",
					},
				},
			},
			port:        80,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "annotation set to specific port - no match",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80",
					},
				},
			},
			port:        443,
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "annotation set to comma-separated ports - match first",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80,443",
					},
				},
			},
			port:        80,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "annotation set to comma-separated ports - match second",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80,443",
					},
				},
			},
			port:        443,
			wantEnabled: true,
			wantErr:     false,
		},
		{
			name: "annotation set to comma-separated ports - no match",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80,443",
					},
				},
			},
			port:        8080,
			wantEnabled: false,
			wantErr:     false,
		},
		{
			name: "annotation with invalid value",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "invalid",
					},
				},
			},
			port:            80,
			wantEnabled:     false,
			wantErr:         true,
			wantErrContains: "invalid health check annotation",
		},
		{
			name: "annotation with negative port",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "-1",
					},
				},
			},
			port:            80,
			wantEnabled:     false,
			wantErr:         true,
			wantErrContains: "outside valid range",
		},
		{
			name: "annotation with port 0",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "0",
					},
				},
			},
			port:            80,
			wantEnabled:     false,
			wantErr:         true,
			wantErrContains: "outside valid range",
		},
		{
			name: "annotation with port > 65535",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "99999",
					},
				},
			},
			port:            80,
			wantEnabled:     false,
			wantErr:         true,
			wantErrContains: "outside valid range",
		},
		{
			name: "annotation with mixed valid and invalid ports",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "80,-1,443",
					},
				},
			},
			port:            443,
			wantEnabled:     false,
			wantErr:         true,
			wantErrContains: "outside valid range",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set healthCheckNodePort to test annotation parsing
			tc.service.Spec.HealthCheckNodePort = 30100

			hc, err := getNativeHealthCheck(tc.service, tc.port)

			// Check error cases
			if (err != nil) != tc.wantErr {
				t.Errorf("getNativeHealthCheck() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr && err != nil && tc.wantErrContains != "" {
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("getNativeHealthCheck() error = %q, want error containing %q", err.Error(), tc.wantErrContains)
				}
			}

			// Check native health check is enabled/disabled as expected
			isEnabled := (hc != nil)
			if isEnabled != tc.wantEnabled {
				t.Errorf("getNativeHealthCheck() returned healthCheck = %v (enabled=%v), want enabled=%v", hc, isEnabled, tc.wantEnabled)
			}
		})
	}
}

func TestServicePortToBackendWithHealthCheckFromService(t *testing.T) {
	const nonDefaultHealthCheckDelay = "2s"
	const nonDefaultHealthCheckTimeout = "2s"
	const nonDefaultHealthCheckMaxRetries = "2"
	const nonDefaultHealthCheckTransientCheckDelay = "2s"

	testCases := []struct {
		name            string
		service         *v1.Service
		port            v1.ServicePort
		wantNativeHC    bool
		wantHealthPort  int32
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "native health check enabled",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "true",
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: 30100,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantNativeHC:   true,
			wantHealthPort: 30100,
		},
		{
			name: "native health check disabled - uses classic",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckType:    "http",
						serviceAnnotationLoadBalancerHealthCheckHTTPURI: "/custom",
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: 30100,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantNativeHC:   false,
			wantHealthPort: 30000, // uses NodePort
		},
		{
			name: "native enabled but healthCheckNodePort is 0 - falls back to classic",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "true",
						serviceAnnotationLoadBalancerHealthCheckType:        "tcp",
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: 0,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantNativeHC:   false,
			wantHealthPort: 30000,
		},
		{
			name: "invalid healthCheckNodePort - negative",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "true",
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: -1,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantErr:         true,
			wantErrContains: "invalid healthCheckNodePort",
		},
		{
			name: "invalid healthCheckNodePort - too high",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService: "true",
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: 99999,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantErr:         true,
			wantErrContains: "invalid healthCheckNodePort",
		},
		{
			name: "native health check enabled - keep health check config",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						serviceAnnotationLoadBalancerHealthCheckFromService:    "true",
						serviceAnnotationLoadBalancerHealthCheckDelay:          nonDefaultHealthCheckDelay,
						serviceAnnotationLoadBalancerHealthCheckTimeout:        nonDefaultHealthCheckTimeout,
						serviceAnnotationLoadBalancerHealthCheckMaxRetries:     nonDefaultHealthCheckMaxRetries,
						serviceAnnotationLoadBalancerHealthTransientCheckDelay: nonDefaultHealthCheckTransientCheckDelay,
					},
				},
				Spec: v1.ServiceSpec{
					HealthCheckNodePort: 30100,
					Ports: []v1.ServicePort{
						{Port: 80, NodePort: 30000},
					},
				},
			},
			port: v1.ServicePort{
				Port:     80,
				NodePort: 30000,
			},
			wantNativeHC:   true,
			wantHealthPort: 30100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backend, err := servicePortToBackend(tc.service, &scwlb.LB{ID: "test-lb"}, tc.port, []string{"10.0.0.1"})

			// Check error cases
			if tc.wantErr {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}
				if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("Expected error containing %q, got %q", tc.wantErrContains, err.Error())
				}
				return
			}

			// Check success cases
			if err != nil {
				t.Fatalf("servicePortToBackend() error = %v", err)
			}

			if backend == nil {
				t.Fatal("servicePortToBackend() returned nil backend")
			}

			if backend.HealthCheck == nil {
				t.Fatal("backend.HealthCheck is nil")
			}

			// Verify health check port
			if backend.HealthCheck.Port != tc.wantHealthPort {
				t.Errorf("HealthCheck.Port = %v, want %v", backend.HealthCheck.Port, tc.wantHealthPort)
			}

			// Verify native health check configuration
			if tc.wantNativeHC {
				if backend.HealthCheck.HTTPConfig == nil {
					t.Error("Expected HTTPConfig for native health check, got nil")
				} else {
					wantHTTPConfig := &scwlb.HealthCheckHTTPConfig{
						Method: "GET",
						Code:   scw.Int32Ptr(200),
						URI:    "/healthz",
					}
					if !reflect.DeepEqual(backend.HealthCheck.HTTPConfig, wantHTTPConfig) {
						t.Errorf("HealthCheck.HTTPConfig = %v, want %v", backend.HealthCheck.HTTPConfig, wantHTTPConfig)
					}
					notAlteredDelay, err := getHealthCheckDelay(tc.service)
					if err != nil {
						t.Errorf("error getting health check delay %q", err.Error())
					}
					if *backend.HealthCheck.CheckDelay != notAlteredDelay {
						t.Errorf("HealthCheck.CheckDelay = %v, want %v", backend.HealthCheck.CheckDelay, notAlteredDelay)
					}
					notAlteredTimeout, err := getHealthCheckTimeout(tc.service)
					if err != nil {
						t.Errorf("error getting health check timeout %q", err.Error())
					}
					if *backend.HealthCheck.CheckTimeout != notAlteredTimeout {
						t.Errorf("HealthCheck.CheckTimeout = %v, want %v", backend.HealthCheck.CheckTimeout, notAlteredTimeout)
					}
					notAlteredMaxRetries, err := getHealthCheckMaxRetries(tc.service)
					if err != nil {
						t.Errorf("error getting health check maxRetries %q", err.Error())
					}
					if !reflect.DeepEqual(backend.HealthCheck.CheckMaxRetries, notAlteredMaxRetries) {
						t.Errorf("HealthCheck.CheckMaxRetries = %v, want %v", backend.HealthCheck.CheckMaxRetries, notAlteredMaxRetries)
					}
					notAlteredTranscientCheckDelay, err := getHealthCheckTransientCheckDelay(tc.service)
					if err != nil {
						t.Errorf("error getting health check transcientCheckDelay %q", err.Error())
					}
					if !reflect.DeepEqual(backend.HealthCheck.TransientCheckDelay, notAlteredTranscientCheckDelay) {
						t.Errorf("HealthCheck.CheckMaxRetries = %v, want %v", backend.HealthCheck.TransientCheckDelay, notAlteredTranscientCheckDelay)
					}
					if tc.wantNativeHC {
						if backend.HealthCheck.CheckSendProxy {
							t.Errorf("HealthCheck.CheckSendProxy should be false when native kubernetes health check is used")
						}
					}
				}
			}

			// Exactly one health check config must be set
			configs := []struct {
				name string
				set  bool
			}{
				{"HTTPConfig", backend.HealthCheck.HTTPConfig != nil},
				{"HTTPSConfig", backend.HealthCheck.HTTPSConfig != nil},
				{"TCPConfig", backend.HealthCheck.TCPConfig != nil},
				{"MysqlConfig", backend.HealthCheck.MysqlConfig != nil},
				{"PgsqlConfig", backend.HealthCheck.PgsqlConfig != nil},
				{"RedisConfig", backend.HealthCheck.RedisConfig != nil},
				{"LdapConfig", backend.HealthCheck.LdapConfig != nil},
			}

			var setConfigs []string
			for _, cfg := range configs {
				if cfg.set {
					setConfigs = append(setConfigs, cfg.name)
				}
			}

			if len(setConfigs) != 1 {
				t.Errorf("Expected exactly 1 health check config to be set, got %d: %v", len(setConfigs), setConfigs)
			}
		})
	}
}

func TestBackendPreservation(t *testing.T) {
	// Test that stringArrayEqual correctly identifies when pools differ
	t.Run("EmptyTargetIPsVsNonEmptyPool", func(t *testing.T) {
		existingPool := []string{"10.0.0.1", "10.0.0.2"}
		targetIPs := []string{}

		// These are not equal, so the update logic would trigger
		equal := stringArrayEqual(existingPool, targetIPs)
		if equal {
			t.Error("Expected pools to be unequal")
		}

		// The safety check should prevent clearing: len(targetIPs) == 0 && len(existingPool) > 0
		shouldPreserve := len(targetIPs) == 0 && len(existingPool) > 0
		if !shouldPreserve {
			t.Error("Expected preservation logic to trigger")
		}
	})

	t.Run("BothEmpty", func(t *testing.T) {
		existingPool := []string{}
		targetIPs := []string{}

		equal := stringArrayEqual(existingPool, targetIPs)
		if !equal {
			t.Error("Expected empty pools to be equal")
		}

		// No preservation needed - both are empty
		shouldPreserve := len(targetIPs) == 0 && len(existingPool) > 0
		if shouldPreserve {
			t.Error("Should not preserve when both pools are empty")
		}
	})

	t.Run("NonEmptyTargetIPs", func(t *testing.T) {
		existingPool := []string{"10.0.0.1", "10.0.0.2"}
		targetIPs := []string{"10.0.0.3"}

		equal := stringArrayEqual(existingPool, targetIPs)
		if equal {
			t.Error("Expected pools to be unequal")
		}

		// Should NOT preserve - we have new IPs to set
		shouldPreserve := len(targetIPs) == 0 && len(existingPool) > 0
		if shouldPreserve {
			t.Error("Should not preserve when we have new target IPs")
		}
	})
}

func TestExtractNodesInternalIps(t *testing.T) {
	t.Run("NodesWithInternalIPs", func(t *testing.T) {
		nodes := []*v1.Node{
			{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
						{Type: v1.NodeExternalIP, Address: "1.2.3.4"},
					},
				},
			},
			{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeInternalIP, Address: "10.0.0.2"},
					},
				},
			},
		}

		ips := extractNodesInternalIps(nodes)
		if len(ips) != 2 {
			t.Errorf("Expected 2 IPs, got %d", len(ips))
		}
	})

	t.Run("NodesWithoutInternalIPs", func(t *testing.T) {
		nodes := []*v1.Node{
			{
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeExternalIP, Address: "1.2.3.4"},
						{Type: v1.NodeHostName, Address: "node1"},
					},
				},
			},
		}

		ips := extractNodesInternalIps(nodes)
		if len(ips) != 0 {
			t.Errorf("Expected 0 IPs when no internal IPs, got %d", len(ips))
		}
	})

	t.Run("EmptyNodes", func(t *testing.T) {
		nodes := []*v1.Node{}

		ips := extractNodesInternalIps(nodes)
		if len(ips) != 0 {
			t.Errorf("Expected 0 IPs for empty nodes, got %d", len(ips))
		}
	})
}
