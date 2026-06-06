package cache

import (
	"fmt"
	"persistence"
	"sync"
)

const entityCacheTableName = "entities"

type EntityCacheRecord struct {
	EntityKey         string
	StateFingerprint  uint64
	OutputFingerprint uint64
}

type EntityCacheStore struct {
	store *persistence.JSONStore
	mu    sync.Mutex
}

func EntityCacheStoreOpen(cacheRoot string) (*EntityCacheStore, error) {
	store, err := persistence.JSONStoreCreate(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("open entity cache store: %w", err)
	}

	_, err = persistence.JSONStoreRegister[EntityCacheRecord](
		store,
		entityCacheTableName,
		func(record EntityCacheRecord) string {
			return record.EntityKey
		},
		func(a, b EntityCacheRecord) bool {
			return a.EntityKey < b.EntityKey
		},
	)
	if err != nil {
		return nil, fmt.Errorf("register entity cache table: %w", err)
	}

	return &EntityCacheStore{store: store}, nil
}

func EntityCacheStoreClose(cacheStore *EntityCacheStore) error {
	if cacheStore == nil {
		return nil
	}
	return EntityCacheStoreSaveAll(cacheStore)
}

func EntityCacheStoreSaveAll(cacheStore *EntityCacheStore) error {
	if cacheStore == nil {
		return nil
	}
	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()
	return persistence.JSONStoreSaveAll(cacheStore.store)
}

func EntityCacheStoreGet(
	cacheStore *EntityCacheStore,
	entityKey string,
) (EntityCacheRecord, bool, error) {
	if cacheStore == nil {
		return EntityCacheRecord{}, false, nil
	}

	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()

	record, err := persistence.JSONStoreGet[EntityCacheRecord](cacheStore.store, entityCacheTableName, entityKey)
	if err != nil {
		return EntityCacheRecord{}, false, nil
	}
	return record, true, nil
}

func EntityCacheStorePut(cacheStore *EntityCacheStore, record EntityCacheRecord) error {
	if cacheStore == nil {
		return nil
	}

	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()

	_, err := persistence.JSONStoreGet[EntityCacheRecord](cacheStore.store, entityCacheTableName, record.EntityKey)
	if err != nil {
		if appendErr := persistence.JSONStoreAppend(cacheStore.store, entityCacheTableName, record); appendErr != nil {
			return appendErr
		}
		return persistence.JSONStoreSaveAll(cacheStore.store)
	}

	if err := persistence.JSONStoreReplace(cacheStore.store, entityCacheTableName, record.EntityKey, record); err != nil {
		return err
	}
	return persistence.JSONStoreSaveAll(cacheStore.store)
}
