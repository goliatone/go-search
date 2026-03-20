package memory

import (
	"testing"

	"github.com/goliatone/go-search/providers"
)

func TestProviderContractSuite(t *testing.T) {
	providers.RunContractSuite(t, func(t *testing.T) providers.Provider {
		t.Helper()
		return New()
	})
}
