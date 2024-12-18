package rpc

import (
	"context"
	"io"
	"sync"
	"time"

	"unsafe"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

var (
	EventStreamClientSubscribeTimeout   = 5 * time.Second
	EventStreamClientUnsubscribeTimeout = 5 * time.Second
)

type EventStreamClient struct {
	mu       sync.Mutex
	ctx      context.Context
	stream   EventService_StreamClient
	handlers map[EventType][]func(*Event)
}

func (s *EventStreamClient) send(req *StreamEventRequest) error {
	err := s.stream.Send(req)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil
		}
		log.Error().Err(err).Msg("failed to send event to the stream")
	}

	return nil
}

func (s *EventStreamClient) SendSubscribe(reqs ...*StreamEventSubscriptionRequest) ([]*EventPayloadSubscribed, error) {
	awaiting := len(reqs)
	ch := make(chan *Event, awaiting)
	res := make([]*EventPayloadSubscribed, awaiting)
	s.AddHandlerN(EventType_EVENT_TYPE_SUBSCRIBED, awaiting, func(ev *Event) {
		ch <- ev
	})
	err := s.send(&StreamEventRequest{Subscribe: reqs})
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-time.After(EventStreamClientSubscribeTimeout):
			return nil, errors.Errorf(
				"timed out waiting for %s for subscribed event",
				EventStreamClientSubscribeTimeout,
			)
		case ev := <-ch:
			awaiting--
			res[awaiting] = ev.GetSubscribed()
			if awaiting <= 0 {
				return res, nil
			}
		}
	}
}

func (s *EventStreamClient) SendUnsubscribe(reqs ...*StreamEventUnsubscriptionRequest) ([]*EventPayloadUnsubscribed, error) {
	awaiting := len(reqs)
	ch := make(chan *Event, awaiting)
	res := make([]*EventPayloadUnsubscribed, awaiting)
	s.AddHandlerN(EventType_EVENT_TYPE_UNSUBSCRIBED, awaiting, func(ev *Event) {
		ch <- ev
	})
	err := s.send(&StreamEventRequest{Unsubscribe: reqs})
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-time.After(EventStreamClientUnsubscribeTimeout):
			return nil, errors.Errorf(
				"timed out waiting for %s for unsubscribed event",
				EventStreamClientUnsubscribeTimeout,
			)
		case ev := <-ch:
			awaiting--
			res[awaiting] = ev.GetUnsubscribed()
			if awaiting <= 0 {
				return res, nil
			}
		}
	}
}

func (s *EventStreamClient) AddHandler(et EventType, f func(*Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[et] = append(s.handlers[et], f)
}

func (s *EventStreamClient) RemoveHandler(er EventType, f func(*Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.handlers[er]
	for n := len(bucket) - 1; n >= 0; n-- { // loop from the end to preserve stack-like semantics for multiple f instances
		v := bucket[n]
		if *(*unsafe.Pointer)(unsafe.Pointer(&v)) == *(*unsafe.Pointer)(unsafe.Pointer(&f)) {
			bucket = append(bucket[:n], bucket[n+1:]...)
			break
		}
	}
	s.handlers[er] = bucket
}

func (s *EventStreamClient) AddHandlerN(et EventType, n int, f func(*Event)) {
	var wrapper func(*Event)
	wrapper = func(ev *Event) {
		n--
		if n == 0 {
			s.RemoveHandler(et, wrapper)
		}
		f(ev)
	}
	s.AddHandler(et, wrapper)
}

func (s *EventStreamClient) dispatch(ev *Event) []func(*Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debug().
		Str("event", ev.String()).
		Msg("event stream client dispatching event")

	res := []func(*Event){}
	res = append(res, s.handlers[ev.Type]...)
	return res
}

func (s *EventStreamClient) pump() {
	var (
		ev  *Event
		err error
	)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			ev, err = s.stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					return
				}
				log.Error().Err(err).Msg("failed to receive event from the stream, closing recv pump")
				return
			}

			handlers := s.dispatch(ev)
			for _, handler := range handlers {
				handler(ev)
			}
		}
	}
}

func NewEventStreamClient(ctx context.Context, cl *Client) (*EventStreamClient, error) {
	stream, err := cl.Event.Stream(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to rpc event stream")
	}
	s := &EventStreamClient{
		ctx:      ctx,
		stream:   stream,
		handlers: map[EventType][]func(*Event){},
	}
	go s.pump()

	//

	s.AddHandler(EventType_EVENT_TYPE_ERROR, func(ev *Event) {
		log.Error().
			Str("event", ev.String()).
			Msg("event stream client got an error from server")
	})

	//

	_, err = s.SendSubscribe(&StreamEventSubscriptionRequest{
		Type: EventType_EVENT_TYPE_HEARTBEAT,
	})
	if err != nil {
		return nil, err
	}

	//

	return s, nil
}
