package objectstore

import "context"

type Store interface {
	Get(context.Context, string) ([]byte, error)
	Put(context.Context, string, []byte, string) error
	EnsureBucket(context.Context) error
}

type Noop struct{}

func (Noop) Get(context.Context, string) ([]byte, error) {
	return nil, ErrObjectStoreDisabled
}

func (Noop) Put(context.Context, string, []byte, string) error {
	return ErrObjectStoreDisabled
}

func (Noop) EnsureBucket(context.Context) error {
	return nil
}
