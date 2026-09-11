package backendgeneration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	domainbackend "analytix.local/runtime-go/internal/domain/backendgeneration"
	backendport "analytix.local/runtime-go/internal/ports/backendgeneration"
)

const maxAllocationInventoryV1 = 1_000_000

type AllocatorV1 struct {
	store  backendport.StoreV1
	random io.Reader
}

type ConsumedAllocationV1 struct {
	Record       domainbackend.AllocationRecordV1
	RecordDigest string
}

func NewAllocatorV1(store backendport.StoreV1, random io.Reader) (*AllocatorV1, error) {
	if store == nil {
		return nil, errors.New("backend generation allocator store is required")
	}
	if random == nil {
		random = rand.Reader
	}
	return &AllocatorV1{store: store, random: random}, nil
}

func (allocator *AllocatorV1) Consume(ctx context.Context) (ConsumedAllocationV1, error) {
	if allocator == nil || allocator.store == nil || allocator.random == nil || ctx == nil {
		return ConsumedAllocationV1{}, errors.New("backend generation allocator is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return ConsumedAllocationV1{}, err
	}
	nonceBytes := make([]byte, 32)
	if _, err := io.ReadFull(allocator.random, nonceBytes); err != nil {
		clearBytes(nonceBytes)
		return ConsumedAllocationV1{}, errors.New("backend generation nonce is unavailable")
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	clearBytes(nonceBytes)
	var consumed ConsumedAllocationV1
	err := allocator.store.WithExclusive(ctx, func(transaction backendport.TransactionV1) error {
		head, err := validateAllocationInventoryV1(ctx, transaction)
		if err != nil {
			return err
		}
		if head.Generation >= domainbackend.MaxSafeGenerationV1 {
			return errors.New("backend generation space is exhausted")
		}
		record, err := domainbackend.NewAllocationRecordV1(head.Generation+1, head.Digest, nonce)
		if err != nil {
			return err
		}
		body, err := domainbackend.AllocationRecordBytesV1(record)
		if err != nil {
			return err
		}
		digest, err := domainbackend.AllocationRecordDigestV1(record)
		if err != nil {
			return err
		}
		putErr := transaction.PutIfAbsent(ctx, digest, body)
		readback, readErr := transaction.Resolve(ctx, digest)
		if readErr != nil || !bytes.Equal(readback, body) {
			return errors.Join(errors.New("backend generation allocation commit is indeterminate"), putErr, readErr)
		}
		if putErr != nil && !errors.Is(putErr, backendport.ErrConflict) {
			return errors.Join(errors.New("backend generation allocation commit failed"), putErr)
		}
		consumed = ConsumedAllocationV1{Record: record, RecordDigest: digest}
		return nil
	})
	if err != nil {
		return ConsumedAllocationV1{}, err
	}
	if consumed.RecordDigest == "" {
		return ConsumedAllocationV1{}, errors.New("backend generation allocation was not committed")
	}
	return consumed, nil
}

func validateAllocationInventoryV1(
	ctx context.Context,
	transaction backendport.TransactionV1,
) (domainbackend.AllocationHeadV1, error) {
	if transaction == nil {
		return domainbackend.AllocationHeadV1{}, backendport.ErrCorrupt
	}
	materials := make([]domainbackend.AllocationMaterialV1, 0)
	err := transaction.Visit(ctx, func(stored backendport.StoredAllocationV1) error {
		if len(materials) >= maxAllocationInventoryV1 {
			return backendport.ErrCorrupt
		}
		materials = append(materials, domainbackend.AllocationMaterialV1{
			Digest: stored.Digest, Body: append([]byte(nil), stored.Body...),
		})
		return nil
	})
	if err != nil {
		return domainbackend.AllocationHeadV1{}, err
	}
	head, err := domainbackend.ValidateAllocationChainV1(materials)
	if err != nil {
		return domainbackend.AllocationHeadV1{}, errors.Join(backendport.ErrCorrupt, err)
	}
	return head, nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
