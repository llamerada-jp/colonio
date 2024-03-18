//go:build !js

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

package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"github.com/llamerada-jp/colonio"
)

type seedTransportNative struct {
	client *http.Client
}

func NewSeedTransportNative(opt *colonio.SeedTransporterOption) colonio.SeedTransporter {
	transport := &seedTransportNative{}

	if opt.Verification {
		transport.client = http.DefaultClient
	} else {
		transport.client = &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
	}

	return transport
}

// Send sends data to the specified URL and returns the response.
// The response is nil if the status code is not 200 or if other errors occurs.
func (t *seedTransportNative) Send(ctx context.Context, url string, data []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %s", body)
	}

	return body, nil
}

func init() {
	colonio.DefaultSeedTransporterFactory =
		func(opt *colonio.SeedTransporterOption) colonio.SeedTransporter {
			return NewSeedTransportNative(opt)
		}
}
