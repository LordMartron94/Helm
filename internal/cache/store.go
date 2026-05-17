package cache

import (
	"fmt"
	"persistence"
	"sync"
)

const targetCacheTableName = "targets"

type TargetCacheRecord struct {
	TargetName        string
	StateFingerprint  uint64
	OutputFingerprint uint64
}

type TargetCacheStore struct {
	store *persistence.JSONStore
	mu    sync.Mutex
}

func TargetCacheStoreOpen(cacheRoot string) (*TargetCacheStore, error) {
	store, err := persistence.JSONStoreCreate(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("open cache store: %w", err)
	}

	_, err = persistence.JSONStoreRegister[TargetCacheRecord](
		store,
		targetCacheTableName,
		func(record TargetCacheRecord) string { return record.TargetName },
		func(a, b TargetCacheRecord) bool { return a.TargetName < b.TargetName },
	)
	if err != nil {
		return nil, fmt.Errorf("register cache table: %w", err)
	}

	return &TargetCacheStore{store: store}, nil
}

func TargetCacheStoreClose(cacheStore *TargetCacheStore) error {
	if cacheStore == nil {
		return nil
	}
	return TargetCacheStoreSaveAll(cacheStore)
}

func TargetCacheStoreSaveAll(cacheStore *TargetCacheStore) error {
	if cacheStore == nil {
		return nil
	}
	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()
	return persistence.JSONStoreSaveAll(cacheStore.store)
}

func TargetCacheStoreGet(cacheStore *TargetCacheStore, targetName string) (TargetCacheRecord, bool, error) {
	if cacheStore == nil {
		return TargetCacheRecord{}, false, nil
	}

	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()

	record, err := persistence.JSONStoreGet[TargetCacheRecord](cacheStore.store, targetCacheTableName, targetName)
	if err != nil {
		return TargetCacheRecord{}, false, nil
	}
	return record, true, nil
}

func TargetCacheStorePut(cacheStore *TargetCacheStore, record TargetCacheRecord) error {
	if cacheStore == nil {
		return nil
	}

	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()

	_, err := persistence.JSONStoreGet[TargetCacheRecord](cacheStore.store, targetCacheTableName, record.TargetName)
	if err != nil {
		if appendErr := persistence.JSONStoreAppend(cacheStore.store, targetCacheTableName, record); appendErr != nil {
			return appendErr
		}
		return persistence.JSONStoreSaveAll(cacheStore.store)
	}

	if err := persistence.JSONStoreReplace(cacheStore.store, targetCacheTableName, record.TargetName, record); err != nil {
		return err
	}
	return persistence.JSONStoreSaveAll(cacheStore.store)
}
