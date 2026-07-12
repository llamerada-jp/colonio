/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package kvs is the public client of the KVS module. A Client is obtained
// from Node.KVS(); users never construct one directly.
//
// Operations take a context and return synchronously. The retryable
// preparing state (ErrPreparing: routing not settled yet, or the key's range
// is mid-split/merge) is retried internally with backoff, so under node churn
// an operation may block for ten-plus seconds instead of failing; the caller
// bounds the total wait through the context deadline. Design: spec/kvs/api.md.
package kvs

import (
	"context"
	"errors"
	"fmt"
	"time"

	kvsTypes "github.com/llamerada-jp/colonio/types/kvs"
)

// Typed errors of KVS operations, tested with errors.Is. They alias the
// types/kvs sentinels so the internal module and this package agree on
// identity.
var (
	// ErrNotFound is returned by Get and Delete when the key does not exist.
	ErrNotFound = kvsTypes.ErrorStoreKeyNotFound

	// ErrPreparing is the retryable rejection class: the operation was refused
	// before acceptance, so nothing took effect. It is normally consumed by the
	// internal retry and reaches the caller only when the context expires while
	// the range is still preparing (wrapped together with the context error),
	// or when the retry is disabled with WithoutRetry.
	ErrPreparing = kvsTypes.ErrorSectorNotReady

	// ErrResultUnknown means the outcome of a write is genuinely unknown: the
	// proposal timed out but may still commit later (raft gives no negative
	// acknowledgment), so the write may or may not have taken effect. Blind
	// retries are unsafe without a conditional write (revision CAS, Stage B).
	ErrResultUnknown = kvsTypes.ErrorOperationResultUnknown
)

const (
	retryInitialBackoff = 100 * time.Millisecond
	retryMaxBackoff     = 2 * time.Second
)

// Backend is the surface of the internal KVS module driven by the Client.
// It is implemented by node/internal/kvs.KVS and declared here structurally
// so this package does not depend on internal packages. The channels are
// buffered and resolve exactly once, so an abandoned response is dropped
// safely.
type Backend interface {
	Get(key string) chan *kvsTypes.GetResult
	Set(key string, value []byte) chan error
	Delete(key string) chan error
}

// Client is the public handle of the KVS module. Obtain it from Node.KVS().
// Methods are safe for concurrent use.
type Client struct {
	backend Backend
}

// NewClient wires a Client to the node's internal KVS module. It is called by
// node.NewNode; there is no reason to call it from user code.
func NewClient(backend Backend) *Client {
	return &Client{backend: backend}
}

// GetResponse is the result of Client.Get.
type GetResponse struct {
	Value []byte
	// Revision (the record's revision for CAS) will be added by the
	// revision/CAS stage (spec/kvs/api.md Stage B).
}

// SetResponse is the result of Client.Set.
type SetResponse struct {
	// Revision (the newly assigned record revision) will be added by the
	// revision/CAS stage (spec/kvs/api.md Stage B).
}

// GetOption adjusts a single Get. No options exist yet; the parameter
// reserves the surface for WithoutValue (spec/kvs/api.md Stage C).
type GetOption func(*getOptions)

type getOptions struct{}

// WriteOption adjusts a single Set or Delete.
type WriteOption func(*writeOptions)

type writeOptions struct {
	withoutRetry bool
}

// WithoutRetry disables the internal ErrPreparing retry: the operation runs
// exactly one attempt and surfaces ErrPreparing as-is. Meant for callers that
// implement their own pacing or want to observe the raw failure class.
func WithoutRetry() WriteOption {
	return func(o *writeOptions) { o.withoutRetry = true }
}

// Get reads the current value of the key from its host replica. The read does
// not go through raft: it always observes the caller's own acknowledged
// writes, but writes committed and not yet applied on the host may be missing
// (spec/kvs/dataplane.md).
func (c *Client) Get(ctx context.Context, key string, opts ...GetOption) (*GetResponse, error) {
	options := &getOptions{}
	for _, opt := range opts {
		opt(options)
	}
	_ = options

	var value []byte
	err := c.withRetry(ctx, "get", key, true, func() error {
		select {
		case result := <-c.backend.Get(key):
			if result.Err != nil {
				return result.Err
			}
			value = result.Data
			return nil
		case <-ctx.Done():
			// A read has no side effect; no unknown-outcome marker needed.
			return ctx.Err()
		}
	})
	if err != nil {
		return nil, err
	}
	return &GetResponse{Value: value}, nil
}

// Set writes the value under the key. The acknowledgment means the write was
// committed and applied on the proposing replica (read-your-writes).
func (c *Client) Set(ctx context.Context, key string, value []byte, opts ...WriteOption) (*SetResponse, error) {
	options := newWriteOptions(opts)
	err := c.withRetry(ctx, "set", key, !options.withoutRetry, func() error {
		return c.awaitWrite(ctx, c.backend.Set(key, value))
	})
	if err != nil {
		return nil, err
	}
	return &SetResponse{}, nil
}

// Delete removes the key. Deleting an absent key returns ErrNotFound.
func (c *Client) Delete(ctx context.Context, key string, opts ...WriteOption) error {
	options := newWriteOptions(opts)
	return c.withRetry(ctx, "delete", key, !options.withoutRetry, func() error {
		return c.awaitWrite(ctx, c.backend.Delete(key))
	})
}

func newWriteOptions(opts []WriteOption) *writeOptions {
	options := &writeOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// awaitWrite waits for a write acknowledgment. When the context expires while
// the request is in flight, the proposal may still commit, so the outcome is
// marked unknown.
func (c *Client) awaitWrite(ctx context.Context, response chan error) error {
	select {
	case err := <-response:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", ErrResultUnknown, ctx.Err())
	}
}

// withRetry runs attempts until one resolves with anything but the retryable
// preparing class, backing off exponentially in between. ErrPreparing means
// the operation was rejected before acceptance, so re-sending is always safe
// (unlike ErrResultUnknown). With no context deadline the retry continues
// indefinitely; bounding the total wait is the caller's job.
func (c *Client) withRetry(ctx context.Context, op, key string, retry bool, attempt func() error) error {
	backoff := retryInitialBackoff
	for {
		err := attempt()
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrPreparing) || !retry {
			return fmt.Errorf("kvs %s %q: %w", op, key, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("kvs %s %q: deadline expired while the range was preparing: %w: %w",
				op, key, ErrPreparing, ctx.Err())
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, retryMaxBackoff)
	}
}
