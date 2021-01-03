package streamer

import (
	"io"
	"log"

	"github.com/apoorvprecisely/envoy-poc/pkg/locator"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
)

type SubscriptionStream interface {
	Stream() error
}
type subscriptionStream struct {
	stream       discoverygrpc.AggregatedDiscoveryService_StreamAggregatedResourcesServer
	subscription *hub.Subscription
	locator      locator.Service
	hub          hub.Service
}

func (es *subscriptionStream) Stream() error {
	var terminate chan bool

	go func() {
		for {
			in, err := es.stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Printf("failed to receive message on stream: %v", err)
				return
			} else if in.VersionInfo == "" {
				log.Printf("received discovery request on stream: %v", in)
				// request being written, how no clue,based on this a DiscoveryResponse is being written
				cla, err := es.locator.CLA()
				if err != nil {
					log.Printf("failed to decode locator values : %v", err)
					return
				}
				clusters, err := es.locator.Clusters()
				if err != nil {
					log.Printf("failed to decode locator values : %v", err)
					return
				}
				routes, err := es.locator.Routes()
				if err != nil {
					log.Printf("failed to decode locator values : %v", err)
					return
				}
				es.hub.Publish(&hub.Event{CLA: cla, Clusters: clusters, Routes: routes})
			} else {
				log.Printf("received ACK on stream: %v", in)
			}
		}
	}()

	go func() {
		responseStream := NewDiscoveryResponseStream(es.stream)
		// this is where on getting an event DiscoveryResponse object is written on grpc
		for {
			select {
			case e, open := <-es.subscription.Events:
				if !open {
					log.Printf("Stopped listening to events channel since it has been closed")
					return
				}
				if e != nil {
					responseStream.SendCDS(e.Clusters)
					responseStream.SendRDS(e.Routes)
					responseStream.SendEDS(e.CLA)
				}
			}
		}
	}()
	go func() {
		select {
		case <-es.stream.Context().Done():
			log.Printf("stream context done")
			es.subscription.Close()
			terminate <- true
		}
	}()
	<-terminate
	return nil
}
func NewSubscriptionStream(
	stream discoverygrpc.AggregatedDiscoveryService_StreamAggregatedResourcesServer,
	subscription *hub.Subscription,
	service locator.Service,
	hub hub.Service) SubscriptionStream {
	return &subscriptionStream{stream: stream, subscription: subscription, locator: service, hub: hub}
}
