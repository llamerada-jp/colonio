/*
 * Copyright 2017 Yuji Ito <llamerada.jp@gmail.com>
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

class ColonioGo {
    mod: any // colonio module

    constructor(mod: any) {
        this.mod = mod
    }

    newColonio(logger: (log: string) => void): Colonio {
        let config = new this.mod.ColonioConfig();
        config.loggerFuncRaw = (_: Colonio, log: string): void => {
            logger(log);
        }

        return new this.mod.Colonio(config);
    }

    newValue(type: number, value: null | boolean | number | string): Value {
        return new this.mod.Value(type, value);
    }

    onEvent(id: number, obj: object | undefined): void {
        console.error("onEvent method must by override by golang", id, obj);
    }

    onResponse(id: number, obj: object | undefined): void {
        console.error("onResponse method must by override by golang", id, obj);
    }

    // helpers for core module
    connect(colonio: Colonio, id: number, url: string, token: string): void {
        colonio.connect(url, token).then((): void => {
            this.onResponse(id, undefined);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    disconnect(colonio: Colonio, id: number): void {
        colonio.disconnect().then(() => {
            this.onResponse(id, undefined);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    // messaging
    messagingPost(colonio: Colonio, id: number, dst: string, name: string, valueType: number, valueValue: any, opt: number) {
        colonio.messagingPost(dst, name, this.newValue(valueType, valueValue), opt).then((response: Value) => {
            this.onResponse(id, response);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    messagingSetHandler(colonio: Colonio, id: number, name: string) {
        colonio.messagingSetHandler(name, (request: MessagingRequest, writer: MessagingResponseWriter | undefined) => {
            this.onEvent(id, {
                request: request,
                writer: writer,
            });
        });
    }

    // kvs
    kvsGetLocalData(colonio: Colonio, id: number) {
        colonio.kvsGetLocalData().then((kld: KvsLocalData): void => {
            this.onResponse(id, kld);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    kvsGet(colonio: Colonio, id: number, key: string) {
        colonio.kvsGet(key).then((value: Value): void => {
            this.onResponse(id, value);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    kvsSet(colonio: Colonio, id: number, key: string, val: ValueSource, opt: number) {
        colonio.kvsSet(key, val, opt).then(() => {
            this.onResponse(id, undefined);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    // spread
    spreadPost(colonio: Colonio, id: number, x: number, y: number, r: number, name: string, message: ValueSource, opt: number) {
        colonio.spreadPost(x, y, r, name, message, opt).then(() => {
            this.onResponse(id, undefined);
        }, (err) => {
            this.onResponse(id, err);
        });
    }

    spreadSetHandler(colonio: Colonio, id: number, name: string) {
        colonio.spreadSetHandler(name, (request: SpreadRequest): void => {
            this.onEvent(id, request)
        })
    }
}