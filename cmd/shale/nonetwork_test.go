package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestLaptopPathsHaveNoNetworkDeps enforces docs/product.md §6: Shale makes
// no network calls from laptop code paths. Every package that runs during
// capture, finalize, or init must have zero dependency — direct or
// transitive — on the Go net stack. Only internal/forge (the CI render
// path) may touch the network.
func TestLaptopPathsHaveNoNetworkDeps(t *testing.T) {
	laptopPkgs := []string{
		"capture", "fold", "store", "gitx", "initx", "redact", "pricing",
	}
	for _, pkg := range laptopPkgs {
		out, err := exec.Command("go", "list", "-deps",
			"github.com/provasign/shale/internal/"+pkg).Output()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		for _, dep := range strings.Split(string(out), "\n") {
			if dep == "net" || strings.HasPrefix(dep, "net/") {
				t.Errorf("internal/%s depends on %q — laptop code paths must never touch the network (§6)", pkg, dep)
			}
			if strings.Contains(dep, "internal/forge") {
				t.Errorf("internal/%s depends on forge — the CI render path must stay out of laptop packages (§6)", pkg)
			}
		}
	}
}
