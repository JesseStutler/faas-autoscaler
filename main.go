package main

import (
	"context"
	"github.com/jessestutler/faas-autoscaler/pkg/config"
	"github.com/jessestutler/faas-autoscaler/pkg/sender"
	"github.com/openfaas/faas-cli/commands"
	"github.com/openfaas/faas-cli/proxy"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/types"
	"log"
	"net/http"
	"time"
)

const (
	DefaultNamespace = "openfaas-fn"
)

func main() {
	osEnv := types.OsEnv{}
	autoScalerConfig := config.ReadConfig(osEnv)
	ctx, cancel := context.WithCancel(context.Background())
	cliAuth := config.NewBasicAuth(autoScalerConfig.BasicAuthPassword, autoScalerConfig.BasicAuthPassword)
	timeout := time.Second * 60
	transport := commands.GetDefaultCLITransport(false, &timeout)
	proxyClient, err := proxy.NewClient(cliAuth, autoScalerConfig.GatewayAddress, transport, &timeout)
	prometheusClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	if err != nil {
		log.Println(err.Error())
		return
	}
	updateTicker := time.NewTicker(30 * time.Second)
	errCh := make(chan error)
	functionTracing := make(map[string]struct{})
	for {
		select {
		case <-updateTicker.C:
			go getFunctionStatus(ctx, proxyClient, prometheusClient, autoScalerConfig, functionTracing, errCh)
		case <-errCh:
			updateTicker.Stop()
			cancel()
			return
		}
	}
}

//get FunctionStatus and update the timer map periodically
func getFunctionStatus(ctx context.Context, proxyClient *proxy.Client, prometheusClient *http.Client, autoScalerConfig *config.AutoScalerConfig,
	functionsTracing map[string]struct{}, errCh chan<- error) {
	gatewayQuery := sender.NewGatewayQuery(proxyClient)
	prometheusQuery := metrics.NewPrometheusQuery(autoScalerConfig.PrometheusHost, autoScalerConfig.PrometheusPort, prometheusClient)
	functions, err := proxyClient.ListFunctions(context.Background(), DefaultNamespace)
	if err != nil {
		log.Println(err.Error())
		errCh <- err
		return
	}
	for _, function := range functions {
		if _, exist := functionsTracing[function.Name]; !exist {
			var ticker *time.Ticker
			if function.Labels != nil {
				labels := *function.Labels
				tickerSetting := config.ExtractLabelValue(labels[config.TickerLabel], config.DefaultTicker)
				ticker = time.NewTicker(time.Duration(tickerSetting) * time.Second)
			} else {
				//use default setting
				ticker = time.NewTicker(config.DefaultTicker * time.Second)
			}
			go func(ctx context.Context) {
				for {
					select {
					case <-ticker.C:
						err := sender.LoadBasedScalingSender(gatewayQuery, prometheusQuery, DefaultNamespace, function.Name)
						if err != nil {
							log.Println(err.Error())
							ticker.Stop()
							errCh <- err
							return
						}
					case <-ctx.Done():
						//other goroutines met error
						ticker.Stop()
						return
					}
				}
			}(ctx)
		}
	}
}
