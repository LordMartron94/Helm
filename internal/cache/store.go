package cache

import (
	"fmt"
	"persistence"
	"sync"
)

const targetCacheTableName = "targets"

type TargetCacheRecord struct {
	TargetName        string
	InstanceKey       string
	StateFingerprint  uint64
	OutputFingerprint uint64
}

func TargetCacheRecordKey(targetName, instanceKey string) string {
	if instanceKey == "" {
		return targetName
	}
	return targetName + "\x00" + instanceKey
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
		func(record TargetCacheRecord) string {
			return TargetCacheRecordKey(record.TargetName, record.InstanceKey)
		},
		func(a, b TargetCacheRecord) bool {
			return TargetCacheRecordKey(a.TargetName, a.InstanceKey) < TargetCacheRecordKey(b.TargetName, b.InstanceKey)
		},
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

func TargetCacheStoreGet(
	cacheStore *TargetCacheStore,
	targetName string,
	instanceKey string,
) (TargetCacheRecord, bool, error) {
	if cacheStore == nil {
		return TargetCacheRecord{}, false, nil
	}

	cacheStore.mu.Lock()
	defer cacheStore.mu.Unlock()

	key := TargetCacheRecordKey(targetName, instanceKey)
	record, err := persistence.JSONStoreGet[TargetCacheRecord](cacheStore.store, targetCacheTableName, key)
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

	key := TargetCacheRecordKey(record.TargetName, record.InstanceKey)
	_, err := persistence.JSONStoreGet[TargetCacheRecord](cacheStore.store, targetCacheTableName, key)
	if err != nil {
		if appendErr := persistence.JSONStoreAppend(cacheStore.store, targetCacheTableName, record); appendErr != nil {
			return appendErr
		}
		return persistence.JSONStoreSaveAll(cacheStore.store)
	}

	if err := persistence.JSONStoreReplace(cacheStore.store, targetCacheTableName, key, record); err != nil {
		return err
	}
	return persistence.JSONStoreSaveAll(cacheStore.store)
}
