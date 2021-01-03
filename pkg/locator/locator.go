package locator

import (
	"encoding/json"
	"strings"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/unbxd/go-base/base/drivers"
	"github.com/unbxd/go-base/base/drivers/zook"
)

type (
	Collection struct {
		Shards            map[string]*Shard `mapstructure:"shards"`
		ReplicationFactor string            `mapstructure:"replicationFactor"`
	}

	Shard struct {
		Name     string              `mapstructure:"name"`
		Range    string              `mapstructure:"range"`
		State    string              `mapstructure:"state"`
		Replicas map[string]*Replica `mapstructure:"replicas"`
	}

	Replica struct {
		Core     string `mapstructure:"core"`
		Leader   string `mapstructure:"leader"`
		BaseUrl  string `mapstructure:"base_url"`
		NodeName string `mapstructure:"node_name"`
		State    string `mapstructure:"state"`
	}
)

type Service interface {
	Clusters() ([]*cluster.Cluster, error)
	Routes() ([]*route.RouteConfiguration, error)
	CLA() ([]*endpoint.ClusterLoadAssignment, error)
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
		ep, err := a.getLbEndpoints(collection)
		if err != nil {
			return nil, err
		}
		cLAs = append(cLAs, &endpoint.ClusterLoadAssignment{
			ClusterName: alias,
			Endpoints: []*endpoint.LocalityLbEndpoints{{
				LbEndpoints: ep,
			}},
		})
	}
	return cLAs, nil
}

func (a *agent) getLbEndpoints(collection string) ([]*endpoint.LbEndpoint, error) {
	var hosts []*endpoint.LbEndpoint
	locations, err := a.getCollectionLocations(collection)
	if err != nil {
		return nil, err
	}
	for _, s := range locations {
		hosts = append(hosts, getLbEndpoint(s))
	}
	return hosts, nil
}

func getLbEndpoint(host string) *endpoint.LbEndpoint {
	return &endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.SocketAddress_TCP,
							Address:  host,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: 8983,
							},
						},
					},
				},
			},
		},
	}
}

func (a *agent) Clusters() ([]*cluster.Cluster, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var clusters []*cluster.Cluster
	for alias, collection := range aliasMap {
		ep, err := a.getLbEndpoints(collection)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, &cluster.Cluster{
			Name:                 alias,
			ConnectTimeout:       ptypes.DurationProto(5 * time.Second),
			ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_LOGICAL_DNS},
			LbPolicy:             cluster.Cluster_ROUND_ROBIN,
			LoadAssignment: &endpoint.ClusterLoadAssignment{
				ClusterName: alias,
				Endpoints: []*endpoint.LocalityLbEndpoints{
					LbEndpoints: ep,
				},
			},
			DnsLookupFamily: cluster.Cluster_V4_ONLY,
		})
	}
	return clusters, nil
}

func (a *agent) Routes() ([]*discovery.RouteConfiguration, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var routes []route.Route
	for alias := range aliasMap {
		routes = append(routes, route.Route{
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: "/",
				},
			}, Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: alias,
					},
				},
			},
		})
	}
	routeConfig := &discovery.RouteConfiguration{
		Name: "local_route",
		VirtualHosts: []route.VirtualHost{
			Name:    "local_service",
			Domains: []string{"*"},
			Routes:  routes,
		},
	}
	return []*discovery.RouteConfiguration{routeConfig}
}

func NewLocator() (Service, error) {
	zkD := zook.NewZKDriver([]string{"zook1:2181"}, time.Duration(2000)*time.Millisecond, "/solr")
	err := zkD.Open()
	if err != nil {
		return nil, err
	}
	return &agent{
		driver:            zkD,
		enableHealthCheck: false,
	}, nil
}
