# WIT-bindgen Migration Plan

## Overview
This document outlines the plan to migrate Alloy's WASM plugin system from the current manual implementation to a WIT-bindgen based approach. This will reduce complexity, improve type safety, and enable better tooling support.

## Current Challenges

1. **Manual Memory Management**: Current implementation requires manual pointer handling between host and guest
2. **JSON Serialization Overhead**: All communication goes through JSON serialization/deserialization
3. **Complex Host-Guest Interface**: The host needs to provide numerous functions with intricate pointer handling
4. **Error-Prone Buffer Management**: Fixed-size buffers and manual memory copying
5. **Limited Type Safety**: Everything is passed as bytes/JSON with no compile-time type checking

## WIT-bindgen Benefits

1. **Type-Safe Interfaces**: Generated code provides proper Go types and functions
2. **Automated Serialization**: No more manual JSON marshaling/unmarshaling
3. **Simplified Communication**: Generated code handles all low-level WASM communication
4. **Component Model Support**: Future-proof for WASM Component Model standard
5. **Reduced Boilerplate**: Eliminate much of the current host/guest interface code

## Migration Strategy

### Phase 1: Preparation (Current Phase)
- [x] Define WIT interface for Alloy's WASM plugin system
- [x] Generate initial bindings with wit-bindgen
- [x] Set up build system integration
- [ ] Create documentation for the new approach

### Phase 2: Dual-Runtime Support
- [ ] Implement a new runtime that uses the WIT bindings alongside the existing one
- [ ] Create adapter layer to translate between old and new interfaces
- [ ] Update plugin SDK to support both approaches
- [ ] Add build flags to select runtime

### Phase 3: Plugin Migration
- [ ] Migrate one simple plugin to use WIT bindings
- [ ] Update build system to support WIT-enabled plugins
- [ ] Create migration guide for plugin authors
- [ ] Provide backward compatibility

### Phase 4: Full Adoption
- [ ] Migrate all core plugins to WIT bindings
- [ ] Remove old runtime and SDK code
- [ ] Update documentation to reflect new approach

## WIT Interface

The WIT interface defined in `wit/alloy.wit` provides:
- Structured message passing between host and guest
- Type-safe function calls with proper Go types
- Key-value storage operations
- Plugin lifecycle management
- Error handling

## Implementation Details

### Host Side
The host will use the generated Rust bindings (for now) as a bridge to the Go implementation. Eventually, when Go support is available in wit-bindgen, we'll switch to native Go bindings.

### Guest Side
Plugins will use the generated TinyGo bindings, which provide:
- Type-safe function signatures
- Proper Go structs for WIT types
- Memory management handled by the generated code

## Build System Updates

The justfile has been updated with targets for:
- `install-wit-bindgen`: Install the wit-bindgen tool
- `generate-wit-bindings`: Generate the bindings from the WIT file
- `build-plugins-wit`: Future target for building WIT-enabled plugins

## Next Steps

1. Implement the new WIT-based runtime
2. Create adapter layer for backward compatibility
3. Migrate one plugin as a proof of concept
4. Update documentation and examples