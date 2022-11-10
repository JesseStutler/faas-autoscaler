package sender

import (
	"context"
	"github.com/jessestutler/faas-autoscaler/pkg/config"
	"github.com/openfaas/faas-cli/proxy"
	"github.com/openfaas/faas/gateway/scaling"
	"log"
)

type GatewayQuery struct {
	ProxyClient *proxy.Client
}

func NewGatewayQuery(proxyClient *proxy.Client) scaling.ServiceQuery {
	return &GatewayQuery{
		ProxyClient: proxyClient,
	}
}

func (g GatewayQuery) GetReplicas(serviceName, serviceNamespace string) (scaling.ServiceQueryResponse, error) {
	function, err := g.ProxyClient.GetFunctionInfo(context.Background(), serviceName, serviceNamespace)
	minReplicas := scaling.DefaultMinReplicas
	maxReplicas := scaling.DefaultMaxReplicas
	scalingFactor := scaling.DefaultScalingFactor
	availableReplicas := function.AvailableReplicas

	targetLoad := scaling.DefaultTargetLoad

	if function.Labels != nil {
		labels := *function.Labels

		minReplicas = config.ExtractLabelValue(labels[scaling.MinScaleLabel], minReplicas)
		maxReplicas = config.ExtractLabelValue(labels[scaling.MaxScaleLabel], maxReplicas)
		extractedScalingFactor := config.ExtractLabelValue(labels[scaling.ScalingFactorLabel], scalingFactor)
		targetLoad = config.ExtractLabelValue(labels[scaling.TargetLoadLabel], targetLoad)

		if extractedScalingFactor >= 0 && extractedScalingFactor <= 100 {
			scalingFactor = extractedScalingFactor
		} else {
			log.Printf("Bad Scaling Factor: %d, is not in range of [0 - 100]. Will fallback to %d", extractedScalingFactor, scalingFactor)
		}
	}

	return scaling.ServiceQueryResponse{
		Replicas:          function.Replicas,
		MaxReplicas:       uint64(maxReplicas),
		MinReplicas:       uint64(minReplicas),
		ScalingFactor:     uint64(scalingFactor),
		AvailableReplicas: availableReplicas,
		Annotations:       function.Annotations,
		TargetLoad:        uint64(targetLoad),
	}, err
}

func (g GatewayQuery) SetReplicas(serviceName, serviceNamespace string, count uint64) error {
	err := g.ProxyClient.ScaleFunction(context.Background(), serviceName, serviceNamespace, count)
	return err
}
