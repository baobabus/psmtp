// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"errors"
	"fmt"
	"sync"
)

type Mailer struct {
	mx   sync.Mutex
	conn *psmtpConn
	ctl  chan *connResp
	pool *Pool
	clsd bool
}

// Sends the message via Mailer's SMTP connection.
// The connection is reset via SMTP RSET command
// before and after sending, as well as after any error.
// A failure to send a message does not necessarily
// invalidates the connection.
func (this *Mailer) Send(msg *Message) error {
	this.mx.Lock()
	defer this.mx.Unlock()
	if this.clsd {
		return errors.New("closed")
	}
	if err := this.waitForConn(); err != nil {
		return err
	}
	if !this.conn.ok || this.conn.client == nil {
		return errors.New("connection closed")
	}
	if err := this.validate(msg); err != nil {
		return err
	}
	c := this.conn.client
	// TODO Convert to glog
	//log.Printf("Resetting %s...", this.server())
	if err := c.Reset(); err != nil {
		return err
	}
	defer c.Reset()
	// TODO Convert to glog
	//log.Printf("Sending Mail to %s...", this.server())
	if err := c.Mail(msg.smtpFrom()); err != nil {
		return err
	}
	// TODO Convert to glog
	//log.Printf("Sending Rcpt to %s...", this.server())
	for _, rcpt := range msg.smtpRcpt() {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	// TODO Convert to glog
	//log.Printf("Starting data for %s...", this.server())
	wc, err := c.Data()
	if err != nil {
		return err
	}
	// TODO Convert to glog
	//log.Printf("Writing headers to %s...", this.server())
	for h, vs := range msg.Header {
		for _, v := range vs {
			if _, err = fmt.Fprintf(wc, "%s: %s\n", h, v); err != nil {
				return err
			}
		}
	}
	if _, err = fmt.Fprintf(wc, "\n"); err != nil {
		return err
	}
	// TODO Convert to glog
	//log.Printf("Writing body to %s...", this.server())
	if _, err := wc.Write(msg.Body); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	// TODO Convert to glog
	//log.Printf("Done sending to %s...", this.server())
	return nil
}

// Closes the Mailer and returns it to the pool.
// No further sending is possible after Mailer is closed.
func (this *Mailer) Close() error {
	this.mx.Lock()
	defer this.mx.Unlock()
	if this.clsd {
		return errors.New("closed")
	}
	// TODO Make waiting for connection and closing asynchronous
	this.waitForConn()
	if this.pool == nil {
		return errors.New("closing mailer with no pool")
	}
	this.pool.putMailer(this)
	this.conn = nil
	this.ctl = nil
	this.pool = nil
	this.clsd = true
	return nil
}

func (this *Mailer) waitForConn() error {
	if this.conn == nil && this.ctl != nil {
		r := <-this.ctl
		this.ctl = nil
		if r.err != nil {
			return r.err
		}
		this.conn = r.conn
	}
	if this.conn == nil && this.ctl == nil {
		return errors.New("no connection and no source")
	}
	return nil
}

// Validates sender and recipients and returns an error if
// any are missing or invalid. Headers and body
// are allowed to be empty.
func (this *Mailer) validate(msg *Message) error {
	if msg == nil {
		return errors.New("No message")
	}
	if msg.From.Address == "" {
		return errors.New("Empty sender")
	}
	if len(msg.To) == 0 {
		return errors.New("No recipients")
	}
	for _, r := range msg.To {
		if r.Address == "" {
			return errors.New("Empty recipient")
		}
	}
	return nil
}

func (this *Mailer) server() string {
	return fmt.Sprintf("%s:%s", this.pool.Host, this.pool.Port)
}
