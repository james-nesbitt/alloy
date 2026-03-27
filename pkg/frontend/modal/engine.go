package modal

// Mode represents the current state of the interaction model (e.g., Normal, Insert, Selection)
type Mode string

const (
	ModeNormal    Mode = "normal"
	ModeInsert    Mode = "insert"
	ModeSelection Mode = "selection"
	ModeCommand   Mode = "command"
)

// Intent is a high-level command derived from keyboard input.
// Specific intents (Move, Delete, UpdateMode) should implement this interface.
type Intent interface {
	Kind() string
}

// Key represents a physical or logical key press
type Key struct {
	Code  string // e.g., "j", "esc", "enter", "f1"
	Alt   bool
	Shift bool
	Ctrl  bool
}

func (k Key) String() string {
	s := ""
	if k.Ctrl {
		s += "ctrl+"
	}
	if k.Alt {
		s += "alt+"
	}
	if k.Shift && (len(k.Code) > 1 || k.Code == " ") {
		s += "shift+"
	}
	s += k.Code
	return s
}

// State represents the current internal state of a Modal Driver.
type State struct {
	Mode      Mode
	Register  string
	Count     int
	Pending   []Key
	Selection Selection
}

// Selection defines an active range of content or elements.
type Selection struct {
	Active bool
	Anchor int
	Cursor int
}

// Result represents the outcome of a key press handled by a driver.
type Result struct {
	Intent     Intent
	Consumed   bool
	Incomplete bool // True if the key is part of a multi-key sequence and waiting for more input
}

// Action represents a logical operation (e.g., "move:down", "mode:insert")
type Action string

// Driver is the core interface for a modal philosophy (Vim, Helix, Meow).
type Driver interface {
	Name() string
	// Handle processes a key event and returns a Result.
	Handle(key Key, state *State) Result
	// Modes returns the modes supported by this driver.
	Modes() []Mode
	// Customize allows overriding specific key bindings
	Customize(mode Mode, key string, action Action)
}

// Registry manages available modal drivers.
type Registry struct {
	drivers map[string]Driver
}

func NewRegistry() *Registry {
	return &Registry{
		drivers: make(map[string]Driver),
	}
}

func (r *Registry) Register(d Driver) {
	r.drivers[d.Name()] = d
}

func (r *Registry) Get(name string) Driver {
	return r.drivers[name]
}

// Global registry for convenience
var DefaultRegistry = NewRegistry()

func init() {
	// We'll register default drivers in their respective files
}

// Engine orchestrates multiple drivers and maintains the active modal state.
type Engine struct {
	ActiveDriver Driver
	State        State
}

func NewEngine(initial Driver) *Engine {
	return &Engine{
		ActiveDriver: initial,
		State: State{
			Mode: ModeNormal,
		},
	}
}

// Process transforms a raw key event into a project-level Intent.
func (e *Engine) Process(key Key) (Intent, bool) {
	result := e.ActiveDriver.Handle(key, &e.State)
	if result.Consumed {
		if result.Incomplete {
			e.State.Pending = append(e.State.Pending, key)
			return nil, true
		}

		// Sequence complete, clear pending
		e.State.Pending = nil

		if result.Intent != nil {
			e.State.Count = 0
			// Example: Internal mode switching intent handling
			if mi, ok := result.Intent.(ModeIntent); ok {
				e.State.Mode = mi.NewMode
			}
		}
	} else if len(e.State.Pending) > 0 {
		// If last was incomplete but current isn't consumed, we might need to reset or try to process as a sequence
		// This depends on the specific driver's philosophy (e.g., timeout in Vim or strict match)
		e.State.Pending = nil
	}

	return result.Intent, result.Consumed
}

// Standard Intents

type ModeIntent struct{ NewMode Mode }

func (m ModeIntent) Kind() string { return "mode-change" }

type MoveIntent struct {
	Direction string
	Count     int
}

func (m MoveIntent) Kind() string { return "movement" }

type ActionIntent struct {
	Verb      string
	Selection Selection
}

func (m ActionIntent) Kind() string { return "action" }

type WindowIntent struct {
	Action string // e.g., "split-v", "split-h", "close", "focus-left"
	Target string // e.g., "left", "right", "up", "down"
}

func (w WindowIntent) Kind() string { return "window" }

type BufferIntent struct {
	Action string // e.g., "save", "reload", "format", "fuzzy-find", "next", "prev"
	Path   string // optional path for specific buffer actions
}

func (b BufferIntent) Kind() string { return "buffer" }

type ProjectIntent struct {
	Action string // e.g., "build", "test", "deploy", "git-commit"
}

func (p ProjectIntent) Kind() string { return "project" }

type SearchIntent struct {
	Query string
	Type  string // e.g., "fuzzy", "regex", "symbol"
}

func (s SearchIntent) Kind() string { return "search" }

type InputIntent struct {
	Text string
}

func (i InputIntent) Kind() string { return "input" }

type LifecycleIntent struct {
	Action string // e.g., "blur", "focus", "close", "resize"
}

func (l LifecycleIntent) Kind() string { return "lifecycle" }
