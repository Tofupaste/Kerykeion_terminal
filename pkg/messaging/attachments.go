package messaging

import (
	"encoding/json"
)

type FileAttachment struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

func (s *Service) SendFileMessage(peerID, room, filename string, size int64, fileID string) error {
	body, err := json.Marshal(FileAttachment{Filename: filename, Size: size})
	if err != nil {
		return err
	}
	if room != "" {
		_, err = s.SendRoomMessage(room, string(body), fileID)
	} else {
		_, err = s.sendDM(peerID, string(body), fileID)
	}
	return err
}
