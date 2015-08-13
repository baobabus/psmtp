// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"net/smtp"
	"container/heap"
)

type cqItem struct {
	conn  *psmtpConn
	index  int
}

type _cQueue []*cqItem

type cPool struct {
	capacity int
	all      _cQueue
	byAuth   map[smtp.Auth]_cQueue
}

func newCPool(capacity int) *cPool {
	result := &cPool{
		capacity:   capacity,
		all:        make(_cQueue, 0, capacity),
		byAuth:     make(map[smtp.Auth]_cQueue, 0),
	}
	heap.Init(&result.all)
	return result
}

func (this *cPool) Push(conn *psmtpConn) []*psmtpConn {
	// Evict if needed
	var result []*psmtpConn
	if this == nil {
		return result
	}
	if this.capacity > 0 {
		for {
			if this.all.Len() < this.capacity {
				break;
			}
			if v, ok := this.PopAny(); ok {
				result = append(result, v)
			}
		}
		item := cqItem{conn: conn}
		heap.Push(&this.all, &item)
		sub, ok := this.byAuth[conn.auth]
		if !ok {
			sub = make(_cQueue, 0, this.capacity)
			heap.Init(&sub)
		}
		heap.Push(&sub, &item)
		this.byAuth[conn.auth] = sub
	} else {
		result = append(result, conn)
	}
	return result
}

// PopAny pops least recently used connection of all
// and correctly updates any by-auth queues.
// Returns ok = false if the pool is empty.
func (this *cPool) PopAny() (*psmtpConn, bool) {
	if this == nil {
		return nil, false
	}
	if this.all.Len() == 0 {
		return nil, false
	}
	result := heap.Pop(&this.all).(*cqItem)
	sub, ok := this.byAuth[result.conn.auth]
	if !ok {
		panic("missign by-auth queue")
	}
	if sub.Len() == 0 {
		panic("by-auth queue is empty")
	}
	check := heap.Pop(&sub).(*cqItem)
	this.byAuth[result.conn.auth] = sub
	if result.conn != check.conn {
		panic("mismatched items in all and by-auth queues")
	}
	return result.conn, true
}

// PopAny pops least recently used connection 
// with given auth and correctly updates global queue.
// Returns ok = false if no matching connection is available.
func (this *cPool) Pop(auth smtp.Auth) (*psmtpConn, bool) {
	if this == nil {
		return nil, false
	}
	sub, ok := this.byAuth[auth]
	if !ok || sub.Len() == 0 {
		return nil, false
	}
	result := heap.Pop(&sub).(*cqItem)
	this.byAuth[auth] = sub
	found := false
	for i, v := range this.all {
		if v == result {
			found = true
			heap.Remove(&this.all, i)
			break
		}
	}
	if !found {
		panic("item missing from all queue")
	}
	return result.conn, true
}

func (this _cQueue) Len() int { return len(this) }

func (this _cQueue) Less(i, j int) bool {
	// We want Pop to give us the least recently used.
	return this[j].conn.atime.Before(this[i].conn.atime)
}

func (this _cQueue) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
	this[i].index = i
	this[j].index = j
}

func (this *_cQueue) Push(x interface{}) {
	n := len(*this)
	item := x.(*cqItem)
	item.index = n
	*this = append(*this, item)
}

func (this *_cQueue) Pop() interface{} {
	old := *this
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*this = old[0 : n-1]
	return item
}
