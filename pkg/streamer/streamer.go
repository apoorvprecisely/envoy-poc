package streamer

import (
	"errors"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	"github.com/apoorvprecisely/envoy-poc/pkg/locator"
	discovery "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"google.golang.org/grpc"
)

type DiscoveryStream interface {
	Send(*discovery.DiscoveryResponse) error
	Recv() (*discovery.DiscoveryRequest, error)
	grpc.ServerStream
}

type Service struct {
	hub     hub.Service
	locator locator.Service
}

//StreamAggregatedResources is a grpc streaming api for streaming Discovery responses
func (e *Service) StreamAggregatedResources(s discoverygrpc.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	subscription := e.hub.Subscribe()
	return NewSubscriptionStream(s, subscription, e.locator, e.hub).Stream()
}

func (s *Service) DeltaAggregatedResources(_ discoverygrpc.AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error {
	return errors.New("not implemented")
}

func New(hub hub.Service, locator locator.Service) *Service {
	return &Service{hub: hub, locator: locator}
}
