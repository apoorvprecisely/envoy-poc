package locator

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/unbxd/go-base/base/drivers"
	"github.com/unbxd/go-base/base/drivers/zook"
)

type (
	Collection struct {
		Shards            map[string]*Shard `json:"shards"`
		ReplicationFactor string            `mapstructure:"replicationFactor"`
	}

	Shard struct {
		Name     string              `json:"name"`
		Range    string              `json:"range"`
		State    string              `json:"state"`
		Replicas map[string]*Replica `json:"replicas"`
	}

	Replica struct {
		Core     string `json:"core"`
		Leader   string `json:"leader"`
		BaseUrl  string `json:"base_url"`
		NodeName string `json:"node_name"`
		State    string `json:"state"`
	}
)

type Service interface {
	Clusters() ([]*cluster.Cluster, error)
	Routes() ([]*route.RouteConfiguration, error)
	CLA() ([]*endpoint.ClusterLoadAssignment, error)
	Listeners() ([]*listener.Listener, error)
}

const (
	regexPathIdentifier = "%regex:"
)

type agent struct {
	driver            drivers.Driver
	enableHealthCheck bool
}

//Helper functions
func (a agent) getAliasMap() (map[string]string, error) {
	bt, err := a.driver.Read("/solr/aliases.json")
	if err != nil {
		return nil, err
	}
	var al map[string]map[string]string
	err = json.Unmarshal(bt, &al)
	if err != nil {
		return nil, err
	}
	c, ok := al["collection"]
	if !ok {
		return make(map[string]string), nil
	}
	return c, nil
}
func (a agent) getCollectionLocations(collection string) ([]string, error) {
	bt, err := a.driver.Read("/solr/collections/" + collection + "/state.json")
	if err != nil {
		return nil, err
	}
	var mm map[string]Collection
	err = json.Unmarshal(bt, &mm)
	if err != nil {
		return nil, err
	}
	var locations []string
	for _, v := range mm[collection].Shards["shard1"].Replicas {
		locations = append(locations, strings.Trim(strings.Trim(v.BaseUrl, ":8983/solr"), "http://"))
	}
	return locations, nil
}

func (a *agent) CLA() ([]*endpoint.ClusterLoadAssignment, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var cLAs []*endpoint.ClusterLoadAssignment
	for alias, collection := range aliasMap {
		ep, err := a.getCollectionLocations(collection)
		if err != nil {
			return nil, err
		}
		cLAs = append(cLAs, MakeEndpoint(alias, ep))
	}
	return cLAs, nil
}

func (a *agent) Clusters() ([]*cluster.Cluster, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var clusters []*cluster.Cluster
	for alias := range aliasMap {
		clusters = append(clusters, MakeCluster(alias))

	}
	return clusters, nil
}

func (a *agent) Routes() ([]*route.RouteConfiguration, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var routes []string
	var routesC []*route.RouteConfiguration
	for alias := range aliasMap {
		routes = append(routes, alias)
	}
	routesC = append(routesC, MakeRoute(routes))
	return routesC, nil
}

/*
[2021-01-06 01:15:27.495][1424213][warning][config] [source/common/config/grpc_subscription_impl.cc:107] gRPC config for type.googleapis.com/envoy.config.listener.v3.Listener rejected: Error adding/updating listener(s) test_read: envoy.config.core.v3.ApiConfigSource must have a statically defined non-EDS cluster: 'test_read' does not exist, was added via api, or is an EDS cluster
test_write: envoy.config.core.v3.ApiConfigSource must have a statically defined non-EDS cluster: 'test_write' does not exist, was added via api, or is an EDS cluster

*/
func (a *agent) Listeners() ([]*listener.Listener, error) {
	var listeners []*listener.Listener
	// for alias := range aliasMap {
	listeners = append(listeners, MakeHTTPListener("listener_0", "listener_0", "0.0.0.0", 9000))
	// }
	return listeners, nil
}

func NewLocator() (Service, error) {
	zkD := zook.NewZKDriver([]string{"zook1:2181"}, time.Duration(2000)*time.Millisecond, "/solr")
	err := zkD.Open()
	if err != nil {
		return nil, err
	}
	log.Printf("created new locator")

	return &agent{
		driver:            zkD,
		enableHealthCheck: false,
	}, nil
}

func MakeCluster(clusterName string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       ptypes.DurationProto(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_EDS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		//LoadAssignment:       makeEndpoint(clusterName, UpstreamHost),
		DnsLookupFamily:  cluster.Cluster_V4_ONLY,
		EdsClusterConfig: makeEDSCluster(clusterName),
	}
}

func makeEDSCluster(alias string) *cluster.Cluster_EdsClusterConfig {
	return &cluster.Cluster_EdsClusterConfig{
		EdsConfig: makeConfigSource("xds_cluster"),
	}
}

func MakeEndpoint(clusterName string, eps []string) *endpoint.ClusterLoadAssignment {
	var endpoints []*endpoint.LbEndpoint

	for _, e := range eps {
		endpoints = append(endpoints, &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  e,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: 8983,
								},
							},
						},
					},
				},
			},
		})
	}

	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: endpoints,
		}},
	}
}

func MakeRoute(routes []string) *route.RouteConfiguration {
	var rts []*route.Route

	for _, r := range routes {
		rts = append(rts, &route.Route{
			//Name: r.Name,
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: "/solr/" + r,
				},
			},
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: r,
					},
				},
			},
		})
	}

	return &route.RouteConfiguration{
		Name: "listener_0",
		VirtualHosts: []*route.VirtualHost{{
			Name:    "local_service",
			Domains: []string{"*"},
			Routes:  rts,
		}},
	}
}

func MakeHTTPListener(listenerName, route, address string, port uint32) *listener.Listener {
	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource("xds_cluster"),
				RouteConfigName: route,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
	pbst, err := ptypes.MarshalAny(manager)
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  address,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}
}

func makeConfigSource(alias string) *core.ConfigSource {
	source := &core.ConfigSource{}
	source.ResourceApiVersion = resource.DefaultAPIVersion
	source.ConfigSourceSpecifier = &core.ConfigSource_ApiConfigSource{
		ApiConfigSource: &core.ApiConfigSource{
			TransportApiVersion:       resource.DefaultAPIVersion,
			ApiType:                   core.ApiConfigSource_GRPC,
			SetNodeOnFirstMessageOnly: true,
			GrpcServices: []*core.GrpcService{{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: alias},
				},
			}},
		},
	}
	return source
}
