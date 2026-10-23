package main

import (
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
)

type frameDispatcher struct {
	messages  *messaging.Service
	rooms     *room.Manager
	transfers *transfer.Manager
}

func (d frameDispatcher) handle(connID string, frameType byte, payload []byte, remotePeerID string) {
	msgSvc, roomMgr, tferMgr := d.messages, d.rooms, d.transfers

	switch frameType {
	case protocol.TypeMsg:
		msgSvc.HandleIncomingMsg(payload, remotePeerID)
	case protocol.TypeMsgAck:
		msgSvc.HandleIncomingMsgAck(payload)
	case protocol.TypeRoomJoin:
		roomMgr.HandleRoomJoin(payload)
	case protocol.TypeRoomLeave:
		roomMgr.HandleRoomLeave(payload)
	case protocol.TypeRoomClosed:
		roomMgr.HandleRoomClosed(payload)
	case protocol.TypeRoomTransfer:
		roomMgr.HandleRoomTransfer(payload)
	case protocol.TypeRoomSyncReq:
		roomMgr.HandleRoomSyncReq(payload, remotePeerID)
	case protocol.TypeRoomSyncRes:
		roomMgr.HandleRoomSyncRes(payload)
	case protocol.TypeFileOffer:
		tferMgr.HandleFileOffer(connID, payload, remotePeerID)
	case protocol.TypeFileAccept:
		tferMgr.HandleFileAccept(payload)
	case protocol.TypeChunk:
		tferMgr.HandleChunk(connID, payload, remotePeerID)
	case protocol.TypeFileDone:
		tferMgr.HandleFileDone(payload)
	case protocol.TypeFileAnnounce:
		tferMgr.HandleFileAnnounce(payload)
	case protocol.TypeFileWhoHas:
		tferMgr.HandleFileWhoHas(payload, remotePeerID)
	case protocol.TypeFileHave:
		tferMgr.HandleFileHave(payload)
	}

}
