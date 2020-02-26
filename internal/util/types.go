package util

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"strings"
	"time"
)

type Monitor struct {
	TargetUrl string `json:"targetUrl"`
}

type Data struct {
	Monitors []Monitor `json:"monitors"`
}

func NewMonitor(url string) *Monitor {
	mon := Monitor{TargetUrl: url}
	return &mon
}

type Result struct {
	target          string
	success         bool
	responseTime    time.Duration
	sslValidityDays int
	requestTime     time.Time
}

type Metrics struct {
	Up        prometheus.Gauge
	Time      prometheus.Gauge
	CertValid prometheus.Gauge
}

func Load(dataPath string) (*Data, error) {
	dataFile, err := ioutil.ReadFile(dataPath)
	if err != nil {
		return nil, err
	}
	data := Data{}
	if err = json.Unmarshal([]byte(dataFile), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (m *Metrics) set(result *Result) error {
	if result.success {
		m.Up.Set(1)
	} else {
		m.Up.Set(0)
	}
	m.Time.Set(float64(result.responseTime.Milliseconds()))
	if result.sslValidityDays > 0 {
		if m.CertValid == nil {
			return fmt.Errorf("sslValidityDays %d, but CertValid is nil for %v", result.sslValidityDays, m)
		}
		m.CertValid.Set(float64(result.sslValidityDays))
	}
	return nil
}


// Poller listens on the in channel, and for each Monitor received, checks its target URL
// and sends a Result to the out channel
func Poller(id int, in <-chan *Monitor, out chan<- *Result, timeout time.Duration) {
	client := resty.New()
	client.SetTimeout(timeout)
	for mon := range in {
		now := time.Now()
		r := Result{target: mon.TargetUrl, requestTime: now}
		resp, err := client.R().Get(mon.TargetUrl)
		if err != nil {
			r.success = false
			r.responseTime = -1
			r.sslValidityDays = -1
		} else {
			switch resp.StatusCode() {
			case http.StatusOK:
				r.success = true
			default:
				r.success = false
			}
			r.responseTime = resp.Time()
			// only consider the first non-CA cert when checking how many days of validity remain
			// and remember that err would have been non-nil if the cert had expired (i.e. we only
			// expect to find valid certs here)
			if resp.RawResponse.TLS != nil {
				for _, cert := range resp.RawResponse.TLS.PeerCertificates {
					if !cert.IsCA {
						r.sslValidityDays = int(cert.NotAfter.Sub(now).Hours() / 24)
						break
					}
				}
			}
		}
		klog.Infof("poller %d processed %v", id, r)
		out <- &r
	}
}

// Publisher reads results from the in channel, looks up or registers metrics as necessary,
// and sets their values.  Because this func maintains the map of target URLs to the associated
// metrics, we expect only one invocation of this func.
func Publisher(in <-chan *Result, env string) {
	metricsByUrl := make(map[string]*Metrics)
	for r := range in {
		m := metricsByUrl[r.target]
		if m == nil {
			labels := map[string]string{"url": r.target}
			if len(env) > 0 {
				labels["env"] = env
			}
			up := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "up", ConstLabels: labels})
			prometheus.MustRegister(up)
			responseTime := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "response_time", ConstLabels: labels})
			prometheus.MustRegister(responseTime)
			m = &Metrics{Up: up, Time: responseTime}
			if strings.HasPrefix(r.target, "https") {
				certValidDays := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "electric_eye", Name: "cert_valid_days", ConstLabels: labels})
				prometheus.MustRegister(certValidDays)
				m.CertValid = certValidDays
			}
			metricsByUrl[r.target] = m
		}
		if err := m.set(r); err != nil {
			klog.Errorf("error setting metric values for %s: %+v", r.target, err)
		}
	}
}

