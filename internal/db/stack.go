package db

import (
	"go.etcd.io/bbolt"

	"ragle/internal/model"
)

// PutStack writes or updates a stack record.
func PutStack(tx *bbolt.Tx, st model.Stack) error {
	b := tx.Bucket(BucketStacks)
	if b == nil {
		var err error
		b, err = tx.CreateBucketIfNotExists(BucketStacks)
		if err != nil {
			return err
		}
	}
	raw, err := encode(st)
	if err != nil {
		return err
	}
	return b.Put([]byte(st.ID), raw)
}

// GetStack reads one stack by ID.
func GetStack(tx *bbolt.Tx, id string) (model.Stack, error) {
	b := tx.Bucket(BucketStacks)
	if b == nil {
		return model.Stack{}, ErrNotFound
	}
	raw := b.Get([]byte(id))
	if raw == nil {
		return model.Stack{}, ErrNotFound
	}
	var st model.Stack
	if err := decode(raw, &st); err != nil {
		return model.Stack{}, err
	}
	return st, nil
}

// DeleteStack removes a stack record.
func DeleteStack(tx *bbolt.Tx, id string) error {
	b := tx.Bucket(BucketStacks)
	if b == nil {
		return nil
	}
	return b.Delete([]byte(id))
}

// ListStacks returns all stacks stored in the database.
func ListStacks(tx *bbolt.Tx) ([]model.Stack, error) {
	b := tx.Bucket(BucketStacks)
	if b == nil {
		return nil, nil
	}
	var out []model.Stack
	err := b.ForEach(func(_, v []byte) error {
		var st model.Stack
		if err := decode(v, &st); err != nil {
			return err
		}
		out = append(out, st)
		return nil
	})
	return out, err
}
