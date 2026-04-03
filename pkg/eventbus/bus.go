package eventbus

import (
	"sync"
	"time"
)

type EventType string

const (
	TypeTaskCompleted EventType = "task.completed"
	TypeToolCalled    EventType = "tool.called"
	TypeMemoryStored  EventType = "memory.stored"
	TypeSkillCreated  EventType = "skill.created"
	TypeJobCompleted  EventType = "job.completed"
	TypeError         EventType = "error"
	TypeProactive     EventType = "proactive.suggestion"
	TypeSessionStart  EventType = "session.start"
	TypeSessionEnd    EventType = "session.end"
)

type Event interface {
	Type() EventType
	TS() time.Time
}

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
	closed   bool
}

func New() *Bus {
	return &Bus{
		handlers: make(map[EventType][]Handler),
	}
}

func (b *Bus) Subscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *Bus) Unsubscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	handlers := b.handlers[eventType]
	for i, h := range handlers {
		if &h == &handler {
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			return
		}
	}
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	handlers := b.handlers[event.Type()]
	b.mu.RUnlock()

	for _, h := range handlers {
		go h(event)
	}
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
}

type TaskCompleted struct {
	TaskID    string
	Success   bool
	Duration  time.Duration
	Error     string
	Timestamp time.Time
}

func (e TaskCompleted) Type() EventType { return TypeTaskCompleted }
func (e TaskCompleted) TS() time.Time   { return e.Timestamp }

type ToolCalled struct {
	ToolName  string
	Args      map[string]any
	Success   bool
	Duration  time.Duration
	Timestamp time.Time
}

func (e ToolCalled) Type() EventType { return TypeToolCalled }
func (e ToolCalled) TS() time.Time   { return e.Timestamp }

type MemoryStored struct {
	Key       string
	Trust     string
	Timestamp time.Time
}

func (e MemoryStored) Type() EventType { return TypeMemoryStored }
func (e MemoryStored) TS() time.Time   { return e.Timestamp }

type SkillCreated struct {
	SkillName string
	Timestamp time.Time
}

func (e SkillCreated) Type() EventType { return TypeSkillCreated }
func (e SkillCreated) TS() time.Time   { return e.Timestamp }

type JobCompleted struct {
	JobID     string
	Success   bool
	Timestamp time.Time
}

func (e JobCompleted) Type() EventType { return TypeJobCompleted }
func (e JobCompleted) TS() time.Time   { return e.Timestamp }

type ErrorEvent struct {
	Component string
	Message   string
	Timestamp time.Time
}

func (e ErrorEvent) Type() EventType { return TypeError }
func (e ErrorEvent) TS() time.Time   { return e.Timestamp }

type ProactiveSuggestion struct {
	Suggestion string
	Reason     string
	Confidence float64
	Timestamp  time.Time
}

func (e ProactiveSuggestion) Type() EventType { return TypeProactive }
func (e ProactiveSuggestion) TS() time.Time   { return e.Timestamp }

type SessionStart struct {
	SessionID string
	Source    string
	Timestamp time.Time
}

func (e SessionStart) Type() EventType { return TypeSessionStart }
func (e SessionStart) TS() time.Time   { return e.Timestamp }

type SessionEnd struct {
	SessionID string
	Reason    string
	Timestamp time.Time
}

func (e SessionEnd) Type() EventType { return TypeSessionEnd }
func (e SessionEnd) TS() time.Time   { return e.Timestamp }
