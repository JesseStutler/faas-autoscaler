package main

import (
	"context"
	"github.com/jessestutler/faas-autoscaler/pkg/config"
	"github.com/jessestutler/faas-autoscaler/pkg/sender"
	"github.com/openfaas/faas-cli/commands"
	"github.com/openfaas/faas-cli/proxy"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

const (
	DefaultNamespace = "openfaas-fn"
)

func main() {
	autoScalerConfig := config.ReadConfig()
	cliAuth := config.NewBasicAuth(autoScalerConfig.BasicAuthPassword, autoScalerConfig.BasicAuthPassword)
	timeout := time.Second * 60
	transport := commands.GetDefaultCLITransport(false, &timeout)
	proxyClient, err := proxy.NewClient(cliAuth, autoScalerConfig.GatewayAddress, transport, &timeout)
	prometheusClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	if err != nil {
		logrus.Errorf("Unable to create prometheus client, the error is %s", err.Error())
		return
	}
	updateTicker := time.NewTicker(30 * time.Second)
	functionTracing := make(map[string]struct{})
	getFunctionStatus(proxyClient, prometheusClient, autoScalerConfig, functionTracing) //first call, before the ticker works
	for {
		select {
		case <-updateTicker.C:
			getFunctionStatus(proxyClient, prometheusClient, autoScalerConfig, functionTracing)
		}
	}
}

//get FunctionStatus and update the timer map periodically
func getFunctionStatus(proxyClient *proxy.Client, prometheusClient *http.Client, autoScalerConfig *config.AutoScalerConfig,
	functionsTracing map[string]struct{}) {
	gatewayQuery := sender.NewGatewayQuery(proxyClient)
	prometheusQuery := metrics.NewPrometheusQuery(autoScalerConfig.PrometheusHost, autoScalerConfig.PrometheusPort, prometheusClient)
	functions, err := proxyClient.ListFunctions(context.Background(), DefaultNamespace)
	if err != nil {
		logrus.Errorf("Unable to list functions status, the error is %s", err.Error())
		return
	}
	for _, function := range functions {
		if _, exist := functionsTracing[function.Name]; !exist {
			functionsTracing[function.Name] = struct{}{}
			var ticker *time.Ticker
			scaler := &sender.Scaler{
				GatewayQuery:       gatewayQuery,
				PrometheusQuery:    prometheusQuery,
				FunctionNamespace:  DefaultNamespace,
				FunctionName:       function.Name,
				DecreasingDuration: autoScalerConfig.DecreasingDuration,
				CancelChan:         make(chan struct{}),
			}
			if function.Labels != nil {
				labels := *function.Labels
				tickerSetting := config.ExtractLabelValue(labels[config.TickerLabel], config.DefaultTicker)
				ticker = time.NewTicker(time.Duration(tickerSetting) * time.Second)
			} else {
				//use default setting
				ticker = time.NewTicker(config.DefaultTicker * time.Second)
			}
			go periodicScaler(scaler, ticker)
		}
	}
}

func periodicScaler(scaler *sender.Scaler, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			scaler.ScalingBasedOnLoad()
		}
	}
}
