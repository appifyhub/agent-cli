// Package realkeychain_test holds the opt-in test of the machine's real credential store. It lives
// in its own package because the go-keyring mock the other tests install is process-wide and cannot
// be undone.
package realkeychain_test

import (
	"errors"
	"os"
	"testing"

	"github.com/appifyhub/agent-cli/internal/credentials"
)

func TestRealKeychain(t *testing.T) {
	if os.Getenv("THE_AGENT_CLI_TEST_KEYCHAIN") != "1" {
		t.Skip("set THE_AGENT_CLI_TEST_KEYCHAIN=1 to use the real credential store of this machine")
	}
	const account = "6b8f0d3e-2a1c-4e55-9a37-0c1d2e3f4a5b"
	const secret = "header.payload.signature"
	service := "agent-cli-test-" + account
	k := credentials.SystemKeychain{}
	t.Cleanup(func() { _ = k.Delete(service, account) })

	if err := credentials.Probe(k, service); err != nil {
		t.Fatalf("the real keychain is not usable: %v", err)
	}
	if err := k.Set(service, account, secret); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get(service, account); err != nil || got != secret {
		t.Errorf("Get() = %q, %v", got, err)
	}
	if err := k.Delete(service, account); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get(service, account); !errors.Is(err, credentials.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}
