package messenger

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// AttachmentStore handles the lifecycle and reference counting of attachments.
type AttachmentStore interface {
	InitRef(key string, count int) error
	UpdateRef(key string, delta int) (int, error)
	Delete(key string) error
}

type NatsAttachmentStore struct {
	js       nats.JetStreamContext
	objStore nats.ObjectStore
	kv       nats.KeyValue
}

const (
	ObjectStoreBucket = "ATTACHMENTS"
	KVBucket          = "ATTACHMENT_REFS"
)

func NewNatsAttachmentStore(js nats.JetStreamContext) (*NatsAttachmentStore, error) {
	// Initialize Object Store (we only need delete access, but we need the handle)
	os, err := js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      ObjectStoreBucket,
		Description: "Storage for message attachments",
		TTL:         24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("init object store: %w", err)
	}

	// Initialize KeyValue Store for Ref Counting
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      KVBucket,
		Description: "Reference counting for attachments",
		TTL:         24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("init kv store: %w", err)
	}

	return &NatsAttachmentStore{
		js:       js,
		objStore: os,
		kv:       kv,
	}, nil
}

func (s *NatsAttachmentStore) Delete(key string) error {
	if err := s.objStore.Delete(key); err != nil && err != nats.ErrObjectNotFound {
		return fmt.Errorf("delete obj: %w", err)
	}
	_ = s.kv.Delete(key)
	return nil
}

func (s *NatsAttachmentStore) InitRef(key string, count int) error {
	_, err := s.kv.Create(key, []byte(fmt.Sprintf("%d", count)))
	if err != nil {
		return fmt.Errorf("init ref: %w", err)
	}
	return nil
}

func (s *NatsAttachmentStore) UpdateRef(key string, delta int) (int, error) {
	for i := 0; i < 10; i++ { // retry 10 times for CAS
		entry, err := s.kv.Get(key)
		if err != nil {
			if err == nats.ErrKeyNotFound {
				current := 1
				newVal := current + delta
				if newVal <= 0 {
					return 0, nil
				}
				_, err = s.kv.Create(key, []byte(fmt.Sprintf("%d", newVal)))
				if err != nil {
					continue
				}
				return newVal, nil
			}
			return 0, err
		}

		var current int
		_, err = fmt.Sscanf(string(entry.Value()), "%d", &current)
		if err != nil {
			return 0, fmt.Errorf("parse ref count: %w", err)
		}

		newVal := current + delta
		if newVal < 0 {
			newVal = 0
		}

		_, err = s.kv.Update(key, []byte(fmt.Sprintf("%d", newVal)), entry.Revision())
		if err != nil {
			continue
		}

		return newVal, nil
	}
	return 0, fmt.Errorf("failed to update ref count after retries")
}
