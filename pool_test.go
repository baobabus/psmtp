// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"net/mail"
	"testing"
	"time"
)

func tc_pool1(mc, mi int) (*Pool, chan chan chan int) {
	ch := make(chan chan chan int, 20)
	res, _ := NewPool(Config{
		Host:            "dummyhost",
		Port:            25,
		TLSClientConfig: nil,
		MaxConns:        mc,
		MaxIdleConns:    mi,
		Timeout:         50 * time.Millisecond,
	})
	res.net_DialTimeout = net_DialTimeout_1allOk(ch)
	res.net_smtp_NewClient = net_smtp_NewClient_allOk()
	return res, ch
}

var (
	msg1 = Message{
		From:   mail.Address{Address: "sndr@foo.com"},
		To:     []mail.Address{mail.Address{Address: "sndr@foo.com"}},
		Header: map[string][]string{"From": []string{"sndr@foo.com"}, "Subject": []string{"Test"}},
		Body:   []byte("Test test test"),
	}
)

func TestGetMailer(t *testing.T) {
	var sbj *Pool
	var cch chan chan chan int
	var s chan chan int
	var c, c1, c2 chan int
	var m, m1, m2 *Mailer
	var err error

	sbj, cch = tc_pool1(0, 0)
	m, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	err = m.Close()
	c <- 1
	if err != nil {
		t.Error(err)
		return
	}
	sbj.Close()
	close(cch)

	sbj, cch = tc_pool1(0, 0)
	m, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	m, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	sbj.Close()
	close(cch)

	sbj, cch = tc_pool1(1, 0)
	m, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	err = m.Close()
	c <- 1
	if err != nil {
		t.Error(err)
		return
	}
	m, err = sbj.GetMailer(nil)
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	err = m.Close()
	c <- 1
	if err != nil {
		t.Error(err)
		return
	}
	sbj.Close()
	close(cch)

	sbj, cch = tc_pool1(1, 1)
	m, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	err = m.Close()
	c <- 1
	if err != nil {
		t.Error(err)
		return
	}
	m, err = sbj.GetMailer(nil)
	s = <-cch
	c = make(chan int)
	s <- c
	close(s)
	if err != nil {
		t.Error(err)
		return
	}
	if m == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	err = m.Close()
	c <- 1
	if err != nil {
		t.Error(err)
		return
	}
	sbj.Close()
	close(cch)

	sbj, cch = tc_pool1(2, 0)
	m1, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m1 == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c1 = make(chan int)
	s <- c1
	close(s)
	err = m1.Close()
	c1 <- 1
	if err != nil {
		t.Error(err)
		return
	}
	m2, err = sbj.GetMailer(nil)
	s = <-cch
	c2 = make(chan int)
	s <- c2
	close(s)
	if err != nil {
		t.Error(err)
		return
	}
	if m2 == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	err = m2.Close()
	c2 <- 1
	if err != nil {
		t.Error(err)
		return
	}
	sbj.Close()
	close(cch)

	sbj, cch = tc_pool1(2, 0)
	m1, err = sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	if m1 == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	s = <-cch
	c1 = make(chan int, 1)
	s <- c1
	close(s)
	m2, err = sbj.GetMailer(nil)
	s = <-cch
	c2 = make(chan int, 1)
	s <- c2
	close(s)
	if err != nil {
		t.Error(err)
		return
	}
	if m2 == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	// Expecting "connection refused" delayed due to async dialing
	err = m2.Close()
	c2 <- 1
	if err == nil {
		t.Errorf("Expected error, but was successful")
		return
	}
	err = m1.Close()
	c1 <- 1
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(200 * time.Millisecond)
	m2, err = sbj.GetMailer(nil)
	s = <-cch
	c2 = make(chan int, 1)
	s <- c2
	close(s)
	if err != nil {
		t.Error(err)
		return
	}
	if m2 == nil {
		t.Errorf("Expected non-nil value")
		return
	}
	err = m2.Close()
	c2 <- 1
	if err != nil {
		t.Error(err)
		return
	}
	sbj.Close()
	close(cch)

}

func _TestPool10(t *testing.T) {
	sbj, _ := NewPool(Config{
		Host:            "dummyhost",
		Port:            25,
		TLSClientConfig: nil,
		MaxConns:        1,
		MaxIdleConns:    0,
		Timeout:         50 * time.Millisecond,
	})
	testGR(sbj, t)
}

func _TestPool11(t *testing.T) {
	sbj, _ := NewPool(Config{
		Host:            "dummyhost",
		Port:            25,
		TLSClientConfig: nil,
		MaxConns:        2,
		MaxIdleConns:    2,
		Timeout:         50 * time.Millisecond,
	})
	testGR(sbj, t)
}

func testGR(sbj *Pool, t *testing.T) {
	t.Logf("Pool %#v", sbj)

	sbj.net_DialTimeout = net_DialTimeout_1allOk(nil)
	sbj.net_smtp_NewClient = net_smtp_NewClient_allOk()

	m, err := sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(200 * time.Millisecond)
	err = m.Close()
	if err != nil {
		t.Error(err)
		return
	}

	m1, err := sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	go func(m *Mailer) {
		time.Sleep(200 * time.Millisecond)
		err = m.Close()
		if err != nil {
			t.Error(err)
			return
		}
	}(m1)
	m2, err := sbj.GetMailer(nil)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(200 * time.Millisecond)
	err = m2.Close()
	if err != nil {
		t.Error(err)
		return
	}

	sbj.Close()
	m, err = sbj.GetMailer(nil)
	if err == nil {
		t.Errorf("Expected error, but was successful")
		return
	}
}
