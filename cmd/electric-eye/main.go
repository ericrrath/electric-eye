package main

import (
	"flag"
	"github.com/ericrrath/electric-eye/internal/fetch"
	"github.com/ericrrath/electric-eye/internal/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
	"net/http"
	"time"
)

func main() {
	dataPath := flag.String("dataPath", "", "path to data file, e.g. /tmp/monitors.json")
	checkerPeriod := flag.Duration("checkerPeriod", 10*time.Minute, "duration between checks, e.g. '10m'")
	checkTimeout := flag.Duration("checkTimeout", 1*time.Minute, "timeout when checking, e.g. '1m'")
	listenAddress := flag.String("listenAddress", ":8080", "address to listen on for HTTP metrics requests")
	numPollers := flag.Int("numPollers", 2, "number of concurrent pollers")
	env := flag.String("env", "", "'env' label added to each metric; allows aggregation of metrics from multiple electric-eye instances")
	uptimeRobotAPIKey := flag.String("uptimeRobotAPIKey", "", "UptimeRobot API key")

	flag.Parse()

	monitorsByUrl := make(map[string]*util.Monitor)

	found := make(chan string)
	if len(*dataPath) > 0 {
		data, err := util.Load(*dataPath)
		if err != nil {
			klog.Fatalf("error loading monitors from %s: %+v", *dataPath, err)
		}
		for i := range data.Monitors {
			m := data.Monitors[i]
			monitorsByUrl[m.TargetUrl] = &m
		}
	}

	pending := make(chan *util.Monitor)
	complete := make(chan *util.Result)

	// Start pollers which receive Monitor instances and send Result instances
	for i := 0; i < *numPollers; i++ {
		go util.Poller(i, pending, complete, *checkTimeout)
	}
	// Start a single publisher which receives Result instances and publishes them as Prometheus metrics
	go util.Publisher(complete, *env)

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(*listenAddress, nil)

	if len(*uptimeRobotAPIKey) > 0 {
		go fetch.Fetch(*uptimeRobotAPIKey, found)
	}

	// Start a ticker, and start sending Monitor instances to the waiting pollers,
	// and add monitors to the list as the fetcher finds them
	pollTicker := time.Tick(*checkerPeriod)
	for {
		select {
			case <-pollTicker:
				klog.Infof("sending %d monitors to pending queue", len(monitorsByUrl))
				for _, v := range monitorsByUrl {
					m := v
					pending <- m
				}
			case url := <-found:
				klog.Infof("received url: %s", url)
				m := util.NewMonitor(url)
				monitorsByUrl[m.TargetUrl] = m
		}
	}
}
