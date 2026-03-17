package native

import (
	"context"
	"log/slog"

	"github.com/jnesbitt/alloy-go/pkg/storage"
)

type PluginConstructor func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error)

var Registry = map[string]PluginConstructor{
	"plugin-events": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewEventManager(logger), nil
	},
	"plugin-kv": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewKVManager(state), nil
	},
	"plugin-cache": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewCacheManager(), nil
	},
	"plugin-doc": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewDocStore(), nil
	},
	"plugin-network": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewNetworkManager(), nil
	},
	"plugin-storage": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewStorageManager(), nil
	},
}
