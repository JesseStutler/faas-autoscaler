package sender

import (
	"fmt"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/scaling"
	"log"
	"math"
)

func LoadBasedScalingSender(service scaling.ServiceQuery, prometheusQuery metrics.PrometheusQueryFetcher, defaultNamespace string, serviceName string) error {
	err := judgeIfNeedScaleService(service, prometheusQuery, defaultNamespace, serviceName)
	return err
}

func judgeIfNeedScaleService(service scaling.ServiceQuery, prometheusQuery metrics.PrometheusQueryFetcher, defaultNamespace string, serviceName string) error {
	functionName := fmt.Sprintf("%s.%s", serviceName, defaultNamespace)
	query := fmt.Sprintf("gateway_function_invocation_total{function_name=\"%s\"}", functionName)
	resp, err := prometheusQuery.Fetch(query)
	sumLoad := resp.Data.Result[0].Value[1].(uint64)
	queryResponse, getErr := service.GetReplicas(serviceName, defaultNamespace)
	if getErr == nil {
		newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, sumLoad, queryResponse.TargetLoad)
		log.Printf("[Scale] function=%s %d => %d.\n", serviceName, queryResponse.Replicas, newReplicas)
		if newReplicas == queryResponse.Replicas {
			return nil
		}
		updateErr := service.SetReplicas(serviceName, defaultNamespace, newReplicas)
		if updateErr != nil {
			err = updateErr
		}
	}
	return err
}

func calculateReplicasBasedOnLoad(maxReplicas uint64, minReplicas uint64, sumLoad uint64, targetLoad uint64) uint64 {
	var newReplicas uint64
	newReplicas = uint64(math.Ceil(float64(sumLoad) / float64(targetLoad)))
	if newReplicas >= maxReplicas {
		newReplicas = maxReplicas
	}
	if newReplicas <= minReplicas {
		newReplicas = minReplicas
	}
	return newReplicas
}
