package events

import (
	"context"
	"errors"
	"sync"
	"time"

	"myagent/internal/siu"
)

var ErrSchedulerClosed = errors.New("scheduler closed")

type Handler interface {
	Handle(context.Context, *siu.Event) error
}

type Scheduler struct {
	ctx         context.Context
	cancel      context.CancelFunc
	handler     Handler
	idleTimeout time.Duration
	workers     chan struct{}
	pending     chan struct{}

	mu     sync.Mutex
	lanes  map[string]*lane
	closed bool
	wg     sync.WaitGroup
}

type lane struct {
	key   string
	queue []*siu.Event
	wake  chan struct{}
}

func NewScheduler(parent context.Context, handler Handler, maxConcurrent, maxPending int, idleTimeout time.Duration) *Scheduler {
	ctx, cancel := context.WithCancel(parent)
	return &Scheduler{
		ctx: ctx, cancel: cancel, handler: handler, idleTimeout: idleTimeout,
		workers: make(chan struct{}, maxConcurrent), pending: make(chan struct{}, maxPending),
		lanes: make(map[string]*lane),
	}
}

func (s *Scheduler) Submit(ctx context.Context, topicID string, event *siu.Event) error {
	select {
	case s.pending <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return ErrSchedulerClosed
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.pending
		return ErrSchedulerClosed
	}
	l := s.lanes[topicID]
	if l == nil {
		l = &lane{key: topicID, wake: make(chan struct{}, 1)}
		s.lanes[topicID] = l
		s.wg.Add(1)
		go s.runLane(l)
	}
	l.queue = append(l.queue, event)
	s.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Scheduler) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) runLane(l *lane) {
	defer s.wg.Done()
	timer := time.NewTimer(s.idleTimeout)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		s.mu.Lock()
		hasWork := len(l.queue) > 0
		s.mu.Unlock()
		if !hasWork {
			timer.Reset(s.idleTimeout)
			select {
			case <-l.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			case <-timer.C:
				s.mu.Lock()
				if len(l.queue) == 0 && s.lanes[l.key] == l {
					delete(s.lanes, l.key)
					s.mu.Unlock()
					return
				}
				s.mu.Unlock()
				continue
			case <-s.ctx.Done():
				return
			}
		}

		select {
		case s.workers <- struct{}{}:
		case <-s.ctx.Done():
			return
		}

		s.mu.Lock()
		event := l.queue[0]
		l.queue[0] = nil
		l.queue = l.queue[1:]
		s.mu.Unlock()
		<-s.pending

		_ = s.handler.Handle(s.ctx, event)
		<-s.workers
	}
}
