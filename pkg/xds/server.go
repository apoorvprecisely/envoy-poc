package xds

import (
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
)

// Server is a collection of handlers for streaming discovery requests.
type Server interface {
	envoy_service_cluster_v3.ClusterDiscoveryServiceServer
	envoy_service_endpoint_v3.EndpointDiscoveryServiceServer
	envoy_service_listener_v3.ListenerDiscoveryServiceServer
	envoy_service_route_v3.RouteDiscoveryServiceServer
	envoy_service_discovery_v3.AggregatedDiscoveryServiceServer
	envoy_service_secret_v3.SecretDiscoveryServiceServer
}

// RegisterServer registers the given xDS protocol Server with the gRPC
// runtime.
func RegisterServer(srv Server, g *grpc.Server) {
	// register services
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(g, srv)
	envoy_service_secret_v3.RegisterSecretDiscoveryServiceServer(g, srv)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(g, srv)
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(g, srv)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(g, srv)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(g, srv)
}
