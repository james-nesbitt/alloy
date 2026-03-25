package native

import (
	"context"
	"log/slog"

	"github.com/james-nesbitt/alloy/pkg/storage"
)

type PluginConstructor func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error)

var Registry = map[string]PluginConstructor{
	"kv": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewKVManager(state), nil
	},
	"cache": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewCacheManager(), nil
	},
	"doc": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewDocStore(), nil
	},
	"network": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewNetworkManager(), nil
	},
	"storage": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewStorageManager(), nil
	},
	"ollama": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewOllamaProvider(ctx, logger, state)
	},
}
