"use strict";
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
/*global Colonio, ColonioConfig, KvsLocalData, MessagingRequest, MessagingResponseWriter, SpreadRequest, Value */
/* eslint no-console: ["error", { allow: ["assert", "error", "log"] }] */
class ColonioGo {
    constructor(mod) {
        this.mod = mod;
    }
    newColonio(timeout, v, logger) {
        let config = new this.mod.ColonioConfig();
        config.seedSessionTimeoutMs = timeout;
        config.disableSeedVerification = v;
        config.loggerFuncRaw = (_, log) => {
            logger(log);
        };
        return new this.mod.Colonio(config);
    }
    newValue(type, value) {
        return new this.mod.Value(type, value);
    }
    onEvent(id, obj) {
        console.error("onEvent method must by override by golang", id, obj);
    }
    onResponse(id, obj) {
        console.error("onResponse method must by override by golang", id, obj);
    }
    convertError(err) {
        try {
            if (err instanceof this.mod.ErrorEntry) {
                return err;
            }
            if (err instanceof Error) {
                console.error(err);
                return new this.mod.ErrorEntry(true, this.mod.ErrorCode.UNDEFINED, err.message, 0, "");
            }
            console.error(err);
            return err;
        }
        catch (e) {
            console.error(e);
            return err;
        }
    }
    // helpers for core module
    connect(colonio, id, url, token) {
        colonio.connect(url, token).then(() => {
            this.onResponse(id);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    disconnect(colonio, id) {
        colonio.disconnect().then(() => {
            this.onResponse(id);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    // messaging
    messagingPost(colonio, id, dst, name, valueType, valueValue, opt) {
        colonio.messagingPost(dst, name, this.newValue(valueType, valueValue), opt).then((response) => {
            this.onResponse(id, response);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    messagingSetHandler(colonio, id, name) {
        colonio.messagingSetHandler(name, (request, writer) => {
            this.onEvent(id, { request, writer });
        });
    }
    // kvs
    kvsGetLocalData(colonio, id) {
        colonio.kvsGetLocalData().then((kld) => {
            this.onResponse(id, kld);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    kvsGet(colonio, id, key) {
        colonio.kvsGet(key).then((value) => {
            this.onResponse(id, value);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    kvsSet(colonio, id, key, val, opt) {
        colonio.kvsSet(key, val, opt).then(() => {
            this.onResponse(id);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    // spread
    spreadPost(colonio, id, x, y, r, name, message, opt) {
        colonio.spreadPost(x, y, r, name, message, opt).then(() => {
            this.onResponse(id);
        }, (err) => {
            this.onResponse(id, this.convertError(err));
        });
    }
    spreadSetHandler(colonio, id, name) {
        colonio.spreadSetHandler(name, (request) => {
            this.onEvent(id, request);
        });
    }
}
//# sourceMappingURL=colonio_go.js.map