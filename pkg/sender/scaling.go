package sender

import (
	"fmt"
	"github.com/openfaas/faas/gateway/metrics"
	"github.com/openfaas/faas/gateway/scaling"
	"github.com/sirupsen/logrus"
	"math"
	"strconv"
	"time"
)

type Scaler struct {
	GatewayQuery      scaling.ServiceQuery
	PrometheusQuery   metrics.PrometheusQuery
	FunctionNamespace string
	FunctionName      string

	//Represents load situation, if the load is decreasing, the NonDeceasing flag will be false, and the DecreasingTimer will be turned on,
	NonDecreasing bool
	//The number of replicas will be scaled down if timer is triggered, if NonDecreasing is true, the timer will be shut down.
	DecreasingTimer    *time.Timer
	DecreasingDuration time.Duration

	//CancelChan is used to prevent goroutine leak when DecreasingTimer was stopped
	CancelChan chan struct{}

	//whether if DecreasingTimer is triggered
	LastTriggerTimer bool
}

func (s *Scaler) ScalingBasedOnLoad() {
	query := fmt.Sprintf("sum(http_requests_in_flight{faas_function=\"%s\",kubernetes_namespace=\"%s\"})by(faas_function,kubernetes_namespace)",
		s.FunctionName, s.FunctionNamespace)
	resp, err := s.PrometheusQuery.Fetch(query)
	if err != nil {
		logrus.Errorf("Unable to query metrics from prometheus, the error is %s", err)
		return
	}
	result := resp.Data.Result
	if len(result) != 0 {
		sumLoad, _ := strconv.Atoi(result[0].Value[1].(string))
		queryResponse, getErr := s.GatewayQuery.GetReplicas(s.FunctionName, s.FunctionNamespace)
		avgLoad := float64(sumLoad) / float64(queryResponse.AvailableReplicas)
		logrus.Infof("Current load(in flight request) of function %s.%s is %d, available replicas are %d, average load is %f", s.FunctionName, s.FunctionNamespace,
			sumLoad, queryResponse.AvailableReplicas, avgLoad)
		if getErr == nil {
			newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, uint64(sumLoad), queryResponse.TargetLoad)
			if newReplicas >= queryResponse.Replicas {
				if s.DecreasingTimer != nil {
					if s.DecreasingTimer.Stop() {
						s.LastTriggerTimer = false
						s.CancelChan <- struct{}{}
					}
				}
				s.NonDecreasing = true
				if newReplicas == queryResponse.Replicas {
					return // not to update replicas
				}
			} else if newReplicas < queryResponse.Replicas {
				if s.NonDecreasing || s.LastTriggerTimer {
					s.NonDecreasing = false
					s.DecreasingTimer = time.NewTimer(s.DecreasingDuration)
					go s.ScaleDownUsingTimer()
				}
				return //not to update replicas
			}
			// only newReplicas > queryResponse.Replicas gets to here
			updateErr := s.GatewayQuery.SetReplicas(s.FunctionName, s.FunctionNamespace, newReplicas)
			logrus.Infof("[Scale] function=%s %d => %d.", s.FunctionName, queryResponse.Replicas, newReplicas)
			if updateErr != nil {
				logrus.Errorf("Unable to set replicas, the error is %s", updateErr.Error())
			}
		} else {
			logrus.Errorf("Unable to get replicas, the error is %s", getErr.Error())
		}
	} else {
		logrus.Errorf("Prometheus scrapes function %s.%s failed, maybe because target down", s.FunctionName, s.FunctionNamespace)
	}
}

func (s *Scaler) ScaleDownUsingTimer() {
	select {
	case <-s.DecreasingTimer.C:
		s.LastTriggerTimer = true
		query := fmt.Sprintf("sum(http_requests_in_flight{faas_function=\"%s\",kubernetes_namespace=\"%s\"})by(faas_function,kubernetes_namespace)",
			s.FunctionName, s.FunctionNamespace)
		resp, err := s.PrometheusQuery.Fetch(query)
		if err != nil {
			logrus.Errorf("Unable to query metrics from prometheus, the error is %s", err)
			return
		}
		result := resp.Data.Result
		if len(result) != 0 {
			sumLoad, _ := strconv.Atoi(result[0].Value[1].(string))
			queryResponse, getErr := s.GatewayQuery.GetReplicas(s.FunctionName, s.FunctionNamespace)
			avgLoad := float64(sumLoad) / float64(queryResponse.AvailableReplicas)
			logrus.Infof("[Scale Down] Current load(in flight request) of function %s.%s is %d, available replicas are %d, average load is %f", s.FunctionName, s.FunctionNamespace,
				sumLoad, queryResponse.AvailableReplicas, avgLoad)
			if getErr == nil {
				newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, uint64(sumLoad), queryResponse.TargetLoad)
				updateErr := s.GatewayQuery.SetReplicas(s.FunctionName, s.FunctionNamespace, newReplicas)
				logrus.Infof("[Scale down] function=%s %d => %d.\n", s.FunctionName, queryResponse.Replicas, newReplicas)
				if updateErr != nil {
					logrus.Errorf("Unable to set replicas, the error is %s", updateErr.Error())
				}
			} else {
				logrus.Errorf("Unable to get replicas, the error is %s", getErr.Error())
			}
		} else {
			logrus.Errorf("[Scale down]Prometheus scrapes function %s.%s failed, maybe because target down", s.FunctionName, s.FunctionNamespace)
		}
	case <-s.CancelChan:
		//exit goroutine
	}
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
