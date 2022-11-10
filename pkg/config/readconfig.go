package config

type AutoScalerConfig struct {
	PrometheusHost    string
	PrometheusPort    int
	GatewayAddress    string
	BasicAuthUser     string
	BasicAuthPassword string
}

func ReadConfig() *AutoScalerConfig {
	cfg := &AutoScalerConfig{
		PrometheusPort:    31114,
		PrometheusHost:    "10.2.0.120",
		GatewayAddress:    "http://10.2.0.120:31112",
		BasicAuthUser:     "admin",
		BasicAuthPassword: "admin",
	}
	return cfg
}
