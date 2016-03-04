// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"crypto/tls"
	"io"
	"net/smtp"
	"time"
)

// Interface extracted from smtp.Client to
// aid with plugging in alternative implementations
// and unit testing harnesses.
type smtpClient interface {
	Auth(a smtp.Auth) error
	Close() error
	Data() (io.WriteCloser, error)
	Extension(ext string) (bool, string)
	Hello(localName string) error
	Mail(from string) error
	Quit() error
	Rcpt(to string) error
	Reset() error
	StartTLS(config *tls.Config) error
	Verify(addr string) error
}

type psmtpConn struct {
	client smtpClient
	ok     bool
	auth   smtp.Auth
	ctime  time.Time
	etime  time.Time
	atime  time.Time
}
