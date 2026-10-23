package db

import (
	"fmt"
	"testing"
)

func TestGetMessagesReturnsLatestWindowChronologically(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for i := 1; i <= 5; i++ {
		_, err := store.SaveMessage(MessageRecord{MsgID: fmt.Sprint(i), ConversationID: "room", AuthorID: "author", Timestamp: int64(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	messages, err := store.GetMessages("room", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[0].MsgID != "3" || messages[2].MsgID != "5" {
		t.Fatalf("wrong window: %+v", messages)
	}
}
