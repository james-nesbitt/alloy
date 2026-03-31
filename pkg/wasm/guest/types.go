package guest

// Option/Result types inspired by WIT-bindgen's output
// but relocated here to be platform-independent.

type optionKind int

const (
	optionNone optionKind = iota
	optionSome
)

type Option[T any] struct {
	kind optionKind
	val  T
}

func (o Option[T]) IsNone() bool { return o.kind == optionNone }
func (o Option[T]) IsSome() bool { return o.kind == optionSome }
func (o Option[T]) Unwrap() T {
	if o.kind != optionSome {
		panic("Option is None")
	}
	return o.val
}
func (o *Option[T]) Set(val T) T {
	o.kind = optionSome
	o.val = val
	return val
}
func (o *Option[T]) Unset() { o.kind = optionNone }

func Some[T any](v T) Option[T] { return Option[T]{kind: optionSome, val: v} }
func None[T any]() Option[T]    { return Option[T]{kind: optionNone} }

type ResultKind int

const (
	resultOk ResultKind = iota
	resultErr
)

type Result[T any, E any] struct {
	kind      ResultKind
	resultOk  T
	resultErr E
}

func (r Result[T, E]) IsOk() bool  { return r.kind == resultOk }
func (r Result[T, E]) IsErr() bool { return r.kind == resultErr }
func (r Result[T, E]) Unwrap() T {
	if r.kind != resultOk {
		panic("Result is Err")
	}
	return r.resultOk
}
func (r Result[T, E]) UnwrapErr() E {
	if r.kind != resultErr {
		panic("Result is Ok")
	}
	return r.resultErr
}

func Ok[T any, E any](v T) Result[T, E]  { return Result[T, E]{kind: resultOk, resultOk: v} }
func Err[T any, E any](v E) Result[T, E] { return Result[T, E]{kind: resultErr, resultErr: v} }

// AlloyMessage is the standard message type for the SDK.
type AlloyMessage struct {
	Id        string
	MsgType   string
	Method    string
	Sender    string
	Actor     string
	Target    Option[string]
	Payload   []byte
	Timestamp uint64
	Metadata  []AlloyTuple2StringStringT
}

// Handler is a function that processes a message and returns an optional response.
type Handler func(msg AlloyMessage) *AlloyMessage

// AlloyHandler is a handler that uses raw message types.
type AlloyHandler func(msg AlloyMessage) AlloyMessage

// Tuple2StringString is a simple key-value pair used in WIT.
type AlloyTuple2StringStringT struct {
	F0 string
	F1 string
}

// AlloyCapability is a plugin's declared capability.
type AlloyCapability struct {
	Method      string
	Description string
	Shortcut    Option[string]
	Annotations Option[[]AlloyTuple2StringStringT]
}

// AlloyBuffer represents a direct data buffer.
type AlloyBuffer struct {
	Id           string
	Name         string
	Content      []byte
	LastModified uint64
	MimeType     string
}

// AlloyWidget represents a dashboard UI element.
type AlloyWidget struct {
	Id                string
	Title             string
	ContentType       string
	Content           []byte
	RefreshIntervalMs uint32
}

// Log levels
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Command is a high-level representation of a plugin command.
type Command struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Handler     CommandHandler     `json:"-"`
	Shortcut    string            `json:"shortcut,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// CommandContext provides context for command execution.
type CommandContext struct {
	Plugin *Plugin
	Args   []string
	Sender string
	Actor  string
}

// CommandHandler is a function that handles a command.
type CommandHandler func(ctx CommandContext) CommandResult

// CommandResult is the outcome of a command execution.
type CommandResult struct {
	Success bool            `json:"success"`
	Output  string          `json:"output"`
	Data    []byte          `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// HostInterface defines all interactions with the Alloy host.
type HostInterface interface {
	Init(id string, caps []AlloyCapability)
	Started()
	GetNextMessage() Option[AlloyMessage]
	SendResponse(msg AlloyMessage)
	Log(level string, msg string)
	Call(msg AlloyMessage) AlloyMessage
	RouteMessage(msg AlloyMessage)

	// KV
	KvSet(key string, val []byte) bool
	KvGet(key string) Option[[]byte]
	KvDelete(key string) bool
	KvList(prefix string) []string

	// Registry
	RegisterCapability(cap AlloyCapability)
	UnregisterCapability(method string)
	FindProviders(method, actor string, contextID string) []string
	GetAllCapabilities(actor string, contextID string) []AlloyCapability

	// Buffers
	ReadBuffer(id string) Option[AlloyBuffer]
	WriteBuffer(id string, content []byte) bool
	ListBuffers() []string
	GetBufferView(id string) (ptr, size uint32, ok bool)

	// Dashboard
	RegisterWidget(w AlloyWidget)
	UnregisterWidget(id string)
	UpdateWidget(id string, content []byte)
}
