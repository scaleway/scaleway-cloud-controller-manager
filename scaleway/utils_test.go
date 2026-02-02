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
	"fmt"
	"net/http"
	"testing"

	"github.com/scaleway/scaleway-sdk-go/scw"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "500 Internal Server Error",
			err:  &scw.ResponseError{StatusCode: http.StatusInternalServerError},
			want: true,
		},
		{
			name: "502 Bad Gateway",
			err:  &scw.ResponseError{StatusCode: http.StatusBadGateway},
			want: true,
		},
		{
			name: "503 Service Unavailable",
			err:  &scw.ResponseError{StatusCode: http.StatusServiceUnavailable},
			want: true,
		},
		{
			name: "504 Gateway Timeout",
			err:  &scw.ResponseError{StatusCode: http.StatusGatewayTimeout},
			want: true,
		},
		{
			name: "400 Bad Request",
			err:  &scw.ResponseError{StatusCode: http.StatusBadRequest},
			want: false,
		},
		{
			name: "401 Unauthorized",
			err:  &scw.ResponseError{StatusCode: http.StatusUnauthorized},
			want: false,
		},
		{
			name: "403 Forbidden",
			err:  &scw.ResponseError{StatusCode: http.StatusForbidden},
			want: false,
		},
		{
			name: "404 Not Found",
			err:  &scw.ResponseError{StatusCode: http.StatusNotFound},
			want: false,
		},
		{
			name: "200 OK",
			err:  &scw.ResponseError{StatusCode: http.StatusOK},
			want: false,
		},
		{
			name: "non-ResponseError",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "ResourceNotFoundError",
			err:  &scw.ResourceNotFoundError{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIs404Error(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "404 Not Found ResponseError",
			err:  &scw.ResponseError{StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "ResourceNotFoundError",
			err:  &scw.ResourceNotFoundError{},
			want: true,
		},
		{
			name: "500 Internal Server Error",
			err:  &scw.ResponseError{StatusCode: http.StatusInternalServerError},
			want: false,
		},
		{
			name: "non-ResponseError",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := is404Error(tt.err)
			if got != tt.want {
				t.Errorf("is404Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
