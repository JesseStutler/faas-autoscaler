package sender

import (
	"context"
	"fmt"
	faas_autoscaler "github.com/jessestutler/faas-autoscaler/pkg/proto/github.com/jessestutler/faas-autoscaler"
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

	PredictMode    bool //true表示开启Predict模式，false表示使用周期性主动扩缩容方式
	PredictClient  faas_autoscaler.PredictServiceClient
	LastCounterVal int // 上次Counter的值，请求数是一个累加值
	NormalFreq     *SlideWindow
	UpdateFreq     *SlideWindow //用来更新模型用
	PredictFreq    *SlideWindow //用来与NormalFreq比较用，如果MAE大于AbsoluteDiffLimit则要进行更新模型
	MAEUpperLimit  float64      //绝对差值上限

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

func (s *Scaler) ScalingBasedOnPrediction(interval time.Duration) {
	serviceName := fmt.Sprintf("%s.%s", s.FunctionName, s.FunctionNamespace) // FunctionName.FunctionNamespace
	totalRequestsQuery := fmt.Sprintf("sum(gateway_function_invocation_total{function_name=\"%s\"}) by (function_name)", serviceName)
	result := s.DoPrometheusQuery(totalRequestsQuery)
	var requests int
	if result != nil {
		tmp, _ := strconv.Atoi(result.(string))
		requests = tmp - s.LastCounterVal
		s.UpdateFreq.Enqueue(requests) //historyWindow
		s.NormalFreq.Enqueue(requests) //用来进行预测的SlideWindow
		s.LastCounterVal = tmp
	}
	totalDurationQuery := fmt.Sprintf("http_request_duration_seconds_sum{status=\"200\",path=\"/system/function/%s\"}", s.FunctionName)
	result = s.DoPrometheusQuery(totalDurationQuery)
	var durationSum float64
	if result != nil {
		durationSum, _ = strconv.ParseFloat(result.(string), 64)
	}
	successTotalRequestQuery := fmt.Sprintf("gateway_function_invocation_total{function_name=\"%s\",code=\"200\"}", serviceName)
	result = s.DoPrometheusQuery(successTotalRequestQuery)
	var successRequests int
	if result != nil {
		successRequests, _ = strconv.Atoi(result.(string))
	}
	serverRate := float64(successRequests) / (durationSum / 60) //处理速率为个/min
	requestRate := float64(requests) / interval.Minutes()       //实际请求速率个/min
	logrus.Infof("The function %s.%s's request rate is %f r/min, server rate is %f r/min", s.FunctionName, s.FunctionNamespace, requestRate, serverRate)
	if s.NormalFreq.IsFull() {
		if s.PredictFreq.IsFull() {
			iterN := s.NormalFreq.Data.Iterator()
			iterP := s.PredictFreq.Data.Iterator()
			maeSum := 0.0
			for iterN.Next() && iterP.Next() {
				maeSum += math.Abs(iterN.Value().(float64) - iterP.Value().(float64))
			}
			if maeSum/float64(s.NormalFreq.Data.Size()) > s.MAEUpperLimit {
				s.PredictMode = false
			}
		} else {
			s.PredictMode = true
		}
	}
	if !s.PredictMode {
		//非预测模式，使用周期性主动扩缩容模式并更新模型
		if s.UpdateFreq.IsFull() {
			updateFreq := s.UpdateFreq.Values()
			historyWindow := make([]float64, len(updateFreq))
			for k, _ := range historyWindow {
				historyWindow[k] = updateFreq[k].(float64)
			}
			updateRequest := &faas_autoscaler.UpdateRequest{HistoryWindow: historyWindow}
			updateResp, err := s.PredictClient.UpdateModel(context.Background(), updateRequest, nil)
			if err != nil {
				logrus.Errorf("Unable to update the model, the error is %s", err.Error())
				return
			}
			if updateResp.Loss < s.MAEUpperLimit {
				s.PredictMode = true
			}
		}
	} else {
		//预测模式
		freq := s.NormalFreq.Values()
		slideWindow := make([]float64, len(freq))
		for k, _ := range slideWindow {
			slideWindow[k] = freq[k].(float64)
		}
		logrus.Infof("The function %s.%s's slidewindow is %v", s.FunctionName, s.FunctionNamespace, slideWindow)
		predictRequest := &faas_autoscaler.PredictRequest{SlideWindow: slideWindow}
		predictResult, err := s.PredictClient.Predict(context.Background(), predictRequest, nil)
		if err != nil {
			logrus.Errorf("Unable to get predict results, the error is %s", err.Error())
			return
		}
		resultsArr := predictResult.GetResults()
		logrus.Infof("The function %s.%s's predict result is %v", s.FunctionName, s.FunctionNamespace, resultsArr)
		s.PredictFreq.Enqueue(resultsArr[0]) //请求数只记录返回结果的第一个请求数量（对于多时间步预测多时间步的情况)
		nextTimeSum := 0.0
		for _, v := range predictResult.GetResults() {
			nextTimeSum += v
		}
		nextTimeRate := nextTimeSum / float64(len(resultsArr))
		queryResponse, err := s.GatewayQuery.GetReplicas(s.FunctionName, s.FunctionNamespace)
		if err != nil {
			logrus.Errorf("Unable to get replicas, the error is %s", err.Error())
			return
		}

		newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, uint64(nextTimeRate), uint64(serverRate))
		if newReplicas == queryResponse.Replicas {
			return
		}

		updateErr := s.GatewayQuery.SetReplicas(s.FunctionName, s.FunctionNamespace, newReplicas)
		if updateErr != nil {
			logrus.Errorf("Unable to set replicas, the error is %s", updateErr.Error())
			return
		}
	}
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
					s.LastTriggerTimer = false
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

func (s *Scaler) DoPrometheusQuery(query string) interface{} {
	resp, err := s.PrometheusQuery.Fetch(query)
	if err != nil {
		logrus.Errorf("Unable to query metrics from prometheus, the error is %s", err)
		return nil
	}
	result := resp.Data.Result
	if len(result) != 0 {
		return result[0].Value[1]
	} else {
		logrus.Errorf("Prometheus scrapes function %s.%s failed, maybe because target down", s.FunctionName, s.FunctionNamespace)
	}
	return nil
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
			logrus.Infof("Current load(in flight request) of function %s.%s is %d, available replicas are %d, average load is %f", s.FunctionName, s.FunctionNamespace,
				sumLoad, queryResponse.AvailableReplicas, avgLoad)
			if getErr == nil {
				newReplicas := calculateReplicasBasedOnLoad(queryResponse.MaxReplicas, queryResponse.MinReplicas, uint64(sumLoad), queryResponse.TargetLoad)
				updateErr := s.GatewayQuery.SetReplicas(s.FunctionName, s.FunctionNamespace, newReplicas)
				logrus.Infof("Scale down function=%s %d => %d.\n", s.FunctionName, queryResponse.Replicas, newReplicas)
				if updateErr != nil {
					logrus.Errorf("Unable to set replicas, the error is %s", updateErr.Error())
				}
			} else {
				logrus.Errorf("Unable to get replicas, the error is %s", getErr.Error())
			}
		} else {
			logrus.Errorf("Prometheus scrapes function %s.%s failed, maybe because target down", s.FunctionName, s.FunctionNamespace)
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
