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

	"github.com/scaleway/scaleway-sdk-go/scw"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
)

type zones struct {
	region  scw.Region
	servers ServersZones
}

type ServersZones interface {
	GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error)
	GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error)
}

func newZones(client *client, pnID string) *zones {
	region, _ := client.scaleway.GetDefaultRegion()
	return &zones{
		servers: newServers(client, pnID),
		region:  region,
	}
}

// GetZone returns the Zone containing the current failure zone and locality region that the program is running in
// In most cases, this method is called from the kubelet querying a local metadata service to acquire its zone.
func (z zones) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	return cloudprovider.Zone{Region: string(z.region)}, nil
}

// GetZoneByProviderID is instance specific, forwarding to instance implementation.
func (z *zones) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	return z.servers.GetZoneByProviderID(ctx, providerID)
}

// GetZoneByNodeName is instance specific, forwarding to instance implementation.
func (z *zones) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	return z.servers.GetZoneByNodeName(ctx, nodeName)
}
