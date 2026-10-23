package ui

import (
	"encoding/json"

	"kerykeion/pkg/db"
	"kerykeion/pkg/messaging"
)

func attachmentText(language string, message db.MessageRecord) string {
	if message.FileRef == "" {
		return message.Body
	}
	var file messaging.FileAttachment
	if json.Unmarshal([]byte(message.Body), &file) != nil || file.Filename == "" {
		return message.Body + "\nID: " + message.FileRef
	}
	return localized(language, "File: %s (%s)\nID: %s", file.Filename, formatBytes(file.Size), message.FileRef)
}

func (m *AppModel) messageBody(message db.MessageRecord) string {
	text := attachmentText(m.cfg.Language, message)
	if message.FileRef == "" {
		return text
	}
	for number, announce := range m.fileAnnounceMap {
		if announce.FileID == message.FileRef && announce.Room == message.ConversationID && announce.SenderID == message.AuthorID {
			return text + "\n" + m.tr("Unduh: F6 → /get %d", number)
		}
	}
	return text + "\n" + m.tr("Unduh: F6 → /get %s", message.FileRef)
}
