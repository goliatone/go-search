package jobs

import "github.com/goliatone/go-job/queue"

func NewOutboxStore(storage queue.Storage, opts ...queue.StorageOutboxOption) *queue.StorageOutboxAdapter {
	return queue.NewStorageOutboxAdapter(storage, opts...)
}
