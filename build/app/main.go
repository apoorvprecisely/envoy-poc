package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/apoorvprecisely/envoy-poc/pkg/locator"
	"github.com/apoorvprecisely/envoy-poc/pkg/streamer"
	"github.com/go-kit/kit/endpoint"
	"github.com/unbxd/go-base/base/drivers/zook"
	"github.com/unbxd/go-base/base/log"
	gb_log "github.com/unbxd/go-base/base/log"
	"github.com/unbxd/go-base/base/transport/zk"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"

	"google.golang.org/grpc"
)

var server *grpc.Server

func main() {
	logger, err := gb_log.NewZapLogger(
		log.ZapWithLevel("error"),
		log.ZapWithEncoding("console"),
		log.ZapWithOutput("stdout"),
	)
	hub := hub.NewHub()
	loc := locator.NewLocator()
	//add watch
	zkD := zook.NewZKDriver([]string{"zook1:2181"}, time.Duration(2000)*time.Millisecond, "/solr")
	err := zkD.Open()
	if err != nil {
		return nil, err
	}
	con, err := zk.NewConsumer(z.logger, "/solr/aliases.json", []zk.ConsumerOption{
		zk.WithZkDriver(zkD),
		zk.WithEndpointConsumerOption(createPubEP(hub,loc)),
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
		})})

	if err != nil {
		return err
	}
	server = grpc.NewServer()
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(server, streamer.New(hub, loc))
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port()))
	if err != nil {
		log.Fatalf("failed to listen: %v\n", err)
	}
	server.Serve(lis)
}
func createPubEP(hub hub.Service,loc locator.Service) endpoint.Endpoint {
	return func(_ context.Context, request interface{}) (interface{}, error) {
		hub.Publish(&pubsub.Event{CLA: locator.CLA(), Clusters: locator.Clusters(), Routes: locator.Routes())
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}
