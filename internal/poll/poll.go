package poll

import (
	"net/http"
	"strings"
	"time"

	"github.com/ericrrath/electric-eye/internal/util"
	"github.com/go-resty/resty/v2"
	"k8s.io/klog/v2"
)

// Poller listens on the in channel, and for each Monitor received, checks its target URL
// and sends a Result to the out channel
func Poller(id int, in <-chan *util.Monitor, out chan<- *util.Result, timeout time.Duration) {
	// Disable keep-alives to avoid accumulating too many idle connections
	transport := http.Transport{
		DisableKeepAlives: true,
	}
	httpClient := http.Client{
		Transport: &transport,
		Timeout:   timeout,
	}
	client := resty.NewWithClient(&httpClient)
	client.SetHeader("User-Agent", "electric-eye")
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	for mon := range in {
		now := time.Now()
		r := util.Result{Target: mon.TargetUrl, RequestTime: now}
		method := http.MethodGet
		if mon.Method == http.MethodHead {
			method = http.MethodHead
		}
		klog.V(4).Infof("polling: %s %s", method, mon.TargetUrl)
		resp, err := client.R().Execute(method, mon.TargetUrl)
		if resp != nil {
			r.ResponseTime = resp.Time()
			switch resp.StatusCode() {
			case http.StatusOK:
				r.Success = true
			default:
				klog.V(4).Infof("unsuccessful status code for %s: %d", mon.TargetUrl, resp.StatusCode())
				r.Success = false
			}
			// only consider the first non-CA cert when checking how many days of validity remain
			// and remember that err would have been non-nil if the cert had expired (i.e. we only
			// expect to find valid certs here)
			if resp.RawResponse != nil && resp.RawResponse.TLS != nil {
				for _, cert := range resp.RawResponse.TLS.PeerCertificates {
					if !cert.IsCA {
						r.SSLValidityDays = int(cert.NotAfter.Sub(now).Hours() / 24)
						break
					}
				}
			}
		}
		if err != nil {
			klog.V(3).Infof("error on request to %s: %+v", mon.TargetUrl, err)
			// TODO: find a cleaner way to check for this error
			if strings.Contains(err.Error(), "too many open files") {
				klog.Warningf("polling request for %s failed; might need to reduce number of pollers: %v", mon.TargetUrl, err)
			}
		}
		klog.V(4).Infof("poller %d processed %v", id, r)
		out <- &r
	}
}
