package native

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/james-nesbitt/alloy/pkg/storage"
)

type PluginConstructor func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error)

var Registry = map[string]PluginConstructor{
	"events": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewEventManager(logger), nil
	},
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
	"iam": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewIdentityManager(ctx, logger, state)
	},
	"health":    NewHealthManagerPlugin,
	"telemetry": NewTelemetryManagerPlugin,
	"command-manager": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewCommandManager(logger), nil
	},
	"logger": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		auditDir := state.BaseDir()
		if auditDir != "" {
			auditDir = filepath.Join(filepath.Dir(auditDir), "audit")
		} else {
			// fallback if it's memory store
			auditDir = "audit"
		}
		return NewLoggerManager(logger, auditDir)
	},
	"ollama": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewOllamaProvider(ctx, logger, state)
	},
}
