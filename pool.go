// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"sync"
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
	atime  time.Time
}

type connResp struct {
	conn *psmtpConn
	err  error
}

type Pool struct {
	Smtps           bool
	Host            string
	Port            uint16
	TLSClientConfig *tls.Config
	MaxConns        int
	MaxIdleConns    int
	MaxLiveTime     time.Duration
	MaxIdleTime     time.Duration
	MinWaitTime     time.Duration
	Timeout         time.Duration

	ftime time.Time

	cmx   sync.Mutex
	cpool *cPool
	ccnt  int
	cntfy chan *connResp

	clsd bool

	net_DialTimeout    func(string, string, time.Duration) (net.Conn, error)
	tls_DialTimeout    func(string, string, time.Duration, *tls.Config) (net.Conn, error)
	net_smtp_NewClient func(net.Conn, string) (smtpClient, error)
}

func (this *Pool) Close() error {
	this.cmx.Lock()
	defer this.cmx.Unlock()
	if !this.clsd {
		// TODO Prevent new mailer check-outs
		// TODO Close all idle connections
		// TODO Close connections from all returned mailers
		if this.cntfy != nil {
			close(this.cntfy)
			this.cntfy = nil
		}
		this.clsd = true
	}
	return nil
}

// Returns Mailer if one is available or if new connection is allowed.
// For newly created mailers dialing the connection may still be
// in progress.
// If the pool is exhausted, the method blocks until a connection
// becomes available.
func (this *Pool) GetMailer(auth smtp.Auth) (*Mailer, error) {
	for {
		if this.clsd {
			return nil, errors.New("Pool is closed")
		}
		result, ctl, err := this.tryMailer(auth)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return result, nil
		}
		if ctl == nil {
			panic("No control channel")
		}
		// TODO Implement this with unbuffered channel
		// TODO Add arbitration based on returned connection properties
		<-ctl
	}
}

// Puts Mailer back in the pool.
func (this *Pool) putMailer(mailer *Mailer) {
	this.cmx.Lock()
	defer this.cmx.Unlock()
	conn := mailer.conn
	this.checkExp(conn)
	if !this.clsd && conn != nil && conn.ok {
		// return to idle pool
		overflow := this.cpool.Push(conn)
		// this should be empty or only have one item,
		// unless pool resizing took place for some reason
		for _, v := range overflow {
			// TODO implement connection wait queue
			// and return a connection instead of closing it
			// if there are waiters
			this.closeConn(v)
		}
		// This may have been returned in overflow and closed if maxIdleConns is 0
		if conn.ok {
			this.notifyCh() <- &connResp{conn, nil}
		}
	} else {
		if conn == nil {
			// TODO Convert to glog
			//log.Printf("Pool.putMailer(): returned mailer with no connection: %v.\n", conn)
		}
		// TODO Deal with mailers for which connection attempts are still pending
		this.ccnt--
		if !this.clsd {
			this.notifyCh() <- nil
		}
	}
}

func (this *Pool) tryMailer(auth smtp.Auth) (*Mailer, <-chan *connResp, error) {
	this.cmx.Lock()
	defer this.cmx.Unlock()
	// Try idle pool
	if !this.clsd && this.cpool == nil {
		this.cpool = newCPool(this.MaxIdleConns)
	}
	if conn, ok := this.cpool.Pop(auth); ok {
		return &Mailer{conn: conn, ctl: nil, pool: this}, nil, nil // TODO add ctl channel
	}
	if this.MaxConns == 0 || this.ccnt < this.MaxConns {
		// Try dialing
		result, err := this.dial(auth)
		if err == nil {
			this.ccnt++
		}
		return result, nil, err
	} else {
		// Try closing idle connection and re-dialing
		conn, ok := this.cpool.PopAny()
		if !ok {
			// Not en error. We just can't provide anything at the moment.
			// Return a channel for notifying when a connection becomes
			// available or when dialing becomes possible.
			return nil, this.notifyCh(), nil
		}
		this.checkExp(conn)
		if conn.ok {
			if conn.auth == auth {
				return &Mailer{conn: conn, ctl: nil, pool: this}, nil, nil
			}
			this.closeConn(conn)
		}
		// Dialing may become possible once the connection closing is complete
		return nil, this.notifyCh(), nil
		// // Try dialing in place of popped connection
		// result, err := this.dial(auth)
		// if err != nil {
		// 	this.ccnt--
		// }
		// return result, nil, err
	}
}

// Caller must hold cmx lock
func (this *Pool) notifyCh() chan *connResp {
	if !this.clsd && this.cntfy == nil {
		// TODO Make buffer size configurable
		this.cntfy = make(chan *connResp, 2)
	}
	return this.cntfy
}

// Caller must hold cmx lock
func (this *Pool) checkExp(conn *psmtpConn) {
	now := time.Now()
	if conn != nil && conn.ok && conn.ctime.Add(this.MaxLiveTime).Before(now) {
		this.closeConn(conn)
	}
}

// Caller must hold cmx lock
func (this *Pool) dial(auth smtp.Auth) (*Mailer, error) {
	now := time.Now()
	if now.Sub(this.ftime) < this.MinWaitTime {
		return nil, errors.New("connect blackout period")
	}
	ctl := make(chan *connResp, 1)
	go func(ctl chan<- *connResp) {
		defer close(ctl)
		server := fmt.Sprintf("%s:%d", this.Host, this.Port)
		// TODO Convert to glog
		//log.Printf("Dialing %s...", server)
		if this.net_DialTimeout == nil {
			this.net_DialTimeout = net.DialTimeout
		}
		if this.tls_DialTimeout == nil {
			this.tls_DialTimeout = func(network string, addr string, timeout time.Duration, tlsc *tls.Config) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: timeout}
				return tls.DialWithDialer(dialer, network, addr, tlsc)
			}
		}
		var l net.Conn
		var err error
		if this.Smtps {
			var tlsc tls.Config
			if this.TLSClientConfig != nil {
				tlsc = *this.TLSClientConfig
			}
			l, err = this.tls_DialTimeout("tcp", server, this.Timeout, &tlsc)
		} else {
			l, err = this.net_DialTimeout("tcp", server, this.Timeout)
		}
		if err != nil {
			// TODO Convert to glog
			//log.Printf("Error dialing %s: %v", server, err)
			this.ftime = now
			ctl <- &connResp{nil, err}
			return
		}
		if this.net_smtp_NewClient == nil {
			this.net_smtp_NewClient = func(conn net.Conn, server string) (smtpClient, error) {
				return smtp.NewClient(conn, server)
			}
		}
		c, err := this.net_smtp_NewClient(l, this.Host)
		if err != nil {
			this.ftime = now
			ctl <- &connResp{nil, err}
			return
		}
		hostname, _ := os.Hostname()
		if err = c.Hello(hostname); err != nil {
			c.Quit()
			ctl <- &connResp{nil, err}
			return
		}
		if this.TLSClientConfig != nil && !this.Smtps {
			tlsc := *this.TLSClientConfig
			// TODO Convert to glog
			//log.Printf("Starting TLS on %s...", server)
			if err = c.StartTLS(&tlsc); err != nil {
				this.ftime = now
				c.Quit()
				ctl <- &connResp{nil, err}
				return
			}
		}
		if auth != nil {
			// TODO Convert to glog
			//log.Printf("Authenticating with %s...", server)
			if err = c.Auth(auth); err != nil {
				c.Quit()
				ctl <- &connResp{nil, err}
				return
			}
		}
		ctl <- &connResp{
			conn: &psmtpConn{
				client: c,
				ok:     true,
				auth:   auth,
				ctime:  now,
				atime:  now,
			},
			err: nil,
		}
	}(ctl)
	return &Mailer{
		conn: nil,
		ctl:  ctl,
		pool: this,
	}, nil
}

// Caller must hold cmx lock
func (this *Pool) closeConn(conn *psmtpConn) {
	if conn.ok {
		// TODO Convert to glog
		//log.Printf("%p Pool.closeConn()", this)
		conn.ok = false
		if conn.client != nil {
			go func(c smtpClient, p *Pool) {
				c.Quit()
				p.cmx.Lock()
				defer p.cmx.Unlock()
				p.ccnt--
				p.notifyCh() <- nil
			}(conn.client, this)
		} else {
			// TODO Convert to glog
			//log.Printf("psmtpConn.close(): No client to close.")
			this.ccnt--
		}
	}
}
