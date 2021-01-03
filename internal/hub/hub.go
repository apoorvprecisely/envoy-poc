package hub

import (
	"log"
	"sync"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	uuid "github.com/satori/go.uuid"
)

type Service interface {
	Subscribe() *Subscription
	Publish(event *Event)
	Size() int
}

type CLAChan chan *discovery.ClusterLoadAssignment
type EventChan chan *Event

type Event struct {
	CLA      []*discovery.ClusterLoadAssignment
	Clusters []*discovery.Cluster
	Routes   []*discovery.RouteConfiguration
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
	log.Println("incoming event")
	h.subscriptions.Range(func(id, subscription interface{}) bool {
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
