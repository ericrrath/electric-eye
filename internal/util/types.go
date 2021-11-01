// SPDX-License-Identifier: MPL-2.0
package util

import (
	"encoding/json"
	"io/ioutil"
	"time"
)

type Monitor struct {
	TargetUrl string `json:"targetUrl"`
	Method    string `json:"method,omitempty"`
	// TTL logic is enforced on monitors with non-nil timestamps
	Timestamp *time.Time
}

type Data struct {
	Monitors []Monitor `json:"monitors"`
}

func NewMonitor(url string) *Monitor {
	mon := Monitor{TargetUrl: url}
	return &mon
}

type Result struct {
	Target          string
	Success         bool
	ResponseTime    time.Duration
	SSLValidityDays int
	RequestTime     time.Time
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
