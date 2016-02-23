// Copyright 2015 Aleksey Blinov.  All rights reserved.

package psmtp

import (
	"net/mail"
)

type Message struct {
	From   mail.Address
	To     []mail.Address
	Header mail.Header
	Body   []byte
}

func (this *Message) smtpFrom() string {
	return this.From.Address
}

func (this *Message) smtpRcpt() []string {
	result := make([]string, len(this.To))
	for i, v := range this.To {
		result[i] = v.Address
	}
	return result
}
