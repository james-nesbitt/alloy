package wasm

import (
	"encoding/json"
	"fmt"
)

// KVStore provides a typed key-value storage interface.
type KVStore[T any] struct {
	prefix string
}

// NewKVStore creates a new typed KVStore with the given prefix.
func NewKVStore[T any](prefix string) *KVStore[T] {
	return &KVStore[T]{prefix: prefix}
}

func (s *KVStore[T]) makeKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", s.prefix, key)
}

// Set stores a value in the durable KV store.
func (s *KVStore[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !KVSet(s.makeKey(key), data) {
		return fmt.Errorf("kv: set failed for key %s", key)
	}
	return nil
}

// Get retrieves a value from the durable KV store.
func (s *KVStore[T]) Get(key string) (T, error) {
	var val T
	data := KVGet(s.makeKey(key))
	if data == nil {
		return val, fmt.Errorf("kv: not found")
	}
	err := json.Unmarshal(data, &val)
	return val, err
}

// Delete removes a key from the durable KV store.
func (s *KVStore[T]) Delete(key string) error {
	if !KVDelete(s.makeKey(key)) {
		return fmt.Errorf("kv: delete failed for key %s", key)
	}
	return nil
}

// SessionCache provides a high-speed transient storage interface using the same KV API.
// Note: In Alloy, host-level caching is currently mapped to the same durable KV system
// but could be optimized within the kernel later.
type SessionCache[T any] struct {
	prefix string
}

func NewSessionCache[T any](prefix string) *SessionCache[T] {
	return &SessionCache[T]{prefix: "cache:" + prefix}
}

func (s *SessionCache[T]) makeKey(key string) string {
	return fmt.Sprintf("%s:%s", s.prefix, key)
}

func (s *SessionCache[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !KVSet(s.makeKey(key), data) {
		return fmt.Errorf("cache: set failed for key %s", key)
	}
	return nil
}

func (s *SessionCache[T]) Get(key string) (T, error) {
	var val T
	data := KVGet(s.makeKey(key))
	if data == nil {
		return val, fmt.Errorf("cache: not found")
	}
	err := json.Unmarshal(data, &val)
	return val, err
}
