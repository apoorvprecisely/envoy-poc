package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/apoorvprecisely/envoy-poc/pkg/locator"
	"github.com/apoorvprecisely/envoy-poc/pkg/streamer"
	sam_zk "github.com/samuel/go-zookeeper/zk"
	"github.com/unbxd/go-base/base/drivers/zook"
	"github.com/unbxd/go-base/base/endpoint"
	gb_log "github.com/unbxd/go-base/base/log"
	"github.com/unbxd/go-base/base/transport/zk"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"

	"google.golang.org/grpc"
)

var server *grpc.Server

func main() {
	logger, err := gb_log.NewZapLogger(
		gb_log.ZapWithLevel("error"),
		gb_log.ZapWithEncoding("console"),
		gb_log.ZapWithOutput([]string{"stdout"}),
	)
	hub := hub.NewHub()
	loc, err := locator.NewLocator()
	if err != nil {
		panic(err)
	}
	//add watch
	zkD := zook.NewZKDriver([]string{"zook1:2181"}, time.Duration(2000)*time.Millisecond, "/solr")
	err = zkD.Open()
	if err != nil {
		panic(err)
	}
	con, err := zk.NewConsumer(logger, "/solr/aliases.json", []zk.ConsumerOption{
		zk.WithZkDriver(zkD),
		zk.WithEndpointConsumerOption(createPubEP(hub, loc)),
		zk.WithReconnectOnErrConsumerOption(func(err error) bool {
			if err == sam_zk.ErrNoNode {
				return false
			}
			return true
		}),
		zk.WithDelayOnErrConsumerOption(func(err error) time.Duration {
			if err != nil {
				return 1 * time.Second
			}
			return 0
		})}...)

	if err != nil {
		panic(err)
	}
	server = grpc.NewServer()
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(server, streamer.New(hub, loc))
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 8053))
	if err != nil {
		log.Fatalf("failed to listen: %v\n", err)
	}
	server.Serve(lis)
}
func createPubEP(hubService hub.Service, locator locator.Service) endpoint.Endpoint {
	return func(_ context.Context, request interface{}) (interface{}, error) {
		cla, err := locator.CLA()
		if err != nil {
			log.Printf("failed to decode locator values : %v", err)
			return nil, err
		}
		clusters, err := locator.Clusters()
		if err != nil {
			log.Printf("failed to decode locator values : %v", err)
			return nil, err
		}
		routes, err := locator.Routes()
		if err != nil {
			log.Printf("failed to decode locator values : %v", err)
			return nil, err
		}
		hubService.Publish(&hub.Event{CLA: cla, Clusters: clusters, Routes: routes})
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}
