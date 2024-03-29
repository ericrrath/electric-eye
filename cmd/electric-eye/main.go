// SPDX-FileCopyrightText: 2021 Eric R. Rath
// SPDX-License-Identifier: MPL-2.0

// Package main provides the entrypoint for the application, and all the
// high-level organization.
//
// The application performs two essential operations, and one optional
// operation.  First, it polls a collection of monitors for "up" status,
// response time, and TLS certificate validity.  Second, it publishes a set of
// Prometheus metrics describing those polling results (so that a Prometheus
// instance can scrape those metrics and fire alerts when monitors go down or
// need new TLS certs).  And optionally, if an UptimeRobot API key is provided,
// it fetches the set of all monitors to poll from UptimeRobot and adds them to the
// collection of monitors to poll.
//
// The three operations are performed by different sets of goroutines, and
// channels are used to coordinate work by those goroutines.  To make it easier
// to understand how the channels and goroutines work together, all channels and
// goroutines are created and "wired together" in this package.
package main

import (
	"flag"
	"net/http"
	"time"

	"github.com/ericrrath/electric-eye/internal/fetch"
	"github.com/ericrrath/electric-eye/internal/poll"
	"github.com/ericrrath/electric-eye/internal/publish"
	"github.com/ericrrath/electric-eye/internal/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

func main() {
	monitorsPath := flag.String("monitorsPath", "", "path to monitors file, e.g. /tmp/monitors.json")
	pollPeriod := flag.Duration("pollPeriod", 1*time.Minute, "duration between polls, default '1m'")
	pollTimeout := flag.Duration("pollTimeout", 30*time.Second, "request timeout when polling, default '30s'")
	listenAddress := flag.String("listenAddress", ":8080", "address to listen on for HTTP metrics requests, default ':8080'")
	numPollers := flag.Int("numPollers", 10, "number of concurrent pollers, default 10")
	env := flag.String("env", "", "'env' label added to each metric; allows aggregation of metrics from multiple electric-eye instances")
	uptimeRobotAPIKey := flag.String("uptimeRobotAPIKey", "", "UptimeRobot API key")
	fetchPeriod := flag.Duration("fetchPeriod", 10*time.Minute, "duration between fetches, default '10m'")

	klog.InitFlags(nil)
	flag.Parse()

	// Ensure the limit on open file descriptors for this process is twice the number of pollers.  I'll probably need
	// to adjust this further as I get a better understanding of how many network connections are used.
	limit := uint64(2 * *numPollers)
	if err := util.EnsureFileDescriptorLimit(limit); err != nil {
		klog.Errorf("error setting process file descriptor limit to %d: %v", limit, err)
	}

	monitorsByUrl := make(map[string]*util.Monitor)

	if len(*monitorsPath) > 0 {
		data, err := util.Load(*monitorsPath)
		if err != nil {
			klog.Fatalf("error loading monitor URLs from %s: %+v", *monitorsPath, err)
		}
		klog.Infof("loaded %d monitors from %s", len(data.Monitors), *monitorsPath)
		for i := range data.Monitors {
			m := data.Monitors[i]
			monitorsByUrl[m.TargetUrl] = &m
		}
	}

	found := make(chan []string)
	pending := make(chan *util.Monitor)
	complete := make(chan *util.Result)

	// Start pollers which receive Monitor instances and send Result instances
	for i := 0; i < *numPollers; i++ {
		go poll.Poller(i, pending, complete, *pollTimeout)
	}
	// Start a single publisher which receives Result instances and publishes them
	// as Prometheus metrics
	go publish.Publisher(complete, *env)

	http.Handle("/metrics", promhttp.Handler())
	go func(addr string) {
		if err := http.ListenAndServe(addr, nil); err != nil {
			klog.Fatalf("error listening for metrics on %s: %v", addr, err)
		}
	}(*listenAddress)

	// If an UptimeRobot API key was provided, start fetching URLs to monitor
	// and sending them to the `found` channel
	if len(*uptimeRobotAPIKey) > 0 {
		go func() {
			fetchTicker := time.NewTicker(*fetchPeriod)
			for ; true; <-fetchTicker.C {
				klog.V(2).Info("starting UptimeRobot fetch")
				count, err := fetch.Fetch(*uptimeRobotAPIKey, found)
				if err != nil {
					klog.Errorf("error fetching from UptimeRobot: %+v", err)
				}
				klog.V(2).Infof("fetched %d monitors from UptimeRobot", count)
			}
		}()
	}

	// Send locally-sourced monitors to the `pending` channel now instead of
	// waiting for the first tick
	sendMonitors(monitorsByUrl, pending)

	// Start a ticker; whenever it fires, send all monitor URLs in the map to
	// the `pending` channel.  Otherwise, whenever the ticker isn't firing,
	// listen for new monitor URLs on the `found` channel, and add them to the
	// map.  By having one thread perform both iteration and map mutation, we
	// avoid concurrency problems.
	pollTicker := time.Tick(*pollPeriod)
	for {
		select {
		case <-pollTicker:
			sent, elapsed := sendMonitors(monitorsByUrl, pending)
			if elapsed.Milliseconds() > pollPeriod.Milliseconds() {
				klog.Warningf("sending %d monitors to pending channel took %v (longer than pollPeriod %v)", sent, elapsed, pollPeriod)
			}
		case urls := <-found:
			now := time.Now()
			// Find monitors older than 30m
			cutoff := now.Add(-30 * time.Minute)
			var toRemove []string
			for _, m := range monitorsByUrl {
				if m.Timestamp != nil && m.Timestamp.Before(cutoff) {
					toRemove = append(toRemove, m.TargetUrl)
				}
			}
			// Remove them from the map
			for _, url := range toRemove {
				delete(monitorsByUrl, url)
			}
			// Put the new monitors in the map with timestamp now
			for _, url := range urls {
				klog.V(4).Infof("received url: %s", url)
				m := util.NewMonitor(url)
				m.Timestamp = &now
				monitorsByUrl[m.TargetUrl] = m
			}
		}
	}
}

func sendMonitors(monitors map[string]*util.Monitor, destination chan *util.Monitor) (int, time.Duration) {
	klog.V(2).Infof("sending %d monitors to pending queue", len(monitors))
	start := time.Now()
	for _, v := range monitors {
		m := v
		destination <- m
	}
	return len(monitors), time.Since(start)
}
