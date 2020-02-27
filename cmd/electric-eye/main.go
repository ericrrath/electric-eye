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
	pollPeriod := flag.Duration("pollPeriod", 10*time.Minute, "duration between polls, e.g. '10m'")
	pollTimeout := flag.Duration("pollTimeout", 1*time.Minute, "request timeout when polling, e.g. '1m'")
	listenAddress := flag.String("listenAddress", ":8080", "address to listen on for HTTP metrics requests")
	numPollers := flag.Int("numPollers", 2, "number of concurrent pollers")
	env := flag.String("env", "", "'env' label added to each metric; allows aggregation of metrics from multiple electric-eye instances")
	uptimeRobotAPIKey := flag.String("uptimeRobotAPIKey", "", "UptimeRobot API key")
	fetchPeriod := flag.Duration("fetchPeriod", 10*time.Minute, "duration between fetches, e.g. '10m'")

	flag.Parse()

	monitorsByUrl := make(map[string]*util.Monitor)

	if len(*dataPath) > 0 {
		data, err := util.Load(*dataPath)
		if err != nil {
			klog.Fatalf("error loading monitor URLs from %s: %+v", *dataPath, err)
		}
		klog.Infof("loaded %d monitors from %s", len(data.Monitors), *dataPath)
		for i := range data.Monitors {
			m := data.Monitors[i]
			monitorsByUrl[m.TargetUrl] = &m
		}
	}

	found := make(chan string)
	pending := make(chan *util.Monitor)
	complete := make(chan *util.Result)

	// Start pollers which receive Monitor instances and send Result instances
	for i := 0; i < *numPollers; i++ {
		go util.Poller(i, pending, complete, *pollTimeout)
	}
	// Start a single publisher which receives Result instances and publishes them
	// as Prometheus metrics
	go util.Publisher(complete, *env)

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(*listenAddress, nil)

	// If an UptimeRobot API key was provided, start fetching URLs to monitor
	// and sending them to the `found` channel
	if len(*uptimeRobotAPIKey) > 0 {
		go func() {
			fetchTicker := time.NewTicker(*fetchPeriod)
			for ; true; <-fetchTicker.C {
				klog.Info("starting UptimeRobot fetch")
				count, err := fetch.Fetch(*uptimeRobotAPIKey, found)
				if err != nil {
					klog.Errorf("error fetching from UptimeRobot: %+v", err)
				}
				klog.Infof("fetched %d monitors from UptimeRobot", count)
			}
		}()
	}

	// Start a ticker; whenever it fires, send each monitor URL in the map to the
	// `pending` channel.  Otherwise, whenever the ticker isn't firing, listen
	// for new monitor URLs on the `found` channel, and add them to the map.
	pollTicker := time.Tick(*pollPeriod)
	for {
		select {
			case <-pollTicker:
				klog.Infof("sending %d monitors to pending queue", len(monitorsByUrl))
				for _, v := range monitorsByUrl {
					m := v
					pending <- m
				}
			case url := <-found:
				klog.V(4).Infof("received url: %s", url)
				m := util.NewMonitor(url)
				monitorsByUrl[m.TargetUrl] = m
			default:
				//nothing to do
		}
	}
}
