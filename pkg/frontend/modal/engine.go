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
	Code string // e.g., "j", "esc", "enter", "ctrl+c"
	Alt  bool
	Shift bool
	Ctrl  bool
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

// Driver is the core interface for a modal philosophy (Vim, Helix, Meow).
type Driver interface {
	Name() string
	// Handle processes a key event and returns an Intent and whether the event was consumed.
	Handle(key Key, state *State) (Intent, bool)
	// Modes returns the modes supported by this driver.
	Modes() []Mode
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
	intent, consumed := e.ActiveDriver.Handle(key, &e.State)
	if consumed && intent != nil {
		// Example: Internal mode switching intent handling
		if mi, ok := intent.(ModeIntent); ok {
			e.State.Mode = mi.NewMode
		}
	}
	return intent, consumed
}

// Standard Intents

type ModeIntent struct { NewMode Mode }
func (m ModeIntent) Kind() string { return "mode-change" }

type MoveIntent struct { Direction string; Count int }
func (m MoveIntent) Kind() string { return "movement" }

type ActionIntent struct { Verb string; Selection Selection }
func (m ActionIntent) Kind() string { return "action" }
