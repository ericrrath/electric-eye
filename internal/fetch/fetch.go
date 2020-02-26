package fetch

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"time"
)

const (
	UptimeRobotAPIURL = "https://api.uptimerobot.com/v2/getMonitors"
	Interval = 5 * time.Second
	Timeout = 10 * time.Second
)

type Pagination struct {
	Offset int
	Limit int
	Total int
}

type Monitor struct {
	URL string
}

type GetMonitorsResponse struct {
	Pagination Pagination
	Monitors []Monitor
}

func Fetch(uptimeRobotAPIKey string, out chan<- string) error {
	client := resty.New()
	client.SetTimeout(Timeout)

	offset := 0
	for {
		request := client.R()
		request.SetHeader("content-type", "application/x-www-form-urlencoded")
		request.SetHeader("cache-control", "no-cache")
		body := fmt.Sprintf("api_key=%s&format=json&limit=50&offset=%d", uptimeRobotAPIKey, offset)
		request.SetBody(body)
		request.SetResult(&GetMonitorsResponse{})

		response, err := request.Post(UptimeRobotAPIURL)
		if err != nil {
			return err
		}
		responseObj := response.Result().(*GetMonitorsResponse)
		for _,m := range responseObj.Monitors {
			out <- m.URL
		}
		offset += len(responseObj.Monitors)
		if offset >= responseObj.Pagination.Total {
			break
		}
		time.Sleep(Interval)
	}

    return nil
}
