// SPDX-License-Identifier: MPL-2.0
package fetch

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
	"k8s.io/klog/v2"
)

const (
	UptimeRobotAPIURL = "https://api.uptimerobot.com/v2/getMonitors"
	Interval          = 1 * time.Second
	Timeout           = 30 * time.Second
)

type Pagination struct {
	Offset int
	Limit  int
	Total  int
}

type Monitor struct {
	URL string
}

type GetMonitorsResponse struct {
	Pagination Pagination
	Monitors   []Monitor
}

// Fetching the URLs to be monitored from UptimeRobot is easy; finding the best way
// to add them to the map of monitors has been difficult.  Keep in mind that
// UptimeRobot API enforces a constraint: you can only fetch 50 monitors per
// request, not more.
//
// Initialy I tried fetching a batch of URLs, then sending each URL one by one to
// the `found` channel (i.e. `chan string`).  This worked fine until the map of
// monitors was large enough that polling all of them took longer than a poll tick
// (default 1m).  At that point, only monitor could be added to the map during each
// poll cycle.
//
// Next I tried changing `found` to `chan[] string`, and sending each batch of 50
// (or less for the final batch).  This worked better, but was still subject to the
// same consequence that only batch could be added per poll cycle.
//
// So now I'm fetching *all* the URLs to be monitored (w/ as many requests as
// necessary), and sending them to the `found` channel in one (very) large batch.
func Fetch(uptimeRobotAPIKey string, out chan<- []string) (count int, err error) {
	client := resty.New()
	client.SetTimeout(Timeout)

	var offset int
	var urls []string
	for {
		request := client.R()
		request.SetHeader("content-type", "application/x-www-form-urlencoded")
		request.SetHeader("cache-control", "no-cache")
		body := fmt.Sprintf("api_key=%s&format=json&limit=50&offset=%d", uptimeRobotAPIKey, offset)
		request.SetBody(body)
		request.SetResult(&GetMonitorsResponse{})

		response, err := request.Post(UptimeRobotAPIURL)
		if err != nil {
			return count, err
		}
		responseObj := response.Result().(*GetMonitorsResponse)
		klog.V(2).Infof("received %d (offset %d) of %d monitors from UptimeRobot", len(responseObj.Monitors), responseObj.Pagination.Offset, responseObj.Pagination.Total)
		for _, m := range responseObj.Monitors {
			urls = append(urls, m.URL)
			count++
		}
		offset += len(responseObj.Monitors)
		if offset >= responseObj.Pagination.Total {
			break
		}
		time.Sleep(Interval)
	}
	out <- urls

	return count, nil
}
