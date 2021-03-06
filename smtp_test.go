// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/smtp"
	"sync"
	"time"
)

type net_Conn struct {
	server  string
	latency time.Duration
	cmd     <-chan int
	pool    chan<- int
}

func (this net_Conn) Read(b []byte) (int, error) {
	time.Sleep(this.latency)
	if this.cmd != nil {
		<-this.cmd
	}
	return 0, nil
}
func (this net_Conn) Write(b []byte) (int, error) {
	time.Sleep(this.latency)
	if this.cmd != nil {
		<-this.cmd
	}
	return 0, nil
}
func (this net_Conn) Close() error {
	time.Sleep(this.latency)
	if this.cmd != nil {
		<-this.cmd
	}
	if this.pool != nil {
		select {
		case this.pool <- 1:
		default:
		}
	}
	return nil
}
func (this *net_Conn) LocalAddr() net.Addr                { return nil }
func (this *net_Conn) RemoteAddr() net.Addr               { return nil }
func (this *net_Conn) SetDeadline(t time.Time) error      { return nil }
func (this *net_Conn) SetReadDeadline(t time.Time) error  { return nil }
func (this *net_Conn) SetWriteDeadline(t time.Time) error { return nil }

type net_smtp_Client struct {
	conn         net_Conn
	errAuth      error
	errClose     error
	errData      error
	errExtension error
	errHello     error
	errMail      error
	errQuit      error
	errRcpt      error
	errReset     error
	errStartTLS  error
	errVerify    error
}

func (this *net_smtp_Client) Auth(a smtp.Auth) error {
	time.Sleep(this.conn.latency)
	return this.errAuth
}
func (this *net_smtp_Client) Close() error {
	this.conn.Close()
	return this.errClose
}
func (this *net_smtp_Client) Data() (io.WriteCloser, error) {
	time.Sleep(this.conn.latency)
	return nil, this.errData
}
func (this *net_smtp_Client) Extension(ext string) (bool, string) { return true, ext }
func (this *net_smtp_Client) Hello(localName string) error {
	time.Sleep(this.conn.latency)
	return this.errHello
}
func (this *net_smtp_Client) Mail(from string) error {
	time.Sleep(this.conn.latency)
	return this.errMail
}
func (this *net_smtp_Client) Quit() error {
	time.Sleep(this.conn.latency)
	this.Close()
	return this.errQuit
}
func (this *net_smtp_Client) Rcpt(to string) error { time.Sleep(this.conn.latency); return this.errRcpt }
func (this *net_smtp_Client) Reset() error         { time.Sleep(this.conn.latency); return this.errReset }
func (this *net_smtp_Client) StartTLS(config *tls.Config) error {
	time.Sleep(this.conn.latency)
	return this.errStartTLS
}
func (this *net_smtp_Client) Verify(addr string) error {
	time.Sleep(this.conn.latency)
	return this.errVerify
}

type net_DialTimeout_case struct {
	latency  time.Duration
	maxConns map[string]int
	sync     chan<- chan chan int
	err      error
}

type net_smtp_NewClient_case struct {
	clnt *net_smtp_Client
	err  error
}

type net_DialTimeout_state struct {
	connsMx sync.Mutex
	conns   map[string]chan int
}

func net_DialTimeout_fake(c *net_DialTimeout_case, s *net_DialTimeout_state) func(string, string, time.Duration) (net.Conn, error) {
	if c.err != nil {
		return func(string, string, time.Duration) (net.Conn, error) {
			time.Sleep(c.latency)
			return nil, c.err
		}
	} else {
		return func(proto string, server string, timeout time.Duration) (net.Conn, error) {
			var err error
			var ch chan int
			var ctl chan chan int
			time.Sleep(c.latency)
			if c.sync != nil {
				ctl = make(chan chan int, 1)
				c.sync <- ctl
			}
			cmd, ok := <-ctl
			if ok {
				if cmd == nil { // simulated timeout
					return nil, errors.New("Connection timeout")
				}
			} else { // simulated timeout
				return nil, errors.New("Aborted")
			}
			if c.maxConns != nil {
				if max, ok := c.maxConns[server]; ok {
					s.connsMx.Lock()
					defer s.connsMx.Unlock()
					ch, ok = s.conns[server]
					if !ok {
						ch = make(chan int, max)
						for i := 0; i < max; i++ {
							ch <- i
						}
						s.conns[server] = ch
					}
					select {
					case <-ch:
					default:
						err = errors.New("Connection refused")
					}
				}
			}
			if err != nil {
				return nil, err
			}
			return &net_Conn{server: server, latency: c.latency, cmd: cmd, pool: ch}, nil
		}
	}
}

func net_smtp_NewClient_fake(c *net_smtp_NewClient_case) func(net.Conn, string) (smtpClient, error) {
	return func(conn net.Conn, server string) (smtpClient, error) {
		if conn != nil {
			time.Sleep(conn.(*net_Conn).latency)
		}
		c.clnt.conn = *conn.(*net_Conn)
		if conn == nil {
			return nil, errors.New("No connection")
		}
		return c.clnt, c.err
	}
}

func net_DialTimeout_allOk(sync chan<- chan chan int) func(string, string, time.Duration) (net.Conn, error) {
	return net_DialTimeout_fake(
		&net_DialTimeout_case{
			latency:  10 * time.Millisecond,
			maxConns: nil,
			sync:     sync,
			err:      nil,
		},
		&net_DialTimeout_state{
			conns: make(map[string]chan int),
		})
}

func net_DialTimeout_1allOk(sync chan<- chan chan int) func(string, string, time.Duration) (net.Conn, error) {
	return net_DialTimeout_fake(
		&net_DialTimeout_case{
			latency:  10 * time.Millisecond,
			maxConns: map[string]int{"dummyhost:25": 1},
			sync:     sync,
			err:      nil,
		},
		&net_DialTimeout_state{
			conns: make(map[string]chan int),
		})
}

func net_smtp_NewClient_allOk() func(net.Conn, string) (smtpClient, error) {
	return net_smtp_NewClient_fake(
		&net_smtp_NewClient_case{
			clnt: &net_smtp_Client{},
			err:  nil,
		})
}
