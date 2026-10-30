package main

import (
	"github.com/dmmoody/poz-pay/internal/broker/rabbitmq"
	"github.com/dmmoody/poz-pay/internal/outbox"
	"github.com/dmmoody/poz-pay/internal/registerupload"
)

var (
	_ outbox.Publisher        = (*rabbitmq.Client)(nil)
	_ registerupload.Listener = (*rabbitmq.Client)(nil)
)
