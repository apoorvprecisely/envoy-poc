package streamer

import (
	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"
)

type DiscoveryStream interface {
	Send(*discovery.DiscoveryResponse) error
	Recv() (*discovery.DiscoveryRequest, error)
	grpc.ServerStream
}

type Service struct {
	hub     hub.Service
	service endpoint.Endpoint
}

//StreamAggregatedResources is a grpc streaming api for streaming Discovery responses
func (e *Service) StreamAggregatedResources(s discoverygrpc.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	subscription := e.hub.Subscribe()
	return NewSubscriptionStream(s, subscription, e.service, e.hub).Stream()
}

func New(hub hub.Service, service endpoint.Endpoint) *Service {
	return &Service{hub: hub, service: service}
}
