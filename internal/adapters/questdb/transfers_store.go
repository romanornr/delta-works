package questdb

import "github.com/romanornr/delta-works/internal/storage"

// TransferStore implements storage.TransferStore using QuestDB
type TransferStore struct {
	client *Client
}

var _ storage.TransferStore = (*TransferStore)(nil)
