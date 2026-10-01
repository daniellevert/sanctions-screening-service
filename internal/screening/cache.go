package screening

import (
	"container/list"
	"sync"
)

type candidate struct {
	UID          string
	MatchedName  string
	NameCategory string
	Score        float32
	Exact        bool
}

type cacheEntry struct {
	key   string
	value []candidate
}

type candidateCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

func newCandidateCache(capacity int) *candidateCache {
	return &candidateCache{capacity: capacity, items: make(map[string]*list.Element), order: list.New()}
}

func cloneCandidates(value []candidate) []candidate {
	return append([]candidate(nil), value...)
}

func (c *candidateCache) Get(key string) ([]candidate, bool) {
	if c.capacity == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return cloneCandidates(element.Value.(cacheEntry).value), true
}

func (c *candidateCache) Put(key string, value []candidate) {
	if c.capacity == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		element.Value = cacheEntry{key: key, value: cloneCandidates(value)}
		c.order.MoveToFront(element)
		return
	}
	element := c.order.PushFront(cacheEntry{key: key, value: cloneCandidates(value)})
	c.items[key] = element
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		delete(c.items, oldest.Value.(cacheEntry).key)
		c.order.Remove(oldest)
	}
}
