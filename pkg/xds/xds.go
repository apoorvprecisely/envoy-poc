package xds

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	"github.com/apoorvprecisely/envoy-poc/pkg/locator"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	envoy_service_secret_v3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type grpcStream interface {
	Context() context.Context
	Send(*envoy_service_discovery_v3.DiscoveryResponse) error
	Recv() (*envoy_service_discovery_v3.DiscoveryRequest, error)
}

func NewXdsServer(log logrus.FieldLogger, subscription *hub.Subscription,
	service locator.Service,
	hub hub.Service) Server {
	c := xdsServer{
		FieldLogger:  log,
		subscription: subscription, locator: service, hub: hub}
	return &c
}

type xdsServer struct {
	// Since we only implement the streaming state of the world
	// protocol, embed the default null implementations to handle
	// the unimplemented gRPC endpoints.
	envoy_service_discovery_v3.UnimplementedAggregatedDiscoveryServiceServer
	envoy_service_secret_v3.UnimplementedSecretDiscoveryServiceServer
	envoy_service_route_v3.UnimplementedRouteDiscoveryServiceServer
	envoy_service_endpoint_v3.UnimplementedEndpointDiscoveryServiceServer
	envoy_service_cluster_v3.UnimplementedClusterDiscoveryServiceServer
	envoy_service_listener_v3.UnimplementedListenerDiscoveryServiceServer

	logrus.FieldLogger
	subscription *hub.Subscription
	locator      locator.Service
	hub          hub.Service
}

// stream processes a stream of DiscoveryRequests.
func (es *xdsServer) stream(st grpcStream) error {
	var terminate chan bool
	log.Printf("opened stream")

	go func() {
		for {
			in, err := st.Recv()
			if err == io.EOF {
				return
			}
			fmt.Println("type:" + in.GetTypeUrl() + " resources:" + strings.Join(in.GetResourceNames(), ","))
			if err != nil {
				log.Printf("failed to receive message on stream: %v", err)
				return
			} else if in.VersionInfo == "" {
				log.Printf("received discovery request on stream: %v", in)
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
				listeners, err := es.locator.Listeners()
				if err != nil {
					log.Printf("failed to decode locator values : %v", err)
					return
				}
				es.hub.Publish(&hub.Event{CLA: cla, Clusters: clusters, Routes: routes, Listeners: listeners})
			} else {
				log.Printf("received ACK on stream: %v", in)
			}
		}
	}()

	go func() {
		// this is where on getting an event DiscoveryResponse object is written on grpc
		for {
			select {
			case e, open := <-es.subscription.Events:
				if !open {
					log.Printf("Stopped listening to events channel since it has been closed")
					return
				}
				if e != nil {
					//cla
					resp, err := GetEDS(e.CLA)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					err = st.Send(resp)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					//clusters
					resp, err = GetClusters(e.Clusters)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					err = st.Send(resp)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					//routes
					resp, err = GetRoutes(e.Routes)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					err = st.Send(resp)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					//listeners
					resp, err = GetListeners(e.Listeners)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
					err = st.Send(resp)
					if err != nil {
						log.Printf("failed to decode locator values : %v", err)
					}
				}
			}
		}
	}()
	go func() {
		select {
		case <-st.Context().Done():
			log.Printf("stream context done")
			es.subscription.Close()
			terminate <- true
		}
	}()
	<-terminate
	return nil
}
func GetEDS(cLAList []*endpoint.ClusterLoadAssignment) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	var resources []*any.Any
	for _, cLA := range cLAList {
		data, err := proto.Marshal(cLA)
		if err != nil {
			return nil, err
		}
		resources = append(resources, &any.Any{
			TypeUrl: "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment",
			Value:   data,
		})
	}

	resp := &envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: strconv.FormatInt(int64(time.Now().UnixNano()), 10),
		Resources:   resources,
		TypeUrl:     "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment",
		Nonce:       strconv.FormatInt(int64(time.Now().UnixNano()), 10),
	}
	return resp, nil
}

func GetClusters(cLAList []*cluster.Cluster) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	var resources []*any.Any
	for _, cLA := range cLAList {
		data, err := proto.Marshal(cLA)
		if err != nil {
			return nil, err
		}
		resources = append(resources, &any.Any{
			TypeUrl: "type.googleapis.com/envoy.config.cluster.v3.Cluster",
			Value:   data,
		})
	}

	resp := &envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: strconv.FormatInt(int64(time.Now().UnixNano()), 10),
		Resources:   resources,
		TypeUrl:     "type.googleapis.com/envoy.config.cluster.v3.Cluster",
		Nonce:       strconv.FormatInt(int64(time.Now().UnixNano()), 10),
	}
	return resp, nil
}
func GetRoutes(cLAList []*route.RouteConfiguration) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	var resources []*any.Any
	for _, cLA := range cLAList {
		data, err := proto.Marshal(cLA)
		if err != nil {
			return nil, err
		}
		resources = append(resources, &any.Any{
			TypeUrl: "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
			Value:   data,
		})
	}

	resp := &envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: strconv.FormatInt(int64(time.Now().UnixNano()), 10),
		Resources:   resources,
		TypeUrl:     "type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
		Nonce:       strconv.FormatInt(int64(time.Now().UnixNano()), 10),
	}
	return resp, nil
}

func GetListeners(cLAList []*listener.Listener) (*envoy_service_discovery_v3.DiscoveryResponse, error) {
	var resources []*any.Any
	for _, cLA := range cLAList {
		data, err := proto.Marshal(cLA)
		if err != nil {
			return nil, err
		}
		resources = append(resources, &any.Any{
			TypeUrl: "type.googleapis.com/envoy.config.listener.v3.Listener",
			Value:   data,
		})
	}

	resp := &envoy_service_discovery_v3.DiscoveryResponse{
		VersionInfo: strconv.FormatInt(int64(time.Now().UnixNano()), 10),
		Resources:   resources,
		TypeUrl:     "type.googleapis.com/envoy.config.listener.v3.Listener",
		Nonce:       strconv.FormatInt(int64(time.Now().UnixNano()), 10),
	}
	return resp, nil
}

func (s *xdsServer) StreamClusters(srv envoy_service_cluster_v3.ClusterDiscoveryService_StreamClustersServer) error {
	return s.stream(srv)
}

func (s *xdsServer) StreamEndpoints(srv envoy_service_endpoint_v3.EndpointDiscoveryService_StreamEndpointsServer) error {
	return s.stream(srv)
}

func (s *xdsServer) StreamListeners(srv envoy_service_listener_v3.ListenerDiscoveryService_StreamListenersServer) error {
	return s.stream(srv)
}

func (s *xdsServer) StreamRoutes(srv envoy_service_route_v3.RouteDiscoveryService_StreamRoutesServer) error {
	return s.stream(srv)
}

func (s *xdsServer) StreamSecrets(srv envoy_service_secret_v3.SecretDiscoveryService_StreamSecretsServer) error {
	return s.stream(srv)
}
