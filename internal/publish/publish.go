package publish

import (
	"fmt"
	"strings"

	"github.com/ericrrath/electric-eye/internal/util"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

type Metrics struct {
	Up        prometheus.Gauge
	Time      prometheus.Gauge
	CertValid prometheus.Gauge
}

func (m *Metrics) set(result *util.Result) error {
	if result.Success {
		m.Up.Set(1)
	} else {
		m.Up.Set(0)
	}
	m.Time.Set(float64(result.ResponseTime.Milliseconds()))
	if result.SSLValidityDays > 0 {
		if m.CertValid == nil {
			return fmt.Errorf("sslValidityDays %d, but CertValid is nil for %v", result.SSLValidityDays, m)
		}
		m.CertValid.Set(float64(result.SSLValidityDays))
	}
	return nil
}

// Publisher reads results from the in channel, looks up or registers metrics as necessary,
// and sets their values.  Because this func maintains the map of target URLs to the associated
// metrics, we expect only one invocation of this func.
func Publisher(in <-chan *util.Result, env string) {
	metricsByUrl := make(map[string]*Metrics)
	for r := range in {
		m := metricsByUrl[r.Target]
		if m == nil {
			labels := map[string]string{"url": r.Target}
			if len(env) > 0 {
				labels["env"] = env
			}
			up := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "up", ConstLabels: labels})
			prometheus.MustRegister(up)
			responseTime := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "response_time", ConstLabels: labels})
			prometheus.MustRegister(responseTime)
			m = &Metrics{Up: up, Time: responseTime}
			if strings.HasPrefix(r.Target, "https") {
				certValidDays := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "cert_valid_days", ConstLabels: labels})
				prometheus.MustRegister(certValidDays)
				m.CertValid = certValidDays
			}
			metricsByUrl[r.Target] = m
		}
		if err := m.set(r); err != nil {
			klog.Errorf("error setting metric values for %s: %+v", r.Target, err)
		}
	}
}
