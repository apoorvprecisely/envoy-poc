package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/apoorvprecisely/envoy-poc/internal/hub"
	"github.com/apoorvprecisely/envoy-poc/pkg/locator"
	"github.com/apoorvprecisely/envoy-poc/pkg/xds"
	"github.com/unbxd/go-base/base/drivers/zook"
	"github.com/unbxd/go-base/base/endpoint"

	logrus "github.com/sirupsen/logrus"
	gb_log "github.com/unbxd/go-base/base/log"

	"github.com/unbxd/go-base/base/transport/zk"

	"google.golang.org/grpc"
)

var server *grpc.Server

func main() {

	hub := hub.NewHub()
	loc, err := locator.NewLocator()
	if err != nil {
		panic(err)
	}
	log.Printf("creating grpc server")

	server = grpc.NewServer()
	subscription := hub.Subscribe()
	xds.RegisterServer(xds.NewXdsServer(logrus.New(), subscription, loc, hub), server)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", 8053))
	if err != nil {
		log.Fatalf("failed to listen: %v\n", err)
	}
	log.Printf("Registered discovery service server")
	//add watch
	logger, err := gb_log.NewZapLogger(
		gb_log.ZapWithLevel("error"),
		gb_log.ZapWithEncoding("console"),
		gb_log.ZapWithOutput([]string{"stdout"}),
	)
	zkD := zook.NewZKDriver([]string{"zook1:2181"}, time.Duration(2000)*time.Millisecond, "/solr")
	err = zkD.Open()
	if err != nil {
		panic(err)
	}
	con, err := zk.NewConsumer(logger, "/solr/aliases.json", []zk.ConsumerOption{
		zk.WithZkDriver(zkD),
		zk.WithEndpointConsumerOption(createPubEP(hub, loc))}...)

	if err != nil {
		panic(err)
	}
	go func(zc *zk.Consumer) {
		err := con.Open()
		if err != nil {
			panic(err)
		}
	}(con)

	log.Printf("Registered watch")
	err = server.Serve(lis)
	if err != nil {
		panic(err)
	}
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
		listeners, err := locator.Listeners()
		if err != nil {
			log.Printf("failed to decode locator values : %v", err)
			return nil, err
		}
		hubService.Publish(&hub.Event{CLA: cla, Clusters: clusters, Routes: routes, Listeners: listeners})
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
}
