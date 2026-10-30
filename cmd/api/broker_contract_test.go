package main

import (
	"github.com/dmmoody/poz-pay/internal/broker"
	"github.com/dmmoody/poz-pay/internal/broker/rabbitmq"
)

var (
	_ broker.Publisher = (*rabbitmq.Client)(nil)
	_ broker.Listener  = (*rabbitmq.Client)(nil)
)
