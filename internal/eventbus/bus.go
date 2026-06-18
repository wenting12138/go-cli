package eventbus

import "sync"

type Bus struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan any
}

func New() *Bus {
	return &Bus{
		subscribers: make(map[int]chan any),
	}
}

func (b *Bus) Subscribe(buffer int) (<-chan any, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++

	ch := make(chan any, buffer)
	b.subscribers[id] = ch

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
	}
}

func (b *Bus) Publish(event any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		ch <- event
	}
}
