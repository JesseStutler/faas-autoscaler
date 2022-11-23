package config

import (
	"github.com/openfaas/faas/gateway/types"
	"time"
)

type AutoScalerConfig struct {
	PrometheusHost       string
	PrometheusPort       int
	GatewayAddress       string
	BasicAuthUser        string
	BasicAuthPassword    string
	PredictServerAddress string
	PredictMaxSize       int           // 预测最多存放多少时间步的数据
	UpdateMaxSize        int           // 更新模型需要多大的历史时间步数据
	PredictInterval      time.Duration //每隔多久做一次预测
	MAEUpperLimit        float64       //绝对误差上限
}

func ReadConfig(hasEnv types.HasEnv) *AutoScalerConfig {
	cfg := &AutoScalerConfig{
		PrometheusPort:  9090,
		PredictMaxSize:  5,
		UpdateMaxSize:   30,
		PredictInterval: time.Minute,
	}
	cfg.PrometheusHost = hasEnv.Getenv("prometheus_host")              // in helm
	cfg.GatewayAddress = hasEnv.Getenv("gateway_address")              // in helm
	cfg.BasicAuthUser = hasEnv.Getenv("basic_auth_user")               // in dockerfile, only for testing
	cfg.BasicAuthPassword = hasEnv.Getenv("basic_auth_password")       // in dockerfile, only for testing
	cfg.PredictServerAddress = hasEnv.Getenv("predict_server_address") // in helm
	return cfg
}
