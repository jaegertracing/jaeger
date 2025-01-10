// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"io"
	"time"
)

// CronJob provides the functionality to run a job automatically
// and periodically in the same process but in different thread.
type CronJob struct {
	done     chan struct{}
	duration time.Duration
	rgs      func()
	io.Closer
}

// NewCronJob creates a simple cron job which will run periodically after the duration specified as first parameter.
// The other parameter is the function which needs to be registered to perform the job.
func NewCronJob(duration time.Duration, rgs func()) *CronJob {
	return &CronJob{
		duration: duration,
		rgs:      rgs,
		done:     make(chan struct{}),
	}
}

// Start will run register the cron job in another go routine which
// will wait for the duration periodically and perform the job.
func (c *CronJob) Start() {
	go c.register()
}

func (c *CronJob) register() {
	timeTicker := time.NewTicker(c.duration)
	defer timeTicker.Stop()
	for {
		select {
		case <-timeTicker.C:
			c.rgs()
		case <-c.done:
			return
		}
	}
}

// Close closes all the channels and free the resources acquired by cron job.
func (c *CronJob) Close() {
	close(c.done)
}
