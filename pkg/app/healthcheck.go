// Package-level health probe: the service binary probes itself.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/config"
)

// healthcheckFlag turns a service binary into its own probe: `service -healthcheck`.
const healthcheckFlag = "-healthcheck"

// MaybeHealthcheck handles the `-healthcheck` argument and exits the process when
// it is present, otherwise it returns immediately.
//
// Why this exists instead of relying on a tool inside the image: the container
// runtime is a static binary on a scratch base, so there is no shell, no wget and
// no curl to call. Probes then need the binary itself — which is also what keeps
// the image small and removes a userland an attacker could use after a foothold.
//
// Call it as the very first statement of main().
func MaybeHealthcheck(service string) {
	if len(os.Args) < 2 || os.Args[1] != healthcheckFlag {
		return
	}

	cfg, err := config.Load(service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: configuration: %v\n", err)
		os.Exit(1)
	}
	os.Exit(probeSelf(cfg.HTTPAddr))
}

// probeSelf requests the service's own /healthz and maps the result to an exit
// code the container runtime understands.
func probeSelf(listenAddr string) int {
	address := listenAddr
	if address == "" {
		address = ":8080"
	}
	// A listen address such as ":8080" means every interface; probe belongs to
	// the loopback interface.
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/healthz", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: build request: %v\n", err)
		return 1
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: request failed: %v\n", err)
		return 1
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d\n", response.StatusCode)
		return 1
	}
	return 0
}
