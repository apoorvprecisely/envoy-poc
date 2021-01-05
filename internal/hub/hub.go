package hub

import (
	"log"
	"sync"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	uuid "github.com/satori/go.uuid"
)

type Service interface {
	Subscribe() *Subscription
	Publish(event *Event)
	Size() int
}

type EventChan chan *Event

type Event struct {
	CLA       []*endpoint.ClusterLoadAssignment
	Clusters  []*cluster.Cluster
	Routes    []*route.RouteConfiguration
	Listeners []*listener.Listener
}

type hub struct {
	subscriptions sync.Map
}

type Subscription struct {
	ID      uuid.UUID
	Events  EventChan
	OnClose func(uuid.UUID)
}

func (s Subscription) Accept(e *Event) {
	select {
	case s.Events <- e:
	}
}

func (s Subscription) Close() {
	s.OnClose(s.ID)
	close(s.Events)
}

func (h *hub) Subscribe() *Subscription {
	id := uuid.NewV4()
	subs := &Subscription{ID: id, Events: make(EventChan, 1000), OnClose: func(subID uuid.UUID) {
		h.subscriptions.Delete(subID)
	}}
	h.subscriptions.Store(id, subs)

	return subs
}

func (h *hub) Publish(event *Event) {
	h.subscriptions.Range(func(id, subscription interface{}) bool {
		log.Println("incoming event, forwarded to subscriber")
		subscription.(*Subscription).Accept(event)
		return true
	})
}

func (h *hub) Size() int {
	var ids []interface{}
	h.subscriptions.Range(func(id, subscription interface{}) bool {
		ids = append(ids, id)
		return true
	})
	return len(ids)
}

func NewHub() Service {
	return &hub{}
}
