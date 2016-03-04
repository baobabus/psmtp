// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"sync"
	"time"

	"github.com/baobabus/slog"
)

type Pool struct {
	Config
	cc                 *sync.Cond
	cpool              *cPool
	clsd               bool
	ftime              time.Time
	ccnt               int
	net_DialTimeout    func(string, string, time.Duration) (net.Conn, error)
	tls_DialTimeout    func(string, string, time.Duration, *tls.Config) (net.Conn, error)
	net_smtp_NewClient func(net.Conn, string) (smtpClient, error)
}

func NewPool(cfg Config) (*Pool, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Pool{Config: cfg, cc: sync.NewCond(&sync.Mutex{}), cpool: newCPool(cfg.MaxIdleConns)}, nil
}

func (this *Pool) Close() error {
	this.cc.L.Lock()
	defer this.cc.L.Unlock()
	if !this.clsd {
		this.closeIdle()
		this.clsd = true
	}
	this.cc.Broadcast()
	return nil
}

// Returns Mailer if one is available or if new connection is allowed.
// For newly created mailers dialing the connection may still be
// in progress.
// If the pool is exhausted, the method blocks until a connection
// becomes available.
func (this *Pool) GetMailer(auth smtp.Auth) (res *Mailer, err error) {
	slog.Trace(1).Printe("Getting mailer", "auth", auth)
	this.cc.L.Lock()
	for {
		if this.clsd {
			err = errors.New("cannot check out from closed pool")
			break
		}
		slog.Trace(2).Printe("Trying pool", "auth", auth, "cnt", this.ccnt)
		if conn, ok := this.popMailer(auth); ok {
			res, err = this.newMailer(conn, auth)
			break
		}
		slog.Trace(2).Printe("Trying new", "auth", auth, "cnt", this.ccnt)
		if this.MaxConns == 0 || this.ccnt < this.MaxConns {
			res, err = this.newMailer(nil, auth)
			break
		}
		slog.Trace(2).Printe("Trying any", "auth", auth, "cnt", this.ccnt)
		if conn, ok := this.popMailer(nil); ok {
			res, err = this.newMailer(conn, auth)
			break
		}
		slog.Trace(2).Printe("Waiting", "auth", auth, "cnt", this.ccnt)
		this.cc.Wait()
	}
	this.cc.L.Unlock()
	if err != nil {
		slog.On(err).Trace(1).Prints("Getting mailer", "auth", auth)
	} else {
		slog.Success().Trace(1).Prints("Getting mailer", "auth", auth)
	}
	return
}

func (this *Pool) popMailer(auth smtp.Auth) (res *psmtpConn, ok bool) {
	res = nil
	ok = true
	for res == nil && ok {
		if auth != nil {
			res, ok = this.cpool.Pop(auth)
		} else {
			res, ok = this.cpool.PopAny()
		}
		if ok {
			res = this.closeConnIfExp(res)
		}
	}
	return
}

func (this *Pool) newMailer(conn *psmtpConn, auth smtp.Auth) (*Mailer, error) {
	if conn != nil && conn.auth == auth {
		return &Mailer{conn: conn, ctl: nil, pool: this}, nil
	}
	now := time.Now()
	if now.Sub(this.ftime) < this.MinWaitTime {
		return nil, errors.New("connect blackout period")
	}
	if conn == nil {
		slog.Trace(3).Printe("Incrementing for new", "cnt", this.ccnt)
		this.ccnt++
		slog.Success().Trace(3).Prints("Incrementing for new", "cnt", this.ccnt)
	}
	ctl := make(chan *connResp, 1)
	go func(ctl chan<- *connResp) {
		defer close(ctl)
		ok := false
		defer func() {
			if !ok {
				this.cc.L.Lock()
				slog.Trace(3).Printe("Decrementing on failure", "cnt", this.ccnt)
				this.ccnt--
				slog.Success().Trace(3).Prints("Decrementing on failure", "cnt", this.ccnt)
				this.cc.L.Unlock()
				this.cc.Signal()
			}
		}()
		if conn != nil && conn.client != nil {
			// TODO Convert to glog
			//log.Printf("Closing old connection")
			conn.client.Quit()
		}
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
		ok = true
		ctl <- &connResp{
			conn: &psmtpConn{
				client: c,
				ok:     true,
				auth:   auth,
				ctime:  now,
				etime:  now.Add(this.MaxLiveTime),
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

// Puts Mailer back in the pool.
func (this *Pool) putMailer(mailer *Mailer) {
	slog.Trace(1).Printe("Putting mailer", "mailer", mailer)
	this.cc.L.Lock()
	defer this.cc.L.Unlock()
	conn := mailer.conn
	if conn != nil {
		conn = this.closeConnIfExp(conn)
	}
	if !this.clsd && conn != nil {
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
		if len(overflow) == 0 {
			this.cc.Signal()
		}
	}
	slog.Success().Trace(1).Prints("Putting mailer", "mailer", mailer)
}

func (this *Pool) closeIdle() {
	for {
		res, ok := this.cpool.PopAny()
		if !ok {
			break
		}
		this.closeConnQuiet(res)
	}
}

func (this *Pool) closeConnIfExp(conn *psmtpConn) *psmtpConn {
	slog.Trace(3).Printe("Checking connection", "conn", conn)
	now := time.Now()
	if !conn.ok || conn.etime.Before(now) {
		slog.With(errors.New("expired")).Trace(3).Prints("Checking connection", "conn", conn)
		this.closeConn(conn)
		return nil
	}
	slog.Success().Trace(3).Prints("Checking connection", "conn", conn)
	return conn
}

func (this *Pool) closeConn(conn *psmtpConn) {
	if conn.ok {
		conn.ok = false
		if conn.client != nil {
			go func(c *psmtpConn, p *Pool) {
				c.client.Quit()
				p.cc.L.Lock()
				slog.Trace(3).Printe("Async closing connection", "conn", conn, "cnt", p.ccnt)
				p.ccnt--
				slog.Success().Trace(3).Prints("Async closing connection", "conn", conn, "cnt", p.ccnt)
				p.cc.L.Unlock()
				p.cc.Signal()
			}(conn, this)
			return
		}
	}
	slog.Trace(3).Printe("Closing connection", "conn", conn, "cnt", this.ccnt)
	this.ccnt--
	slog.Success().Trace(3).Prints("Closing connection", "conn", conn, "cnt", this.ccnt)
	this.cc.Signal()
}

func (this *Pool) closeConnQuiet(conn *psmtpConn) {
	if conn.ok {
		conn.ok = false
		if conn.client != nil {
			go conn.client.Quit()
		}
	}
	this.ccnt--
}

type connResp struct {
	conn *psmtpConn
	err  error
}
