package config

import "github.com/openfaas/faas/gateway/types"

type AutoScalerConfig struct {
	PrometheusHost    string
	PrometheusPort    int
	GatewayAddress    string
	BasicAuthUser     string
	BasicAuthPassword string
}

func ReadConfig(hasEnv types.HasEnv) *AutoScalerConfig {
	cfg := &AutoScalerConfig{
		PrometheusPort: 9090,
	}
	cfg.PrometheusHost = hasEnv.Getenv("prometheus_host")        // in helm
	cfg.GatewayAddress = hasEnv.Getenv("gateway_address")        // in helm
	cfg.BasicAuthUser = hasEnv.Getenv("basic_auth_user")         // in dockerfile, only for testing
	cfg.BasicAuthPassword = hasEnv.Getenv("basic_auth_password") // in dockerfile, only for testing
	return cfg
}
