package ui

import (
	"kerykeion/pkg/ai"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

type Services struct {
	Config    *config.Config
	Identity  *identity.Manager
	Store     *db.Store
	Discovery *discovery.Discovery
	Messaging *messaging.Service
	Rooms     *room.Manager
	Transfers *transfer.Manager
	AI        *ai.Service
	Transport *transport.Manager
}
