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

func TestApplySkipsStringSlice(t *testing.T) {
	type extractLike struct {
		BroadcastKey string   `env:"MPC_CLI_BROADCAST_KEY"`
		TradeKey     []string // 如 Extract.tradeKey，不应触发 panic
		Tags         []string
	}
	cfg := extractLike{
		BroadcastKey: "file-bk",
		TradeKey:     []string{"a", "b"},
		Tags:         []string{"x"},
	}
	os.Setenv("MPC_CLI_BROADCAST_KEY", "env-bk")
	t.Cleanup(func() { os.Unsetenv("MPC_CLI_BROADCAST_KEY") })

	if err := Apply(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BroadcastKey != "env-bk" {
		t.Fatalf("broadcastKey: got %q", cfg.BroadcastKey)
	}
	if len(cfg.TradeKey) != 2 || cfg.TradeKey[0] != "a" {
		t.Fatalf("tradeKey should be unchanged: %v", cfg.TradeKey)
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
