package simulator

import "fmt"

func unsupportedVersionError(version string) error {
	return fmt.Errorf("unsupported OCPP version %q (want 1.6, 2.0.1, or 2.1)", version)
}
