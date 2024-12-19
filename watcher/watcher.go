package watcher

import (
	"context"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

type WatcherCallback func(ev *fsnotify.Event)
type WatcherCallbackWrapper func(next WatcherCallback) WatcherCallback
type WatcherFilter func(ev *fsnotify.Event) bool

func WithWatcherModifyFilter() WatcherFilter {
	return func(ev *fsnotify.Event) bool {
		return ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create)
	}
}

func WithWatcherCallbackDebounce(dur time.Duration) WatcherCallbackWrapper {
	return func(next WatcherCallback) WatcherCallback {
		contexts := map[string]struct {
			ctx    context.Context
			cancel context.CancelFunc
		}{}
		return func(ev *fsnotify.Event) {
			if c, ok := contexts[ev.Name]; ok && c.cancel != nil {
				c.cancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			contexts[ev.Name] = struct {
				ctx    context.Context
				cancel context.CancelFunc
			}{ctx: ctx, cancel: cancel}
			go func(ctx context.Context) {
				select {
				case <-ctx.Done():
				case <-time.After(dur):
					next(ev)
				}
			}(ctx)
		}
	}
}

type watcherWatch struct {
	callback WatcherCallback
	filters  []WatcherFilter
}
type Watcher struct {
	mu      sync.Mutex
	notify  *fsnotify.Watcher
	names   map[string][]watcherWatch
	watches map[string][]watcherWatch
}

func (w *Watcher) Watch(name string, cb WatcherCallback, filters ...WatcherFilter) error {
	absName, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	absDir := filepath.Dir(absName)

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.watches[absDir]; !ok {
		err := w.notify.Add(absDir)
		if err != nil {
			return err
		}
	}

	w.watches[absDir] = append(w.watches[absDir], watcherWatch{
		callback: cb,
		filters:  filters,
	})
	w.names[absName] = w.watches[absDir]
	return nil
}

func (w *Watcher) Unwatch(name string, cb WatcherCallback) error {
	absName, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	absDir := filepath.Dir(absName)

	cbptr := *(*unsafe.Pointer)(unsafe.Pointer(&cb))

	w.mu.Lock()
	defer w.mu.Unlock()
	bucket, ok := w.watches[absDir]
	if ok {
		for n, w := range bucket {
			if *(*unsafe.Pointer)(unsafe.Pointer(&w.callback)) == cbptr {
				bucket = append(bucket[:n], bucket[n+1:]...)
				break
			}
		}
		if len(bucket) == 0 {
			err := w.notify.Remove(absDir)
			if err != nil {
				return err
			}
			delete(w.watches, absDir)
			names := []string{}
			bucketptr := *(*unsafe.Pointer)(unsafe.Pointer(&bucket))
			for name, nameBucket := range w.names {
				if *(*unsafe.Pointer)(unsafe.Pointer(&nameBucket)) == bucketptr {
					names = append(names, name)
				}
			}
			for _, name := range names {
				delete(w.names, name)
			}
		} else {
			delete(w.names, absName)
		}
	}

	return nil
}

func (w *Watcher) emit(ev *fsnotify.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	bucket, ok := w.names[ev.Name]
	if ok {
	loop:
		for _, watch := range bucket {
			if watch.filters != nil {
				for _, filter := range watch.filters {
					if !filter(ev) {
						continue loop
					}
				}
			}
			watch.callback(ev)
		}
	}
}

func (w *Watcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.notify.Events:
			if !ok {
				return
			}
			w.emit(&event)
		}
	}
}

func New() (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		notify:  w,
		watches: map[string][]watcherWatch{},
		names:   map[string][]watcherWatch{},
	}, nil
}

//

type MultiWatcher struct {
	mu              sync.Mutex
	watcher         *Watcher
	names           map[string]bool
	filters         []WatcherFilter
	callback        func()
	watcherCallback WatcherCallback
}

type MultiWatcherOption any

func NewMulti(w *Watcher, names []string, callback func(), opts ...MultiWatcherOption) (*MultiWatcher, error) {
	mw := &MultiWatcher{
		watcher:  w,
		names:    make(map[string]bool),
		callback: callback,
	}
	mw.watcherCallback = mw.eventCallback

	for _, opt := range opts {
		switch v := opt.(type) {
		case WatcherFilter:
			mw.filters = append(mw.filters, v)
		case WatcherCallbackWrapper:
			mw.watcherCallback = v(mw.watcherCallback)
		default:
			return nil, errors.Errorf("unsupported option type %T", opt)
		}
	}

	for _, name := range names {
		absName, err := filepath.Abs(name)
		if err != nil {
			return nil, err
		}
		mw.names[absName] = false
	}

	return mw, nil
}

func (m *MultiWatcher) eventCallback(event *fsnotify.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.names[event.Name] = true
	if m.all() {
		m.callback()
		m.reset()
	}
}

func (m *MultiWatcher) all() bool {
	for _, modified := range m.names {
		if !modified {
			return false
		}
	}
	return true
}

func (m *MultiWatcher) reset() {
	for file := range m.names {
		m.names[file] = false
	}
}

func (m *MultiWatcher) Watch() error {
	for name := range m.names {
		err := m.watcher.Watch(name, m.watcherCallback, m.filters...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MultiWatcher) Unwatch() error {
	for file := range m.names {
		err := m.watcher.Unwatch(file, m.watcherCallback)
		if err != nil {
			return err
		}
	}
	return nil
}
