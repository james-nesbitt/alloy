package cmdutil

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/james-nesbitt/alloy/pkg/frontend"
)

// SecurityFlags represents the common security-related command line flags.
type SecurityFlags struct {
	Insecure    *bool
	NoMTLS      *bool
	SecurityDir *string
}

// RegisterSecurityFlags adds common security flags to the provided FlagSet.
func RegisterSecurityFlags(fs *flag.FlagSet) SecurityFlags {
	return SecurityFlags{
		Insecure:    fs.Bool("insecure", false, "Disable all security (RBAC and mTLS)"),
		NoMTLS:      fs.Bool("no-mtls", false, "Disable mTLS transport security only"),
		SecurityDir: fs.String("security-dir", "", "Path to the security/identity directory"),
	}
}

// ValidateSecurity ensures that insecure mode and security paths are not used simultaneously.
func (s SecurityFlags) Validate() error {
	if *s.Insecure {
		if *s.SecurityDir != "" {
			return fmt.Errorf("cannot use --insecure together with --security-dir")
		}
		// If insecure is set, automatically disable mTLS unless already disabled
		*s.NoMTLS = true
	}
	return nil
}

// HandleSecurityError prints a security validation error and exits.
func HandleSecurityError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Security Configuration Error: %v\n", err)
		os.Exit(1)
	}
}

// SetupLogger initializes a global slog logger with the given level.
func SetupLogger(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	return logger
}

// InitClient is a helper to initialize a standard Alloy frontend client from flags.
func InitClient(name, actor, socket string, sf SecurityFlags) (*frontend.Client, error) {
	if socket == "" {
		socket = fmt.Sprintf("%s/default.sock", frontend.GetAlloyRuntimeDir())
	}

	if actor == "" {
		actor = name
	}

	return frontend.NewClientWithActorAndSecurity(name, actor, socket, *sf.Insecure, *sf.SecurityDir)
}
