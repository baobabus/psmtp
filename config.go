// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"
)

type Config struct {
	Smtps           bool
	Host            string
	Port            uint16
	TLSClientConfig *tls.Config
	MaxConns        int
	MaxIdleConns    int
	MaxLiveTime     time.Duration
	MaxIdleTime     time.Duration
	MinWaitTime     time.Duration
	MaxWaitTime     time.Duration
	Timeout         time.Duration
}

var errNoHost = errors.New("host not specified")

func (c *Config) Validate() error {
	if len(c.Host) == 0 {
		return errNoHost
	}
	if c.MaxConns < 0 {
		c.MaxConns = 1
	}
	if c.MaxIdleConns < 0 {
		c.MaxIdleConns = 0
	}
	if c.MaxConns < c.MaxIdleConns && c.MaxConns != 0 {
		c.MaxConns = c.MaxIdleConns
	}
	if c.MaxLiveTime < 0 {
		c.MaxLiveTime = 0
	}
	if c.MaxIdleTime < 0 {
		c.MaxIdleTime = 0
	}
	if c.MaxLiveTime < c.MaxIdleTime {
		c.MaxLiveTime = c.MaxIdleTime
	}
	if c.MinWaitTime < 0 {
		c.MinWaitTime = 0
	}
	if c.MaxWaitTime < 0 {
		c.MaxWaitTime = 0
	}
	if c.MaxWaitTime < c.MinWaitTime {
		c.MaxWaitTime = c.MinWaitTime
	}
	if c.MaxWaitTime < 0 {
		c.MaxWaitTime = 0
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("invalid timeout %v", c.Timeout)
	}
	return nil
}
