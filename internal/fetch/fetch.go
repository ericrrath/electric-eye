package fetch

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"k8s.io/klog"
	"time"
)

const (
	UptimeRobotAPIURL = "https://api.uptimerobot.com/v2/getMonitors"
	Interval          = 5 * time.Second
	Timeout           = 10 * time.Second
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

func Fetch(uptimeRobotAPIKey string, out chan<- string) (count int, err error) {
	client := resty.New()
	client.SetTimeout(Timeout)

	var offset int
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
		klog.Infof("received %d (offset %d) of %d monitors from UptimeRobot", len(responseObj.Monitors), responseObj.Pagination.Offset, responseObj.Pagination.Total)
		for _, m := range responseObj.Monitors {
			out <- m.URL
			count++
		}
		offset += len(responseObj.Monitors)
		if offset >= responseObj.Pagination.Total {
			break
		}
		time.Sleep(Interval)
	}

	return count, nil
}
