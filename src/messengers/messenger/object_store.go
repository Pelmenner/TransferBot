package messenger

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go"
)

type ObjectStore interface {
	Put(data []byte, filename string) (string, error)
	Get(key string) ([]byte, string, error)
	Delete(key string) error
}

type NatsObjectStore struct {
	js       nats.JetStreamContext
	objStore nats.ObjectStore
	kv       nats.KeyValue
}

const (
	ObjectStoreBucket = "ATTACHMENTS"
	KVBucket          = "ATTACHMENT_REFS"
)

func NewNatsObjectStore(js nats.JetStreamContext) (*NatsObjectStore, error) {
	// Initialize Object Store
	os, err := js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      ObjectStoreBucket,
		Description: "Storage for message attachments",
		TTL:         24 * time.Hour, // fallback TTL
	})
	if err != nil {
		return nil, fmt.Errorf("init object store: %w", err)
	}

	// Initialize KeyValue Store for Ref Counting (Cleaner usage only)
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      KVBucket,
		Description: "Reference counting for attachments",
		TTL:         24 * time.Hour, // fallback TTL
	})
	if err != nil {
		return nil, fmt.Errorf("init kv store: %w", err)
	}

	return &NatsObjectStore{
		js:       js,
		objStore: os,
		kv:       kv,
	}, nil
}

func (s *NatsObjectStore) Put(data []byte, filename string) (string, error) {
	// ObjectStore PutBytes takes a name. We need to generate a unique one.
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), nats.NewInbox()) // nats.NewInbox returns unique string

	_, err := s.objStore.Put(&nats.ObjectMeta{
		Name:        id,
		Description: filename, // Store filename in Description
	}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *NatsObjectStore) Get(key string) ([]byte, string, error) {
	// GetBytes doesn't return meta. We need Get helper or GetInfo + GetBytes.
	// But Get(key) returns ObjectResult which has Info() and Read().

	obj, err := s.objStore.Get(key)
	if err != nil {
		return nil, "", err
	}

	info, err := obj.Info()
	if err != nil {
		return nil, "", fmt.Errorf("get info: %w", err)
	}

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	return data, info.Description, nil
}

func (s *NatsObjectStore) Delete(key string) error {
	// Delete object
	if err := s.objStore.Delete(key); err != nil && err != nats.ErrObjectNotFound {
		return fmt.Errorf("delete obj: %w", err)
	}
	// Try delete KV too if exists
	if s.kv != nil {
		_ = s.kv.Delete(key)
	}
	return nil
}

// InitRef initializes the reference count.
// This is not in the ObjectStore interface and should only be used by the Controller.
func (s *NatsObjectStore) InitRef(key string, count int) error {
	_, err := s.kv.Create(key, []byte(fmt.Sprintf("%d", count)))
	if err != nil {
		return fmt.Errorf("init ref: %w", err)
	}
	return nil
}

// UpdateRef atomically updates the reference count.
// This is not in the ObjectStore interface and should only be used by the Cleaner.
func (s *NatsObjectStore) UpdateRef(key string, delta int) (int, error) {
	for i := 0; i < 10; i++ { // retry 10 times
		entry, err := s.kv.Get(key)
		if err != nil {
			if err == nats.ErrKeyNotFound {
				// See blob_store.go comments for logic context.
				current := 0
				newVal := current + delta
				if newVal <= 0 {
					return 0, nil
				}
				_, err = s.kv.Create(key, []byte(fmt.Sprintf("%d", newVal)))
				if err != nil {
					continue // retry
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
