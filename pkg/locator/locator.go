package locator

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpointv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	routev2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
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
	Clusters() ([]*discovery.Cluster, error)
	Routes() ([]*discovery.RouteConfiguration, error)
	CLA() ([]*discovery.ClusterLoadAssignment, error)
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
	log.Println(c)
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
	log.Println(locations)
	return locations, nil
}

func (a *agent) CLA() ([]*discovery.ClusterLoadAssignment, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var cLAs []*discovery.ClusterLoadAssignment
	for alias, collection := range aliasMap {
		ep, err := a.getLbEndpoints(collection)
		if err != nil {
			return nil, err
		}
		cLAs = append(cLAs, &discovery.ClusterLoadAssignment{
			ClusterName: alias,
			Policy:      &discovery.ClusterLoadAssignment_Policy{},
			Endpoints: []*endpointv2.LocalityLbEndpoints{{
				Locality:    a.Locality(),
				LbEndpoints: ep,
			}},
		})
	}
	return cLAs, nil
}

//Locality translates Agent info to envoy control plane locality
func (a *agent) Locality() *core.Locality {
	return &core.Locality{
		Region: "test",
	}
}

func (a *agent) Clusters() ([]*discovery.Cluster, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var clusters []*discovery.Cluster
	for alias := range aliasMap {
		clusters = append(clusters, &discovery.Cluster{
			Name:              alias,
			ProtocolSelection: discovery.Cluster_USE_DOWNSTREAM_PROTOCOL,
			EdsClusterConfig: &discovery.Cluster_EdsClusterConfig{
				EdsConfig: &core.ConfigSource{
					ConfigSourceSpecifier: &core.ConfigSource_Ads{
						Ads: &core.AggregatedConfigSource{},
					},
				},
			},
		})

	}
	return clusters, nil
}

func (a *agent) Routes() ([]*discovery.RouteConfiguration, error) {
	aliasMap, err := a.getAliasMap()
	if err != nil {
		return nil, err
	}
	var routes []*routev2.Route
	for alias := range aliasMap {
		routes = append(routes, &routev2.Route{
			Match: &routev2.RouteMatch{
				PathSpecifier: &routev2.RouteMatch_Prefix{
					Prefix: "/",
				},
			}, Action: &routev2.Route_Route{
				Route: &routev2.RouteAction{
					ClusterSpecifier: &routev2.RouteAction_Cluster{
						Cluster: alias,
					},
				},
			},
		})
	}
	routeConfig := &discovery.RouteConfiguration{
		Name: "local_route",
		VirtualHosts: []*routev2.VirtualHost{{
			Name:    "local_service",
			Domains: []string{"*"},
			Routes:  routes,
		}},
	}
	return []*discovery.RouteConfiguration{routeConfig}, nil
}

func (a *agent) getLbEndpoints(collection string) ([]*endpointv2.LbEndpoint, error) {
	var hosts []*endpointv2.LbEndpoint
	locations, err := a.getCollectionLocations(collection)
	if err != nil {
		return nil, err
	}
	for _, s := range locations {
		hosts = append(hosts, getLbEndpoint(s))
	}
	return hosts, nil
}

func getLbEndpoint(host string) *endpointv2.LbEndpoint {
	return &endpointv2.LbEndpoint{
		HealthStatus: core.HealthStatus_HEALTHY,
		HostIdentifier: &endpointv2.LbEndpoint_Endpoint{
			Endpoint: &endpointv2.Endpoint{Address: &core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						Protocol: core.SocketAddress_TCP,
						Address:  host,
						PortSpecifier: &core.SocketAddress_PortValue{
							PortValue: uint32(8983),
						},
					},
				},
			}}}}
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
