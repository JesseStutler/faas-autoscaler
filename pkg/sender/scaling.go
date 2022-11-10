package sender

import (
	"fmt"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/scaling"
	"log"
	"math"
	"strconv"
)

func LoadBasedScalingSender(service scaling.ServiceQuery, prometheusQuery metrics.PrometheusQueryFetcher, defaultNamespace string, serviceName string) error {
	err := judgeIfNeedScaleService(service, prometheusQuery, defaultNamespace, serviceName)
	return err
}

func judgeIfNeedScaleService(service scaling.ServiceQuery, prometheusQuery metrics.PrometheusQueryFetcher, defaultNamespace string, serviceName string) error {
	query := fmt.Sprintf("sum(http_requests_in_flight{faas_function=\"%s\",kubernetes_namespace=\"%s\"})by(faas_function,kubernetes_namespace)",
		serviceName, defaultNamespace)
	resp, err := prometheusQuery.Fetch(query)
	if err != nil {
		return err
	}
	result := resp.Data.Result
	if len(result) != 0 {
		sumLoad, _ := strconv.Atoi(result[0].Value[1].(string))
		queryResponse, getErr := service.GetReplicas(serviceName, defaultNamespace)
		avgLoad := float64(sumLoad) / float64(queryResponse.AvailableReplicas)
		log.Printf("Current load(in flight request) of function %s.%s is %d, available replicas are %d, average load is %f", serviceName, defaultNamespace,
			sumLoad, queryResponse.AvailableReplicas, avgLoad)
		if getErr == nil {
			newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, uint64(sumLoad), queryResponse.TargetLoad)
			log.Printf("[Scale] function=%s %d => %d.\n", serviceName, queryResponse.Replicas, newReplicas)
			if newReplicas == queryResponse.Replicas {
				return nil
			}
			updateErr := service.SetReplicas(serviceName, defaultNamespace, newReplicas)
			if updateErr != nil {
				err = updateErr
			}
		}
	} else {
		log.Printf("Prometheus scrapes function %s.%s failed, maybe because target down", serviceName, defaultNamespace)
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
