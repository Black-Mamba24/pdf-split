package measure

import (
	"container/list"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Black-Mamba24/pdf-split/internal/domain"
	"github.com/Black-Mamba24/pdf-split/internal/pdf"
)

type Measurer interface {
	Measure(ctx context.Context, pages domain.PageRange) (int64, error)
	Measurements() int
}

type cacheKey struct {
	input string
	start int
	end   int
}

type cacheEntry struct {
	key  cacheKey
	size int64
}

type measurer struct {
	engine         pdf.Engine
	input          string
	canonicalInput string
	tempDir        string
	cacheEntries   int

	mu           sync.Mutex
	measurements int
	lru          *list.List
	cache        map[cacheKey]*list.Element
}

func New(engine pdf.Engine, input, tempDir string, cacheEntries int) Measurer {
	canonicalInput, err := filepath.Abs(input)
	if err != nil {
		canonicalInput = input
	}

	return &measurer{
		engine:         engine,
		input:          input,
		canonicalInput: canonicalInput,
		tempDir:        tempDir,
		cacheEntries:   cacheEntries,
		lru:            list.New(),
		cache:          make(map[cacheKey]*list.Element),
	}
}

func (m *measurer) Measure(ctx context.Context, pages domain.PageRange) (int64, error) {
	key := cacheKey{input: m.canonicalInput, start: pages.Start, end: pages.End}
	if size, ok := m.lookup(key); ok {
		return size, nil
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	candidate, err := os.CreateTemp(m.tempDir, "pdf-split-measure-*.pdf")
	if err != nil {
		return 0, fmt.Errorf("create measurement candidate: %w", err)
	}
	candidatePath := candidate.Name()
	if err := candidate.Close(); err != nil {
		_ = os.Remove(candidatePath)
		return 0, fmt.Errorf("close measurement candidate: %w", err)
	}
	if err := os.Remove(candidatePath); err != nil {
		return 0, fmt.Errorf("prepare measurement candidate: %w", err)
	}
	defer os.Remove(candidatePath)

	m.recordMeasurement()
	if err := m.engine.WriteRange(m.input, candidatePath, pages); err != nil {
		return 0, err
	}

	info, err := os.Stat(candidatePath)
	if err != nil {
		return 0, fmt.Errorf("stat measurement candidate: %w", err)
	}
	size := info.Size()
	m.store(key, size)
	return size, nil
}

func (m *measurer) Measurements() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.measurements
}

func (m *measurer) lookup(key cacheKey) (int64, bool) {
	if m.cacheEntries <= 0 {
		return 0, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	element, ok := m.cache[key]
	if !ok {
		return 0, false
	}
	m.lru.MoveToFront(element)
	return element.Value.(cacheEntry).size, true
}

func (m *measurer) store(key cacheKey, size int64) {
	if m.cacheEntries <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if element, ok := m.cache[key]; ok {
		element.Value = cacheEntry{key: key, size: size}
		m.lru.MoveToFront(element)
		return
	}

	element := m.lru.PushFront(cacheEntry{key: key, size: size})
	m.cache[key] = element
	for m.lru.Len() > m.cacheEntries {
		oldest := m.lru.Back()
		if oldest == nil {
			return
		}
		m.lru.Remove(oldest)
		delete(m.cache, oldest.Value.(cacheEntry).key)
	}
}

func (m *measurer) recordMeasurement() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.measurements++
}
