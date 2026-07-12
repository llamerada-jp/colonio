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
	// acknowledgment), so the write may or may not have taken effect.
	// Conditional writes (WithRevision / WithAbsent) are retried through this
	// automatically — the CAS makes the retry at-most-once; unconditional
	// writes surface it to the caller.
	ErrResultUnknown = kvsTypes.ErrorOperationResultUnknown

	// ErrConflict means a conditional write (WithRevision / WithAbsent) found
	// a different record state at apply time and left the store untouched.
	// Not blindly retryable: re-read (Get) and decide. Note the auto-retry of
	// an unknown-outcome CAS can yield a false conflict — the first attempt
	// may have applied — so a conflict after such a write also warrants a
	// re-read (spec/kvs/lock.md「CAS 操作」).
	ErrConflict = kvsTypes.ErrorCasConflict
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
	Set(key string, value []byte, casRevision uint64, casAbsent bool) chan *kvsTypes.SetResult
	Delete(key string, casRevision uint64, casAbsent bool) chan error
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
	// Revision is the record's current revision: pass it to WithRevision for
	// a conditional overwrite (read-modify-write). Never 0 for an existing
	// record.
	Revision uint64
}

// SetResponse is the result of Client.Set.
type SetResponse struct {
	// Revision is the newly assigned revision of the written record, usable
	// as the expected revision of a follow-up conditional write without
	// re-reading.
	Revision uint64
}

// GetOption adjusts a single Get. No options exist yet; the parameter
// reserves the surface for WithoutValue (spec/kvs/api.md Stage C).
type GetOption func(*getOptions)

type getOptions struct{}

// WriteOption adjusts a single Set or Delete.
type WriteOption func(*writeOptions)

type writeOptions struct {
	withoutRetry   bool
	casRevision    uint64
	casRevisionSet bool
	casAbsent      bool
}

// WithoutRetry disables the internal ErrPreparing retry: the operation runs
// exactly one attempt and surfaces ErrPreparing as-is. Meant for callers that
// implement their own pacing or want to observe the raw failure class.
func WithoutRetry() WriteOption {
	return func(o *writeOptions) { o.withoutRetry = true }
}

// WithRevision makes the write conditional: it applies only when the record
// currently exists with exactly this revision, and fails with ErrConflict
// otherwise. revision must be a value previously returned by Get or Set
// (never 0). The condition also makes unknown-outcome retries safe
// (at-most-once), so the client auto-retries ErrResultUnknown for this write.
func WithRevision(revision uint64) WriteOption {
	return func(o *writeOptions) {
		o.casRevision = revision
		o.casRevisionSet = true
	}
}

// WithAbsent makes the write conditional on the record not existing (creation
// without overwrite races); it fails with ErrConflict when the record exists.
// Like WithRevision, it enables the unknown-outcome auto-retry.
func WithAbsent() WriteOption {
	return func(o *writeOptions) { o.casAbsent = true }
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

	var response GetResponse
	err := c.withRetry(ctx, "get", key, &writeOptions{}, func() error {
		select {
		case result := <-c.backend.Get(key):
			if result.Err != nil {
				return result.Err
			}
			response.Value = result.Data
			response.Revision = result.Revision
			return nil
		case <-ctx.Done():
			// A read has no side effect; no unknown-outcome marker needed.
			return ctx.Err()
		}
	})
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Set writes the value under the key. The acknowledgment means the write was
// committed and applied on the proposing replica (read-your-writes).
func (c *Client) Set(ctx context.Context, key string, value []byte, opts ...WriteOption) (*SetResponse, error) {
	options, err := newWriteOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("kvs set %q: %w", key, err)
	}
	var revision uint64
	err = c.withRetry(ctx, "set", key, options, func() error {
		select {
		case result := <-c.backend.Set(key, value, options.casRevision, options.casAbsent):
			if result.Err != nil {
				return result.Err
			}
			revision = result.Revision
			return nil
		case <-ctx.Done():
			// the proposal may still commit later
			return fmt.Errorf("%w: %w", ErrResultUnknown, ctx.Err())
		}
	})
	if err != nil {
		return nil, err
	}
	return &SetResponse{Revision: revision}, nil
}

// Delete removes the key. Deleting an absent key returns ErrNotFound.
func (c *Client) Delete(ctx context.Context, key string, opts ...WriteOption) error {
	options, err := newWriteOptions(opts)
	if err != nil {
		return fmt.Errorf("kvs delete %q: %w", key, err)
	}
	return c.withRetry(ctx, "delete", key, options, func() error {
		select {
		case err := <-c.backend.Delete(key, options.casRevision, options.casAbsent):
			return err
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrResultUnknown, ctx.Err())
		}
	})
}

func newWriteOptions(opts []WriteOption) (*writeOptions, error) {
	options := &writeOptions{}
	for _, opt := range opts {
		opt(options)
	}
	if options.casRevisionSet && options.casAbsent {
		return nil, errors.New("WithRevision and WithAbsent are mutually exclusive")
	}
	// 0 is the "unconditional" sentinel on the wire and is never a real
	// revision; passing it through would silently drop the condition.
	if options.casRevisionSet && options.casRevision == 0 {
		return nil, errors.New("WithRevision requires a non-zero revision (a value returned by Get or Set)")
	}
	return options, nil
}

// conditional reports whether the write carries a CAS condition. ErrPreparing
// is a pre-acceptance rejection and is always safe to re-send; ErrResultUnknown
// is re-sent only for conditional writes, where the CAS makes the retry
// at-most-once (a retry of an already-applied attempt lands as ErrConflict).
func (o *writeOptions) conditional() bool {
	return o.casRevisionSet || o.casAbsent
}

// withRetry runs attempts until one resolves with anything but the retryable
// classes, backing off exponentially in between. With no context deadline the
// retry continues indefinitely; bounding the total wait is the caller's job.
func (c *Client) withRetry(ctx context.Context, op, key string, options *writeOptions, attempt func() error) error {
	backoff := retryInitialBackoff
	for {
		err := attempt()
		if err == nil {
			return nil
		}
		retryable := errors.Is(err, ErrPreparing) ||
			(options.conditional() && errors.Is(err, ErrResultUnknown) && ctx.Err() == nil)
		if !retryable || options.withoutRetry {
			return fmt.Errorf("kvs %s %q: %w", op, key, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("kvs %s %q: deadline expired while retrying: %w: %w",
				op, key, err, ctx.Err())
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, retryMaxBackoff)
	}
}
