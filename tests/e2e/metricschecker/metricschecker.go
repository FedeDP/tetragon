package metricschecker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/cilium/tetragon/tests/e2e/state"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// MetricsChecker checks prometheus metrics from one or more events streams.
type MetricsChecker struct {
	name      string
	timeLimit time.Duration
}

// NewMetricsChecker constructs a new Metrics from a MultiEventChecker.
func NewMetricsChecker(name string) *MetricsChecker {
	rc := &MetricsChecker{
		name:      name,
		timeLimit: 0,
	}
	return rc
}

// WithTimeLimit sets the time limit for a MetricsChecker. MetricsChecker.Check takes longer
// than the time limit, it stops the check loop, calls FinalCheck(), and returns its value.
func (rc *MetricsChecker) WithTimeLimit(limit time.Duration) *MetricsChecker {
	rc.timeLimit = limit
	return rc
}

type metricOp int

const (
	opEqual   metricOp = 1
	opGreater          = 1 << iota
	opLess
)

type metricCheck struct {
	Value model.Value
	op    metricOp
}

func (rc *MetricsChecker) Equal(metric string, val int) features.Func {
	return rc.checkWithOp(metric, metricCheck{
		Value: &model.Scalar{
			Value: model.SampleValue(val),
		},
		op: opEqual,
	})
}

func (rc *MetricsChecker) Less(metric string, val int) features.Func {
	return rc.checkWithOp(metric, metricCheck{
		Value: &model.Scalar{
			Value: model.SampleValue(val),
		},
		op: opLess,
	})
}

func (rc *MetricsChecker) LessThanOrEqual(metric string, val int) features.Func {
	return rc.checkWithOp(metric, metricCheck{
		Value: &model.Scalar{
			Value: model.SampleValue(val),
		},
		op: opEqual | opLess,
	})
}

func (rc *MetricsChecker) Greater(metric string, val int) features.Func {
	return rc.checkWithOp(metric, metricCheck{
		Value: &model.Scalar{
			Value: model.SampleValue(val),
		},
		op: opGreater,
	})
}

func (rc *MetricsChecker) GreaterOrEqual(metric string, val int) features.Func {
	return rc.checkWithOp(metric, metricCheck{
		Value: &model.Scalar{
			Value: model.SampleValue(val),
		},
		op: opEqual | opGreater,
	})
}

func (rc *MetricsChecker) checkWithOp(metric string, check metricCheck) features.Func {
	return func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
		klog.InfoS("Gathering metrics clients", "metricschecker", rc.name)
		ports, ok := ctx.Value(state.PromForwardedPorts).(map[string]int)
		if !ok {
			assert.Fail(t, "failed to find forwarded prometheus ports")
			return ctx
		}
		results := make(map[string]model.Value)
		for podName, port := range ports {
			client, err := api.NewClient(api.Config{
				Address: "127.0.0.1:" + strconv.FormatInt(int64(port), 10) + "/metrics",
			})
			if err != nil {
				assert.Fail(t, "Error creating prometheus client", err)
				return ctx
			}
			v1Api := v1.NewAPI(client)

			// Gather results from current pod
			result, warnings, err := v1Api.Query(ctx, metric+"{pod="+podName+"}", time.Now(), v1.WithTimeout(rc.timeLimit))
			if err != nil {
				assert.Fail(t, "Error querying prometheus", err)
				return ctx
			}
			if len(warnings) > 0 {
				klog.V(2).Info("Metrics checker query warnings", "checker", rc.name, "pod", podName, "warnings", warnings)
			}
			// Store results for current pod
			results[podName] = result
		}
		if err := rc.check(results, metric, check); !assert.NoError(t, err, "checks should pass") {
			return ctx
		}
		return ctx
	}
}

func (rc *MetricsChecker) check(results map[string]model.Value, metric string, check metricCheck) error {
	klog.InfoS("Running metrics checks", "metricschecker", rc.name)

	for podName, result := range results {
		if result.Type() != check.Value.Type() {
			return errors.New("result type and check type mismatch")
		}
		switch result.Type() {
		case model.ValScalar:
			val, err := strconv.Atoi(result.String())
			if err != nil {
				return err
			}
			expectedVal, err := strconv.Atoi(check.Value.String())
			if err != nil {
				return err
			}
			success := false
			if check.op&opEqual != 0 {
				success = val == expectedVal
			}
			if check.op&opGreater != 0 {
				success = success || (val > expectedVal)
			}
			if check.op&opLess != 0 {
				success = success || (val < expectedVal)
			}
			if !success {
				return fmt.Errorf("failed metricscheck for metric '%s' on pod '%s'", metric, podName)
			}
		default:
			// TODO implement check logic for non-scalar types
			return errors.New("metrics checker unsupported for non-scalar types")
		}
	}
	return nil
}

// Name returns the name of the checker
func (rc *MetricsChecker) Name() string {
	return rc.name
}
