package extension

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	spec "github.com/ShadowSmallBaby/ClawProxyHub/sdk/extension"
)

func TestManagedTrustControlsInstallationAndCannotRevokeActivePackages(t *testing.T) {
	f := newFixture(t)
	m := New(filepath.Join(t.TempDir(), "data"), "1.5.2", nil)
	var persisted spec.TrustStore
	if err := m.ConfigureManagedTrust(nil, func(value spec.TrustStore) error { persisted = value; return nil }); err != nil {
		t.Fatal(err)
	}
	file := f.pack("editor", "1.0.0", nil, nil)
	if _, err := m.Inspect(file); err == nil {
		t.Fatal("untrusted package accepted")
	}
	key := f.trust["test"]
	if err := m.UpdateTrust("test", &key); err != nil {
		t.Fatal(err)
	}
	if persisted["test"].PublicKey != key.PublicKey {
		t.Fatal("trust was not persisted")
	}
	if _, err := m.Install(context.Background(), file, []string{"workspace.read"}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTrust("test", nil); err == nil {
		t.Fatal("revoked the identity of an active extension")
	}
	restricted := key
	restricted.Permissions = nil
	if err := m.UpdateTrust("test", &restricted); err == nil {
		t.Fatal("revoked an active extension's grants")
	}
	if err := m.SetEnabled(context.Background(), "editor", false); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTrust("test", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(context.Background(), "editor", true); err == nil {
		t.Fatal("enabled a package after its trust was revoked")
	}
}

func TestManagedTrustKeepsFixedIdentitiesAndRollsBackFailedPersistence(t *testing.T) {
	f := newFixture(t)
	m := New(t.TempDir(), "1.5.2", f.trust)
	key := f.trust["test"]
	if err := m.ConfigureManagedTrust(spec.TrustStore{"test": key}, nil); err == nil {
		t.Fatal("database shadowed a fixed identity")
	}
	if err := m.ConfigureManagedTrust(nil, func(spec.TrustStore) error { return errors.New("database unavailable") }); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTrust("test", nil); err == nil {
		t.Fatal("deleted fixed identity")
	}
	if err := m.UpdateTrust("third-party", &key); err == nil {
		t.Fatal("ignored persistence failure")
	}
	entries := m.TrustEntries()
	if len(entries) != 1 || !entries[0].ReadOnly || entries[0].Code != "test" {
		t.Fatal(entries)
	}
	entries[0].Config.IDs[0] = "changed"
	if m.TrustEntries()[0].Config.IDs[0] == "changed" {
		t.Fatal("caller modified an identity through a snapshot")
	}
}

func TestManagedTrustRejectsInvalidIdentityConfigurations(t *testing.T) {
	f := newFixture(t)
	key := f.trust["test"]
	for _, edit := range []func(*spec.TrustKey){
		func(k *spec.TrustKey) { k.PublicKey = "invalid" },
		func(k *spec.TrustKey) { k.Certificate = "invalid" },
		func(k *spec.TrustKey) { k.IDs = []string{"*"} },
		func(k *spec.TrustKey) { k.Permissions = []string{"workspace.read", "workspace.read"} },
	} {
		invalid := key
		edit(&invalid)
		if err := validateTrust("test", invalid); err == nil {
			t.Fatal("accepted invalid trust configuration")
		}
	}
}
