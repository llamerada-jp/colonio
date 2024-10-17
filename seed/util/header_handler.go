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
package util

import (
	"net/http"
)

type headerHandler struct {
	header  map[string]string
	handler http.Handler
}

// NewHeaderHandler creates a new http.Handler that adds the given headers to the response.
// You can also add custom headers by passing a map of key-value pairs.
// To use WebAssembly with SharedArrayBuffer, you need to set the following headers.
// - Cross-Origin-Opener-Policy: same-origin
// - Cross-Origin-Embedder-Policy: require-corp
func NewHeaderHandler(header map[string]string, handler http.Handler) http.Handler {
	return &headerHandler{
		header:  header,
		handler: handler,
	}
}

// ServeHTTP is the implementation of http.Handler. It adds the headers to the response and calls the original handler.
func (h *headerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	for k, v := range h.header {
		header.Add(k, v)
	}

	h.handler.ServeHTTP(w, r)
}
