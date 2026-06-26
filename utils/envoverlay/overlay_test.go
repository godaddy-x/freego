package envoverlay

import (
	"os"
	"testing"
)

type testConfig struct {
	ClientPrk string `json:"clientPrk" env:"MPC_NODE_CLIENT_PRK"`
	Keystore  string `json:"keystoreKey" env:"MPC_KEYSTORE_KEY"`
}

func TestApplyOverridesFromEnv(t *testing.T) {
	os.Setenv("MPC_NODE_CLIENT_PRK", "tee-prk")
	os.Setenv("MPC_KEYSTORE_KEY", "tee-ks")
	t.Cleanup(func() {
		os.Unsetenv("MPC_NODE_CLIENT_PRK")
		os.Unsetenv("MPC_KEYSTORE_KEY")
	})

	cfg := testConfig{ClientPrk: "file-prk", Keystore: "file-ks"}
	if err := Apply(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClientPrk != "tee-prk" || cfg.Keystore != "tee-ks" {
		t.Fatalf("got clientPrk=%q keystore=%q", cfg.ClientPrk, cfg.Keystore)
	}
}

func TestApplyKeepsFileWhenEnvMissing(t *testing.T) {
	cfg := testConfig{ClientPrk: "file-prk"}
	if err := Apply(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClientPrk != "file-prk" {
		t.Fatalf("clientPrk: got %q", cfg.ClientPrk)
	}
}

func TestApplyNestedSlice(t *testing.T) {
	os.Setenv("NESTED_SECRET", "from-env")
	t.Cleanup(func() { os.Unsetenv("NESTED_SECRET") })

	type item struct {
		Secret string `env:"NESTED_SECRET"`
	}
	type root struct {
		Items []item
	}
	cfg := root{Items: []item{{Secret: "file"}, {Secret: "file2"}}}
	if err := Apply(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Items[0].Secret != "from-env" || cfg.Items[1].Secret != "from-env" {
		t.Fatalf("items: %+v", cfg.Items)
	}
}
