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
	}

	for _, tt := range matrix {
		t.Run(tt.name, func(t *testing.T) {
			got := frontendEquals(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", got, tt.want)
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
	otherDuration, _ := time.ParseDuration("50m")
	boolTrue := true
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
		TimeoutServer:          &defaultTimeoutServer,
		TimeoutConnect:         &defaultTimeoutConnect,
		TimeoutTunnel:          &defaultTimeoutTunnel,
		OnMarkedDownAction:     "action",
		ProxyProtocol:          "proxy",
		RedispatchAttemptCount: &int0,
		MaxRetries:             &int1,
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
	}{"with same HTTP healthchecks", httpRef, httpDiff, false})

	for _, tt := range matrix {
		t.Run(tt.Name, func(t *testing.T) {
			got := backendEquals(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
				t.Errorf("want: %v, got: %v", got, tt.want)
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
