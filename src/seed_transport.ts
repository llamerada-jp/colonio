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
const MAX_ID = 0x100000000;

(globalThis as any).SeedTransport = class {
  onReceive(id: number, content: Uint8Array | undefined, status: number, err: string): void {
    console.error("SeedTransport::onReceive method must by override by go module");
  }

  send(id: number, url: string, content: Uint8Array): number {
    const request = new Request(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/octet-stream",
      },
      body: content.buffer,
    });

    let statusCode = 0;
    fetch(request).then((response: Response) => {
      statusCode = response.status;
      return response.arrayBuffer();

    }).then((buffer: ArrayBuffer | undefined) => {
      if (buffer === undefined) {
        this.onReceive(id, undefined, 0, "unexpected response");
        return;
      }
      this.onReceive(id, new Uint8Array(buffer), statusCode, "");

    }).catch((error) => {
      this.onReceive(id, undefined, 0, error.message);
    });

    return id;
  }
}