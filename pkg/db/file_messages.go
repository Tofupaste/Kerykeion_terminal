package db

func (s *Store) GetFileMessages(fileID string) ([]MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT msg_id,conversation_id,author_id,body,ts,sig,status,COALESCE(file_ref,''),created_at FROM messages WHERE file_ref=? ORDER BY ts DESC,created_at DESC LIMIT 100`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []MessageRecord
	for rows.Next() {
		var message MessageRecord
		if err := rows.Scan(&message.MsgID, &message.ConversationID, &message.AuthorID, &message.Body, &message.Timestamp, &message.Signature, &message.Status, &message.FileRef, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}
