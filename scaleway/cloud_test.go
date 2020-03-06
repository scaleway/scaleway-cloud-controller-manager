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
	"os"
	"strings"
	"testing"
)

func init() {
	// dummy data to test client
	_ = os.Setenv("SCW_ACCESS_KEY", "SCWXXXXXXXXXXXXXXXXX")
	_ = os.Setenv("SCW_SECRET_KEY", "00000000-0000-0000-0000-000000000000")
	_ = os.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "00000000-0000-0000-0000-000000000000")
	_ = os.Setenv("SCW_DEFAULT_PROJECT_ID", "00000000-0000-0000-0000-000000000000")
	_ = os.Setenv("SCW_DEFAULT_REGION", "fr-par")
	_ = os.Setenv("SCW_DEFAULT_ZONE", "fr-par-1")
}

func TestNewCloud(t *testing.T) {
	_, err := newCloud(strings.NewReader(""))
	AssertNoError(t, err)
}

func TestCloud(t *testing.T) {
	cloudInterface, err := newCloud(strings.NewReader(""))
	AssertNoError(t, err)

	t.Run("Instances", func(t *testing.T) {
		_, supported := cloudInterface.Instances()
		AssertTrue(t, supported)
	})

	t.Run("Zones", func(t *testing.T) {
		_, supported := cloudInterface.Zones()
		AssertTrue(t, supported)
	})

	t.Run("LoadBalancer", func(t *testing.T) {
		_, supported := cloudInterface.LoadBalancer()
		AssertTrue(t, supported)
	})

	t.Run("Clusters", func(t *testing.T) {
		_, supported := cloudInterface.Clusters()
		AssertFalse(t, supported)
	})

	t.Run("Routes", func(t *testing.T) {
		_, supported := cloudInterface.Routes()
		AssertFalse(t, supported)
	})

	t.Run("HasClusterID", func(t *testing.T) {
		AssertFalse(t, cloudInterface.HasClusterID())
	})

	t.Run("ProviderName", func(t *testing.T) {
		Equals(t, cloudInterface.ProviderName(), "scaleway")
	})
}
