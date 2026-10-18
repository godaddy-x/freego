package grpcgw

import "fmt"

// MethodConfig mirrors httpsvr.RouterConfig for grpcgw routes.
type MethodConfig struct {
	Guest       bool
	UsePlan2    bool
	KeyRoute    bool
	LoginRoute  bool
	AesRequest  bool
	AesResponse bool
}

func validateMethodConfig(cfg *MethodConfig) error {
	if cfg.KeyRoute && cfg.LoginRoute {
		return fmt.Errorf("grpcgw: KeyRoute and LoginRoute cannot both be true")
	}
	if (cfg.KeyRoute || cfg.LoginRoute) && !cfg.UsePlan2 {
		return fmt.Errorf("grpcgw: key/login route requires UsePlan2")
	}
	if cfg.LoginRoute && cfg.Guest {
		return fmt.Errorf("grpcgw: LoginRoute cannot be Guest")
	}
	return nil
}
