package main

import (
	"kerykeion/pkg/ai"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

type frameDispatcher struct {
	messages  *messaging.Service
	rooms     *room.Manager
	transfers *transfer.Manager
	aiSvc     *ai.Service
	transport *transport.Manager
}

func (d frameDispatcher) handle(connID string, frameType byte, payload []byte, remotePeerID string) {
	msgSvc, roomMgr, tferMgr := d.messages, d.rooms, d.transfers

	switch frameType {
	case protocol.TypeMsg:
		msgSvc.HandleIncomingMsg(payload, remotePeerID)
	case protocol.TypeMsgAck:
		msgSvc.HandleIncomingMsgAck(payload, remotePeerID)
	case protocol.TypeRoomJoin:
		roomMgr.HandleRoomJoin(payload)
	case protocol.TypeRoomMeta:
		roomMgr.HandleRoomMeta(payload, remotePeerID)
	case protocol.TypeRoomLeave:
		roomMgr.HandleRoomLeave(payload)
	case protocol.TypeRoomClosed:
		roomMgr.HandleRoomClosed(payload)
	case protocol.TypeRoomTransfer:
		roomMgr.HandleRoomTransfer(payload)
	case protocol.TypeRoomSyncReq:
		roomMgr.HandleRoomSyncReq(payload, remotePeerID)
	case protocol.TypeRoomSyncRes:
		roomMgr.HandleRoomSyncRes(payload, remotePeerID)
	case protocol.TypeFileOffer:
		tferMgr.HandleFileOffer(connID, payload, remotePeerID)
	case protocol.TypeFileAccept:
		tferMgr.HandleFileAccept(connID, payload)
	case protocol.TypeChunk:
		tferMgr.HandleChunk(connID, payload, remotePeerID)
	case protocol.TypeFileDone:
		tferMgr.HandleFileDone(connID, payload)
	case protocol.TypeFileAnnounce:
		tferMgr.HandleFileAnnounce(payload, remotePeerID)
	case protocol.TypeFileWhoHas:
		tferMgr.HandleFileWhoHas(payload, remotePeerID)
	case protocol.TypeFileHave:
	case protocol.TypeFileHave:
		tferMgr.HandleFileHave(payload, remotePeerID)
	case protocol.TypeFileRequest:
		tferMgr.HandleFileRequest(connID, payload, remotePeerID)
	case protocol.TypeFileReject:
		tferMgr.HandleFileReject(connID, payload)
	case protocol.TypeAIQueryReq:
		if d.aiSvc != nil && d.transport != nil {
			go func() {
				resBytes, err := d.aiSvc.HandleIncomingQuery(payload)
				if err == nil {
					_ = d.transport.SendToPeer(remotePeerID, protocol.TypeAIQueryRes, resBytes)
				}
			}()
		}
	case protocol.TypeAIQueryRes:
		if d.aiSvc != nil {
			d.aiSvc.HandleIncomingRes(payload)
		}
	case protocol.TypeAISessionSync:
		if d.aiSvc != nil {
			_, _, _ = d.aiSvc.HandleIncomingSync(payload)
		}
	}

}
