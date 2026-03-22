package cmdutil

import (
	"flag"
	"fmt"
	"os"
)

// SecurityFlags represents the common security-related command line flags.
type SecurityFlags struct {
	Insecure    *bool
	SecurityDir *string
}

// RegisterSecurityFlags adds common security flags to the provided FlagSet.
func RegisterSecurityFlags(fs *flag.FlagSet) SecurityFlags {
	return SecurityFlags{
		Insecure:    fs.Bool("insecure", false, "Disable mTLS and use insecure communication"),
		SecurityDir: fs.String("security-dir", "", "Path to the security/identity directory"),
	}
}

// ValidateSecurity ensures that insecure mode and security paths are not used simultaneously.
func (s SecurityFlags) Validate() error {
	if *s.Insecure && *s.SecurityDir != "" {
		return fmt.Errorf("cannot use --insecure together with --security-dir")
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
