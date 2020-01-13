package eventlogger

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Broker
type Broker struct {
	graphs     map[EventType]*graph
	graphMutex sync.RWMutex

	*clock
}

func NewBroker() *Broker {
	return &Broker{
		graphs: make(map[EventType]*graph),
	}
}

// clock only exists to make testing simpler.
type clock struct {
	now time.Time
}

func (c *clock) Now() time.Time {
	if c == nil {
		return time.Now()
	}
	return c.now
}

func (b *Broker) Validate() error {
	b.graphMutex.RLock()
	defer b.graphMutex.RUnlock()

	if len(b.graphs) == 0 {
		return fmt.Errorf("no graphs in broker")
	}

	for _, g := range b.graphs {
		if err := g.validate(); err != nil {
			return err
		}
	}

	return nil
}

type Status struct {
	SentToSinks []string
	Warnings    []error
}

func (s Status) GetError(threshold int) error {
	if len(s.SentToSinks) < threshold {
		return fmt.Errorf("event not written to enough sinks")
	}
	return nil
}

func (b *Broker) Send(ctx context.Context, t EventType, payload interface{}) (Status, error) {

	b.graphMutex.RLock()
	g, ok := b.graphs[t]
	b.graphMutex.RUnlock()

	if !ok {
		return Status{}, fmt.Errorf("No graph for EventType %s", t)
	}

	e := &Event{
		Type:      t,
		CreatedAt: b.clock.Now(),
		Formatted: make(map[string][]byte),
		Payload:   payload,
	}

	return g.process(ctx, e)
}

func (b *Broker) Reopen(ctx context.Context) error {
	b.graphMutex.RLock()
	defer b.graphMutex.RUnlock()

	for _, g := range b.graphs {
		if err := g.reopen(ctx); err != nil {
			return err
		}
	}

	return nil
}

// PipelineID is a string that uniquely identifies a Pipeline within a given EventType.
type PipelineID string

// RegisterPipeline adds a pipeline to the broker.
func (b *Broker) RegisterPipeline(t EventType, id PipelineID, root Node) error {
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	g, ok := b.graphs[t]
	if !ok {
		g = &graph{roots: make(map[PipelineID]Node)}
		b.graphs[t] = g
	}

	err := g.doValidate(nil, root)
	if err != nil {
		return err
	}

	g.roots[id] = root

	return nil
}

// RemovePipeline removes a pipeline from the broker.
func (b *Broker) RemovePipeline(t EventType, id PipelineID) error {
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	g, ok := b.graphs[t]
	if !ok {
		return fmt.Errorf("No graph for EventType %s", t)
	}

	delete(g.roots, id)
	return nil
}

// SetSuccessThreshold sets the succes threshold per eventType.  For the
// overall processing of a given event to be considered a success, at least as
// many sinks as the threshold value must successfully process the event.
func (b *Broker) SetSuccessThreshold(t EventType, successThreshold int) error {
	b.graphMutex.Lock()
	defer b.graphMutex.Unlock()

	if successThreshold < 0 {
		return fmt.Errorf("successThreshold must be 0 or greater")
	}

	g, ok := b.graphs[t]
	if !ok {
		g = &graph{roots: make(map[PipelineID]Node)}
		b.graphs[t] = g
	}

	g.successThreshold = successThreshold
	return nil
}
