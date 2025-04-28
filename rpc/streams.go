package rpc

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

type StreamSubscription struct {
	closeCh      chan void
	eventsBitmap uint32
}

func NewStreamSubscription(closeCh chan void, eventsBitmap uint32) *StreamSubscription {
	return &StreamSubscription{
		closeCh:      closeCh,
		eventsBitmap: eventsBitmap,
	}
}

//

type Stream[Channel comparable, Event any] struct {
	mu                     *sync.Mutex
	subscriptionsByChannel map[Channel]map[chan<- Event]*StreamSubscription
	subscriptionsGlobal    map[chan<- Event]*StreamSubscription
	source                 <-chan Event
	identify               func(Event) Channel
	event                  func(Event) uint32
	name                   string
}

func (s *Stream[Channel, Event]) ClientPump(clientCh chan Event, sub *StreamSubscription, send func(Event) error) error {
	var err error
	for {
		select {
		case q, ok := <-clientCh:
			if !ok {
				return nil
			}
			err = send(q)
			if err != nil {
				return err
			}
		case <-sub.closeCh:
			return nil
		}
	}
}

func (s *Stream[Channel, Event]) broadcast(m Event) {
	key := s.identify(m)
	log.Debug().
		Str("stream_name", s.name).
		Str("bucket", fmt.Sprintf("%v", key)).
		Str("payload", fmt.Sprintf("%v", m)).
		Msg("broadcasting message")

	s.mu.Lock()
	defer s.mu.Unlock()

	if bucket, ok := s.subscriptionsByChannel[key]; ok {
		for clientCh, sub := range bucket {
			s.send(sub, clientCh, m, key)
		}
	}
	for clientCh, sub := range s.subscriptionsGlobal {
		s.send(sub, clientCh, m, key)
	}
}

func (s *Stream[Channel, Event]) send(sub *StreamSubscription, clientCh chan<- Event, m Event, channel Channel) {
	eventMatch := sub.eventsBitmap == 0 || (sub.eventsBitmap&s.event(m) != 0)
	if !eventMatch {
		return
	}

	select {
	case clientCh <- m:
	default:
		select {
		case sub.closeCh <- void{}:
			log.Warn().
				Str("stream_name", s.name).
				Any("channel", channel).
				Str("client", fmt.Sprintf("%p", clientCh)).
				Msgf("failed to write %s to client, queue is full, disconnecting client", s.name)
		default: // already closing
		}
	}
}

func (s *Stream[Channel, Event]) Pump() {
	for message := range s.source {
		s.broadcast(message)
	}
}

func (s *Stream[Channel, Event]) Subscribe(clientCh chan<- Event, sub *StreamSubscription, channels ...Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(channels) == 0 {
		s.subscriptionsGlobal[clientCh] = sub
		return
	}
	for _, id := range channels {
		bucket, ok := s.subscriptionsByChannel[id]
		if !ok {
			bucket = make(map[chan<- Event]*StreamSubscription)
			s.subscriptionsByChannel[id] = bucket
		}
		bucket[clientCh] = sub
	}
}

func (s *Stream[Channel, Event]) Unsubscribe(clientCh chan Event, channels ...Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(channels) == 0 {
		delete(s.subscriptionsGlobal, clientCh)
		return
	}

	for _, id := range channels {
		if bucket, ok := s.subscriptionsByChannel[id]; ok {
			delete(bucket, clientCh)
			if len(bucket) == 0 {
				delete(s.subscriptionsByChannel, id)
			}
		}
	}
}

// NewStream creates a gRPC stream wrapper for server which introduces pubsub semantics to the stream.
func NewStream[Channel comparable, Event any](
	name string,
	source <-chan Event,
	identify func(Event) Channel,
	event func(Event) uint32,
) *Stream[Channel, Event] {
	return &Stream[Channel, Event]{
		mu:                     &sync.Mutex{},
		name:                   name,
		subscriptionsByChannel: make(map[Channel]map[chan<- Event]*StreamSubscription),
		subscriptionsGlobal:    make(map[chan<- Event]*StreamSubscription),
		source:                 source,
		identify:               identify,
		event:                  event,
	}
}
