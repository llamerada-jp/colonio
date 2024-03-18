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
package main

import (
	"io"
	"net/http"
)

// constants should be equal to internal/network/seed_transport_wasm_test.go
const (
	Request        = "🤖"
	responseNormal = "✋"
	responseError  = "🙅"

	SeedPort   = 8080
	NormalPath = "seed_transport_hello"
	ErrorPath  = "seed_transport_error"
)

func setSeedTransportHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/"+NormalPath, func(w http.ResponseWriter, r *http.Request) {
		bin, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		if string(bin) != Request {
			http.Error(w, "request is invalid", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		_, err = w.Write([]byte(responseNormal))
		if err != nil {
			panic(err)
		}
	})

	mux.HandleFunc("/"+ErrorPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, responseError, http.StatusInternalServerError)
	})
}
