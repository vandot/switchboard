package integrations

import (
	"context"
	"testing"

	"github.com/borchero/switchboard/internal/k8tests"
	"github.com/borchero/switchboard/internal/switchboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/external-dns/endpoint"
)

func TestExternalDNSWatchedObject(t *testing.T) {
	integration := NewExternalDNS(nil, switchboard.NewTarget("my-name", "my-namespace"))
	obj := integration.WatchedObject()
	assert.Equal(t, "my-name", obj.GetName())
	assert.Equal(t, "my-namespace", obj.GetNamespace())
}

func TestExternalDNSUpdateResource(t *testing.T) {
	// Setup
	ctx := context.Background()
	scheme := k8tests.NewScheme()
	client := k8tests.NewClient(t, scheme)
	namespace, shutdown := k8tests.NewNamespace(ctx, t, client)
	defer shutdown()

	// Create a dummy service as owner and target
	owner := k8tests.DummyService("my-service", namespace, 80)
	err := client.Create(ctx, &owner)
	require.Nil(t, err)
	integration := NewExternalDNS(client, switchboard.NewTarget(owner.Name, namespace))

	// No resource should be created if no hosts are provided
	info := IngressInfo{}
	err = integration.UpdateResource(ctx, &owner, info)
	require.Nil(t, err)
	assert.Len(t, getDNSEndpoints(ctx, t, client, namespace), 0)

	// A resource with the name of the service should be created for at least one host
	info.Hosts = []string{"example.com"}
	err = integration.UpdateResource(ctx, &owner, info)
	require.Nil(t, err)
	endpoints := getDNSEndpoints(ctx, t, client, namespace)
	assert.Len(t, endpoints, 1)
	assert.Contains(t, endpoints, owner.Name)
	assert.ElementsMatch(t, endpoints[owner.Name], info.Hosts)

	// When the hosts are changed, more endpoints should be added
	info.Hosts = []string{"example.com", "www.example.com"}
	err = integration.UpdateResource(ctx, &owner, info)
	require.Nil(t, err)
	endpoints = getDNSEndpoints(ctx, t, client, namespace)
	assert.Len(t, endpoints, 1)
	assert.Contains(t, endpoints, owner.Name)
	assert.ElementsMatch(t, endpoints[owner.Name], info.Hosts)

	// When no hosts are set, the endpoints should be removed
	info.Hosts = nil
	err = integration.UpdateResource(ctx, &owner, info)
	require.Nil(t, err)
	assert.Len(t, getDNSEndpoints(ctx, t, client, namespace), 0)
}

func TestExternalDNSEndpoints(t *testing.T) {
	integration := externalDNS{ttl: 250}
	hosts := []string{"example.com", "www.example.com"}
	endpoints := integration.endpoints(hosts, "127.0.0.1")
	assert.Len(t, endpoints, 2)
	for _, ep := range endpoints {
		assert.ElementsMatch(t, ep.Targets, []string{"127.0.0.1"})
		assert.Equal(t, ep.RecordTTL, endpoint.TTL(250))
		assert.Equal(t, ep.RecordType, "A")
		assert.Contains(t, hosts, ep.DNSName)
	}
}

//-------------------------------------------------------------------------------------------------
// UTILS
//-------------------------------------------------------------------------------------------------

func getDNSEndpoints(
	ctx context.Context, t *testing.T, ctrlClient client.Client, namespace string,
) map[string][]string {
	var list endpoint.DNSEndpointList
	err := ctrlClient.List(ctx, &list, &client.ListOptions{
		Namespace: namespace,
	})
	require.Nil(t, err)

	result := make(map[string][]string)
	for _, item := range list.Items {
		names := make([]string, 0)
		for _, ep := range item.Spec.Endpoints {
			names = append(names, ep.DNSName)
		}
		result[item.Name] = names
	}
	return result
}