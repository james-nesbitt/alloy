package native

import (
	"context"
	"log/slog"
	"path/filepath"

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
	"plugin-iam": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewIdentityManager(ctx, logger, state)
	},
	"plugin-otel": NewTelemetryManagerPlugin,
	"plugin-command-manager": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		return NewCommandManager(logger), nil
	},
	"plugin-logger": func(ctx context.Context, logger *slog.Logger, state storage.StateStore) (any, error) {
		auditDir := state.BaseDir()
		if auditDir != "" {
			auditDir = filepath.Join(filepath.Dir(auditDir), "audit")
		} else {
			// fallback if it's memory store
			auditDir = "audit"
		}
		return NewLoggerManager(logger, auditDir)
	},
}
