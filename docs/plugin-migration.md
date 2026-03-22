# Plugin Migration and Standardization

## Overview
This document summarizes the migration of plugins to the WIT-based system and the standardization of plugin naming and structure.

## Plugin Name Standardization

As part of the migration, plugin names have been standardized to remove redundant prefixes and make the naming more consistent:

| Old Name | New Name | Description |
|----------|----------|-------------|
| plugin-chat | chat | Chat functionality |
| plugin-project-manager | project | Project management |
| health-wasm | health | Health monitoring |
| buffer-manager | buffer | Buffer management |
| ai-agent | ai | AI capabilities |
| iam | iam | Identity and access management |
| secrets | secrets | Secrets management |
| tasks | tasks | Task management |

## Migration Process

### 1. Plugin Structure

Each plugin now has a standardized structure:

```
<plugin-name>/
├── main.go         # Legacy implementation (optional)
├── main_wit.go     # WIT-based implementation
└── wit/            # WIT bindings
```

### 2. Migration Steps

1. **Create WIT Version**: Created `main_wit.go` alongside existing `main.go`
2. **Standardize Naming**: Removed redundant prefixes from plugin names
3. **Enhance Metadata**: Added comprehensive plugin metadata
4. **Improve Error Handling**: Added better error handling and logging
5. **Add Type Safety**: Used proper Go types for all data structures
6. **Standardize API**: Created consistent API patterns across plugins

### 3. Build System Integration

The build system was updated to:

1. Automatically detect WIT versions (`main_wit.go`)
2. Use TinyGo for better WASM support
3. Support standardized plugin names
4. Generate WIT bindings as part of the build process

## Standardized Plugin API

All plugins now follow a consistent API pattern:

### Plugin Initialization

```go
plugin := guest.NewPlugin("plugin-id").
	WithMetadata(
		"Display Name", 
		"Description",
		"Version", 
		"Author",
	).
	WithTags("tag1", "tag2").
	WithCapability("method", "description")
```

### Message Handling

```go
plugin.Handle("method", func(msg guest.AlloyMessage) guest.AlloyMessage {
	// Handle message
	return guest.Reply(msg, responseData)
})
```

### Error Handling

```go
if err := json.Unmarshal(msg.Payload, &req); err != nil {
	return guest.ErrorReply(msg, "invalid_request: " + err.Error())
}
```

### Plugin Lifecycle

```go
plugin.OnInit(func() error {
	// Initialization code
	return nil
})

plugin.OnStart(func() {
	// Background process
})
```

## Migrated Plugins

### 1. Health Plugin
- **Name**: health
- **Capabilities**: status
- **Features**: Basic health monitoring
- **Improvements**: Enhanced metadata, better error handling

### 2. Buffer Plugin
- **Name**: buffer
- **Capabilities**: create, read, write, append, list, delete, save, load
- **Features**: Data buffer management
- **Improvements**: Standardized API, improved event handling

### 3. Chat Plugin
- **Name**: chat
- **Capabilities**: send, history, direct:send, direct:history, presence:update, presence:list
- **Features**: Chat functionality with channels and direct messages
- **Improvements**: Better message handling, standardized API

### 4. AI Plugin
- **Name**: ai
- **Capabilities**: config:set, config:get, provider:set, query, summarize
- **Features**: AI capabilities with multiple providers
- **Improvements**: Type-safe configuration, standardized provider handling

### 5. Project Plugin
- **Name**: project
- **Capabilities**: create, list, active, add:buffer, add:channel, open, save
- **Features**: Project management
- **Improvements**: Better project organization, standardized API

### 6. IAM Plugin
- **Name**: iam
- **Capabilities**: check, policy:set, identity:set
- **Features**: Identity and access management
- **Improvements**: Type-safe policies, better error handling

### 7. Secrets Plugin
- **Name**: secrets
- **Capabilities**: store_secret, get_secret
- **Features**: Secrets management
- **Improvements**: Standardized API, better error handling

### 8. Tasks Plugin
- **Name**: tasks
- **Capabilities**: create, list
- **Features**: Task management
- **Improvements**: Type-safe task structure, standardized API

## Build System

The build system has been enhanced to support the migrated plugins:

```bash
# Build all WIT plugins
just build-plugins

# Build a specific plugin
just build-plugins-plugin <plugin-name>

# Run WIT tests
just test-wit
just test-wit-plugins
```

## Benefits of Standardization

1. **Consistency**: All plugins follow the same patterns and conventions
2. **Discoverability**: Standardized metadata makes plugins easier to discover
3. **Maintainability**: Consistent structure makes plugins easier to maintain
4. **Extensibility**: Standardized APIs make it easier to add new features
5. **Documentation**: Self-documenting capabilities through metadata
6. **Tooling**: Better integration with build and deployment tools

## Next Steps

1. **Testing**: Complete thorough testing of all migrated plugins
2. **Performance Optimization**: Profile and optimize plugin performance
3. **Documentation**: Create comprehensive documentation for each plugin
4. **Examples**: Add more examples demonstrating plugin usage
5. **Tutorials**: Create tutorials for plugin development
6. **Monitoring**: Add monitoring and debugging tools for plugins