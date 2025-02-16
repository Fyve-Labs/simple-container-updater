package main

import "github.com/prometheus/client_golang/prometheus"

type UpdateCollector struct {
}

func newUpdateCollector() *UpdateCollector {
	return &UpdateCollector{}
}

func (c *UpdateCollector) Describe(_ chan<- *prometheus.Desc) {

}

func (c *UpdateCollector) Collect(ch chan<- prometheus.Metric) {

}
