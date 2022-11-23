package main

import (
	"context"
	"github.com/jessestutler/faas-autoscaler/pkg/config"
	faas_autoscaler "github.com/jessestutler/faas-autoscaler/pkg/proto/github.com/jessestutler/faas-autoscaler"
	"github.com/jessestutler/faas-autoscaler/pkg/sender"
	"github.com/openfaas/faas-cli/commands"
	"github.com/openfaas/faas-cli/proxy"
	"github.com/openfaas/faas-provider/types"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net/http"
	"time"
)

const (
	DefaultNamespace = "openfaas-fn"
)

func main() {
	osEnv := types.OsEnv{}
	autoScalerConfig := config.ReadConfig(osEnv)
	cliAuth := config.NewBasicAuth(autoScalerConfig.BasicAuthPassword, autoScalerConfig.BasicAuthPassword)
	timeout := time.Second * 60
	transport := commands.GetDefaultCLITransport(false, &timeout)
	proxyClient, err := proxy.NewClient(cliAuth, autoScalerConfig.GatewayAddress, transport, &timeout)
	grpcConn, err := grpc.Dial(autoScalerConfig.PredictServerAddress, grpc.WithInsecure())
	if err != nil {
		logrus.Errorf("Unable to connect to the grpc server, the error is %s", err.Error())
		return
	}
	prometheusClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	if err != nil {
		logrus.Errorf("Unable to create prometheus client, the error is %s", err.Error())
		return
	}
	updateTicker := time.NewTicker(30 * time.Second)
	functionTracing := make(map[string]struct{})
	getFunctionStatus(proxyClient, prometheusClient, autoScalerConfig, functionTracing, grpcConn) //first call, before the ticker works
	for {
		select {
		case <-updateTicker.C:
			getFunctionStatus(proxyClient, prometheusClient, autoScalerConfig, functionTracing, grpcConn)
		}
	}
}

//get FunctionStatus and update the timer map periodically
func getFunctionStatus(proxyClient *proxy.Client, prometheusClient *http.Client, autoScalerConfig *config.AutoScalerConfig,
	functionsTracing map[string]struct{}, grpcConn *grpc.ClientConn) {
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
			var periodicTicker *time.Ticker
			scaler := &sender.Scaler{
				GatewayQuery:      gatewayQuery,
				PrometheusQuery:   prometheusQuery,
				FunctionNamespace: DefaultNamespace,
				FunctionName:      function.Name,
				NormalFreq:        sender.NewSlideWindow(autoScalerConfig.PredictMaxSize),
				UpdateFreq:        sender.NewSlideWindow(autoScalerConfig.UpdateMaxSize),
				PredictClient:     faas_autoscaler.NewPredictServiceClient(grpcConn),
			}
			if function.Labels != nil {
				labels := *function.Labels
				tickerSetting := config.ExtractLabelValue(labels[config.TickerLabel], config.DefaultTicker)
				periodicTicker = time.NewTicker(time.Duration(tickerSetting) * time.Second)
			} else {
				//use default setting
				periodicTicker = time.NewTicker(config.DefaultTicker * time.Second)
			}
			predictTicker := time.NewTicker(autoScalerConfig.PredictInterval)
			go multiScaler(scaler, periodicTicker, predictTicker, autoScalerConfig.PredictInterval)
		}
	}
}

func multiScaler(scaler *sender.Scaler, periodicTicker *time.Ticker, predictTicker *time.Ticker, interval time.Duration) {
	for {
		select {
		case <-periodicTicker.C:
			if !scaler.PredictMode {
				//非预测模式
				scaler.ScalingBasedOnLoad()
			}
		case <-predictTicker.C:
			scaler.ScalingBasedOnPrediction(interval)
		}
	}
}
