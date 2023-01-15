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

// avoiding typescript warnings related to emscripten
declare var HEAP8: Int8Array;
declare var HEAPU8: Uint8Array;

declare function _malloc(size: number): number;
declare function _free(ptr: number): void;

// avoiding ESLint warnings related to emscripten
/*global addFunction, removeFunction */
/*global cwrap, ccall, getValue */
/*global UTF8ToString, stringToUTF8, lengthBytesUTF8 */
/* eslint new-cap: ["error", { "capIsNewExceptions": ["UTF8ToString"] }] */
/* eslint no-console: ["error", { allow: ["assert", "error", "log"] }] */
/* eslint no-use-before-define: ["error", { "classes": false }] */

// helper functions
function allocPtr(siz: number): number {
  return _malloc(siz);
}

function allocPtrString(str: string): [number, number] {
  console.assert(typeof (str) === "string");
  let siz = lengthBytesUTF8(str);
  let ptr = _malloc(siz + 1);
  stringToUTF8(str, ptr, siz + 1);
  return [ptr, siz];
}

function allocPtrArrayBuffer(buffer: ArrayBuffer): [number, number] {
  let siz = buffer.byteLength;
  let ptr = allocPtr(siz === 0 ? 1 : siz);
  if (siz !== 0) {
    HEAP8.set(new Int8Array(buffer), ptr);
  }
  return [ptr, siz];
}

function freePtr(ptr: number): void {
  _free(ptr);
}

function convertError(ptr: number): ErrorEntry | undefined {
  if (!ptr || ptr === 0) {
    return;
  }

  let fatal = ccall("js_error_get_fatal", "boolean", ["number"], [ptr]);
  let code = ccall("js_error_get_code", "number", ["number"], [ptr]);
  let message = UTF8ToString(
    ccall("js_error_get_message", "number", ["number"], [ptr]),
    ccall("js_error_get_message_length", "number", ["number"], [ptr])
  );
  let line = ccall("js_error_get_line", "number", ["number"], [ptr]);
  let file = UTF8ToString(
    ccall("js_error_get_file", "number", ["number"], [ptr]),
    ccall("js_error_get_file_length", "number", ["number"], [ptr])
  );

  return new ErrorEntry(fatal, code, message, line, file);
}

// those functions are used to workaround for avoiding an error when call `connect` before finish loading the wasm file.
let funcsAfterLoad: Array<() => void> | null = new Array<() => void>();

function addFuncAfterLoad(func: () => void): void {
  if (funcsAfterLoad === null) {
    func();
  } else {
    funcsAfterLoad.push(func);
  }
}

function execFuncsAfterLoad(): void {
  if (funcsAfterLoad === null) { return; }

  let funcs: Array<() => void> = funcsAfterLoad;
  funcsAfterLoad = null;

  let func: (() => void) | undefined;
  while (typeof (func = funcs.shift()) !== "undefined") {
    func();
  }
}

const ID_MAX: number = Math.floor(Math.pow(2, 30));

// id map helper
function idMapPush<T>(map: Map<number, T>, v: T) {
  let id: number = Math.floor(Math.random() * ID_MAX);
  while (map.has(id)) {
    id = Math.floor(Math.random() * ID_MAX);
  }
  map.set(id, v);
  return id;
}

function idMapPop<T>(map: Map<number, T>, id: number): T | undefined {
  let v = map.get(id);
  console.assert(typeof (v) !== "undefined", "Wrong id :" + id);
  map.delete(id);
  return v;
}

function idMapGet<T>(map: Map<number, T>, id: number): T | undefined {
  let v = map.get(id);
  console.assert(typeof (v) !== "undefined", "Wrong id :" + id);
  return v;
}

// ATTENTION: Use same value with another languages.
class LogLevel {
  static readonly ERROR: string = "error";
  static readonly WARN: string = "warn";
  static readonly INFO: string = "info";
  static readonly DEBUG: string = "debug";
}

class LogEntry {
  file: string;
  level: string;
  line: number;
  message: string;
  param: object;
  time: string;

  constructor(f: string, lv: string, l: number, m: string, p: object, t: string) {
    this.file = f;
    this.level = lv;
    this.line = l;
    this.message = m;
    this.param = p;
    this.time = t;
  }
}

// ATTENTION: Use same value with another languages.
class ErrorCode {
  static readonly UNDEFINED: number = 0;
  static readonly SYSTEM_INCORRECT_DATA_FORMAT: number = 1;
  static readonly SYSTEM_CONFLICT_WITH_SETTING: number = 2;

  static readonly CONNECTION_FAILED: number = 3;
  static readonly CONNECTION_OFFLINE: number = 4;

  static readonly PACKET_NO_ONE_RECV: number = 5;
  static readonly PACKET_TIMEOUT: number = 6;

  static readonly MESSAGING_HANDLER_NOT_FOUND: number = 7;

  static readonly KVS_NOT_FOUND: number = 8;
  static readonly KVS_PROHIBIT_OVERWRITE: number = 9;
  static readonly KVS_COLLISION: number = 10;

  static readonly SPREAD_NO_ONE_RECEIVE: number = 11;
}

class ErrorEntry {
  fatal: boolean;
  code: number;
  message: string;
  line: number;
  file: string;

  constructor(f: boolean, c: number, m: string, l: number, fi: string) {
    this.fatal = f;
    this.code = c;
    this.message = m;
    this.line = l;
    this.file = fi;
  }
}

type ValueSource = null | boolean | number | string | ArrayBuffer | Value;
/**
 * Value is wrap for Value class.
 */
class Value {
  // ATTENTION: Use same value with another languages.
  static readonly VALUE_TYPE_NULL: number = 0;
  static readonly VALUE_TYPE_BOOL: number = 1;
  static readonly VALUE_TYPE_INT: number = 2;
  static readonly VALUE_TYPE_DOUBLE: number = 3;
  static readonly VALUE_TYPE_STRING: number = 4;
  static readonly VALUE_TYPE_BINARY: number = 5;

  _type: number;
  _value: null | boolean | number | string | ArrayBuffer;

  static newNull(): Value {
    return new Value(Value.VALUE_TYPE_NULL, null);
  }

  static newBool(value: boolean): Value {
    console.assert(typeof (value) === "boolean");
    return new Value(Value.VALUE_TYPE_BOOL, value);
  }

  static newInt(value: number): Value {
    console.assert(typeof (value) === "number");
    console.assert(Number.isInteger(value));
    return new Value(Value.VALUE_TYPE_INT, value);
  }

  static newDouble(value: number): Value {
    console.assert(typeof (value) === "number");
    return new Value(Value.VALUE_TYPE_DOUBLE, value);
  }

  static newString(value: string): Value {
    console.assert(typeof (value) === "string");
    return new Value(Value.VALUE_TYPE_STRING, value);
  }

  static newBinary(value: ArrayBuffer): Value {
    console.assert(value instanceof ArrayBuffer);
    return new Value(Value.VALUE_TYPE_BINARY, value);

  }

  static fromJsValue(value: ValueSource): Value | undefined {
    if (value instanceof Value) {
      return value;
    }

    if (value instanceof ArrayBuffer) {
      return Value.newBinary(value);
    }

    switch (typeof (value)) {
      case "object":
        console.assert(value === null, value);
        return Value.newNull();

      case "boolean":
        return Value.newBool(value);

      case "string":
        return Value.newString(value);

      case "number":
        console.assert(false, "Number is not explicit value type. Please use Value::asInt or Value::asDouble.");
        break;

      default:
        console.assert(false, "Unsupported value type");
        break;
    }
  }

  static fromCValue(valueC: number) {
    let type = ccall("js_value_get_type", "number", ["number"], [valueC]);
    let value: null | boolean | number | string | ArrayBuffer = null;
    switch (type) {
      case Value.VALUE_TYPE_NULL:
        value = null;
        break;

      case Value.VALUE_TYPE_BOOL:
        value = ccall("js_value_get_bool", "boolean", ["number"], [valueC]);
        break;

      case Value.VALUE_TYPE_INT:
        value = ccall("js_value_get_int", "number", ["number"], [valueC]);
        break;

      case Value.VALUE_TYPE_DOUBLE:
        value = ccall("js_value_get_double", "number", ["number"], [valueC]);
        break;

      case Value.VALUE_TYPE_STRING: {
        let length = ccall("js_value_get_string_length", "number", ["number"], [valueC]);
        let stringPtr = ccall("js_value_get_string", "number", ["number"], [valueC]);
        value = UTF8ToString(stringPtr, length);
      } break;

      case Value.VALUE_TYPE_BINARY: {
        let length = ccall("js_value_get_binary_length", "number", ["number"], [valueC]);
        let binaryPtr = ccall("js_value_get_binary", "number", ["number"], [valueC]);
        value = new ArrayBuffer(length);
        let bin = new Int8Array(value);
        bin.set(HEAP8.subarray(binaryPtr, binaryPtr + length));
      } break;
    }

    return new Value(type, value);
  }

  static free(valueC: number) {
    ccall("js_value_free", null, ["number"], [valueC]);
  }

  constructor(type: number, value: null | boolean | number | string | ArrayBuffer) {
    this._type = type;
    this._value = value;
  }

  getType() {
    return this._type;
  }

  getJsValue() {
    return this._value;
  }

  write(valueC?: number): number {
    if (typeof (valueC) === "undefined") {
      valueC = ccall("js_value_create", "number", [], []);
    }

    switch (this._type) {
      case Value.VALUE_TYPE_NULL:
        break;

      case Value.VALUE_TYPE_BOOL:
        if (typeof (this._value) !== "boolean") {
          console.error("logic error");
          break;
        }
        ccall("js_value_set_bool", null, ["number", "boolean"], [valueC, this._value]);
        break;

      case Value.VALUE_TYPE_INT:
        if (typeof (this._value) !== "number") {
          console.error("logic error");
          break;
        }
        ccall("js_value_set_int", null, ["number", "number"], [valueC, this._value]);
        break;

      case Value.VALUE_TYPE_DOUBLE:
        if (typeof (this._value) !== "number") {
          console.error("logic error");
          break;
        }
        ccall("js_value_set_double", null, ["number", "number"], [valueC, this._value]);
        break;

      case Value.VALUE_TYPE_STRING:
        if (typeof (this._value) !== "string") {
          console.error("logic error");
          break;
        }
        let [stringPtr, stringSiz] = allocPtrString(this._value);
        ccall("js_value_set_string", null, ["number", "number", "number"], [valueC, stringPtr, stringSiz]);
        freePtr(stringPtr);
        break;

      case Value.VALUE_TYPE_BINARY:
        if (!(this._value instanceof ArrayBuffer)) {
          console.error("logic error");
          break;
        }
        let [binaryPtr, binarySiz] = allocPtrArrayBuffer(this._value);
        ccall("js_value_set_binary", null, ["number", "number", "number"], [valueC, binaryPtr, binarySiz]);
        freePtr(binaryPtr);
        break;
    }

    return valueC;
  }
}

class ColonioConfig {
  loggerFuncRaw: (c: Colonio, json: string) => void;
  loggerFunc: (c: Colonio, log: LogEntry) => void;

  constructor() {
    this.loggerFuncRaw = (c: Colonio, json: string): void => {
      let log = JSON.parse(json) as LogEntry;
      this.loggerFunc(c, log);
    };

    this.loggerFunc = (_: Colonio, log: LogEntry): void => {
      if (log.level === LogLevel.ERROR) {
        console.error(`${log.time} ${log.level} ${log.line}@${log.file} ${log.message} ${JSON.stringify(log.param)}`);
      } else {
        console.log(`${log.time} ${log.level} ${log.line}@${log.file} ${log.message} ${JSON.stringify(log.param)}`);
      }
    };
  }
}

enum FuncID {
  LOGGER,
  MESSAGING_POST_ON_SUCCESS,
  MESSAGING_POST_ON_FAILURE,
  MESSAGING_HANDLER,
  KVS_GET_ON_SUCCESS,
  KVS_GET_ON_FAILURE,
  KVS_SET_ON_SUCCESS,
  KVS_SET_ON_FAILURE,
  KVS_LOCAL_DATA_HANDLER,
  SPREAD_POST_ON_SUCCESS,
  SPREAD_POST_ON_FAILURE,
  SPREAD_HANDLER,
}

const MESSAGING_ACCEPT_NEARBY: number = 0x01;
const MESSAGING_IGNORE_RESPONSE: number = 0x02;
const SPREAD_SOMEONE_MUST_RECEIVE: number = 0x01;

let loggerMap: Map<number, (json: string) => void> = new Map();
let colonioMap: Map<number, Colonio> = new Map();

addFuncAfterLoad(() => {
  let cbSuccess = addFunction((c: number, id: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let obj = idMapPop(colonio._idVsPromise, id);
    if (typeof (obj) !== "undefined") {
      obj.onSuccess(null);
    }
  }, "vii");

  let cbFailure = addFunction((c: number, id: number, errorPtr: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let obj = idMapPop(colonio._idVsPromise, id);
    if (typeof (obj) !== "undefined" && typeof (obj.onFailure) !== "undefined") {
      obj.onFailure(convertError(errorPtr));
    }
  }, "viii");

  let cbValueSuccess = addFunction((c: number, id: number, valueC: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let obj = idMapPop(colonio._idVsPromiseValue, id);
    if (typeof (obj) !== "undefined") {
      obj.onSuccess(Value.fromCValue(valueC));
    }
  }, "viii");

  let cbValueFailure = addFunction((c: number, id: number, errorPtr: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let obj = idMapPop(colonio._idVsPromiseValue, id);
    if (typeof (obj) !== "undefined" && typeof (obj.onFailure) !== "undefined") {
      obj.onFailure(convertError(errorPtr));
    }
  }, "viii");

  // logger
  let logger = addFunction((_colonio: number, ptr: number, siz: number): void => {
    let json = UTF8ToString(ptr, siz);
    let loggerEntry = loggerMap.get(_colonio);
    if (typeof (loggerEntry) === "undefined") {
      console.log(json);
    } else {
      loggerEntry(json);
    }
  }, "viii");
  ccall("js_bind_function", null, ["number", "number"], [FuncID.LOGGER, logger]);

  // messaging
  ccall("js_bind_function", null, ["number", "number"], [FuncID.MESSAGING_POST_ON_SUCCESS, cbValueSuccess]);
  ccall("js_bind_function", null, ["number", "number"], [FuncID.MESSAGING_POST_ON_FAILURE, cbValueFailure]);

  let messagingHandler = addFunction((c: number, id: number, s: number, v: number, o: number, w: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let handler = idMapGet(colonio._idVsMessagingHandler, id);
    if (typeof (handler) === "undefined") {
      console.log("?");
      return;
    }

    let nid = UTF8ToString(s);
    let value = Value.fromCValue(v);
    let req = new MessagingRequest(nid, value, o);
    if (w === 0) {
      handler(req);
    } else {
      let res = new MessagingResponseWriter(colonio, w);
      handler(req, res);
    }
  }, "viiiiii");
  ccall("js_bind_function", null, ["number", "number"], [FuncID.MESSAGING_HANDLER, messagingHandler]);

  // kvs
  ccall("js_bind_function", null, ["number", "number"], [FuncID.KVS_GET_ON_SUCCESS, cbValueSuccess]);
  ccall("js_bind_function", null, ["number", "number"], [FuncID.KVS_GET_ON_FAILURE, cbValueFailure]);
  ccall("js_bind_function", null, ["number", "number"], [FuncID.KVS_SET_ON_SUCCESS, cbSuccess]);
  ccall("js_bind_function", null, ["number", "number"], [FuncID.KVS_SET_ON_FAILURE, cbFailure]);

  let kvsLocalDataHandler = addFunction((c: number, id: number, cur: number, keysPtr: number, keysSiz): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let handler = idMapGet(colonio._idVsKvsLocalDataHandler, id);
    if (typeof (handler) === "undefined") {
      console.log("?");
      return;
    }

    let keys = new Array<string>();
    for (let idx = 0; idx < keysSiz; idx++) {
      let ptr = getValue(keysPtr + 4 * idx, "i8*"); // 4 is pointer size
      let key = UTF8ToString(ptr);
      keys.push(key);
    }

    let kld = new KvsLocalData(colonio, cur, keys);
    colonio._refKvsLocalData.set(cur, new WeakRef(kld));
    handler(kld);
  }, "viiiii");
  ccall("js_bind_function", null, ["number", "number"], [FuncID.KVS_LOCAL_DATA_HANDLER, kvsLocalDataHandler]);

  // spread
  ccall("js_bind_function", null, ["number", "number"], [FuncID.SPREAD_POST_ON_SUCCESS, cbSuccess]);
  ccall("js_bind_function", null, ["number", "number"], [FuncID.SPREAD_POST_ON_FAILURE, cbFailure]);

  let spreadHandler = addFunction((c: number, id: number, s: number, v: number, o: number): void => {
    let colonio = colonioMap.get(c);
    if (typeof (colonio) === "undefined") {
      console.log("?");
      return;
    }
    let handler = idMapGet(colonio._idVsSpreadHandler, id);
    if (typeof (handler) === "undefined") {
      console.log("?");
      return;
    }

    let nid = UTF8ToString(s);
    let value = Value.fromCValue(v);
    let req = new SpreadRequest(nid, value, o);
    handler(req);
  }, "viiiii");
  ccall("js_bind_function", null, ["number", "number"], [FuncID.SPREAD_HANDLER, spreadHandler]);
});

class MessagingRequest {
  sourceNid: string;
  message: Value;
  options: number;

  constructor(s: string, m: Value, o: number) {
    this.sourceNid = s;
    this.message = m;
    this.options = o;
  }
}

class MessagingResponseWriter {
  _colonio: Colonio;
  _writer: number;

  constructor(c: Colonio, w: number) {
    this._colonio = c;
    this._writer = w;
  }

  write(v: ValueSource) {
    let valueC = Value.fromJsValue(v)?.write();
    if (typeof (valueC) === "undefined") {
      console.log("unsupported type");
      return;
    }

    ccall("js_messaging_response_writer", null, ["number", "number", "number"],
      [this._colonio._colonio, this._writer, valueC]);

    Value.free(valueC);
  }
}

class KvsLocalData {
  _parent: Colonio;
  _cur: number;
  _keys: Array<string>;

  constructor(p: Colonio, c: number, k: Array<string>) {
    this._parent = p;
    this._cur = c;
    this._keys = k;
  }

  getKeys(): Array<string> {
    console.assert(this._cur !== 0, "this instance is freed");
    return this._keys;
  }

  getValue(key: string): Value | undefined {
    console.assert(this._cur !== 0, "this instance is freed");
    let idx = this._keys.indexOf(key);
    if (idx === -1) {
      return;
    }

    let valueC = ccall("js_kvs_local_data_get_value", "number",
      ["number", "number", "number"], [this._parent._colonio, this._cur, idx]);
    let value = Value.fromCValue(valueC);
    return value;
  }

  free(): void {
    if (this._cur === 0) {
      return;
    }
    this._parent._refKvsLocalData.delete(this._cur);
    ccall("js_kvs_local_data_free", null,
      ["number", "number"], [this._parent._colonio, this._cur]);
    this._cur = 0;
  }
}

class SpreadRequest {
  sourceNid: string;
  message: Value;
  options: number;

  constructor(s: string, m: Value, o: number) {
    this.sourceNid = s;
    this.message = m;
    this.options = o;
  }
}

class Colonio {
  _colonio: number;
  _idVsPromiseValue: Map<number, {
    onSuccess: (value: Value) => void;
    onFailure: (error?: ErrorEntry) => void | undefined;
  }> = new Map();
  _idVsPromise: Map<number, {
    onSuccess: (_: null) => void;
    onFailure: (error?: ErrorEntry) => void | undefined;
  }> = new Map();
  _idVsMessagingHandler: Map<number, (request: MessagingRequest, writer?: MessagingResponseWriter) => void> = new Map();
  _idVsKvsLocalDataHandler: Map<number, (kld: KvsLocalData) => void> = new Map();
  _refKvsLocalData: Map<number, WeakRef<KvsLocalData>> = new Map();
  _idVsSpreadHandler: Map<number, (request: SpreadRequest) => void> = new Map();

  constructor(config: ColonioConfig) {
    // init
    this._colonio = ccall("js_init", "number", [], []);
    colonioMap.set(this._colonio, this);

    // logger
    loggerMap.set(this._colonio, (json: string) => {
      config.loggerFuncRaw(this, json);
    });
  }

  connect(url: string, token: string): Promise<void> {
    // check webrtcImpl and use default module if it isn't set.
    if (typeof (webrtcImpl) === "undefined") {
      setWebRTCImpl(new DefaultWebrtcImplement());
    }

    const promise = new Promise<void>((resolve, reject): void => {
      addFuncAfterLoad((): void => {
        let funcs: { onSuccess: number; onFailure: number; } = { onSuccess: 0, onFailure: 0 };

        funcs.onSuccess = addFunction((_1: number, _2: number): void => {
          resolve();
          removeFunction(funcs.onSuccess);
          removeFunction(funcs.onFailure);
        }, "vii");

        funcs.onFailure = addFunction((_1: number, _2: number, errorPtr: number): void => {
          reject(convertError(errorPtr));
          removeFunction(funcs.onSuccess);
          removeFunction(funcs.onFailure);
        }, "viii");

        let [urlPtr, urlSiz] = allocPtrString(url);
        let [tokenPtr, tokenSiz] = allocPtrString(token);

        ccall("js_connect", null,
          ["number", "number", "number", "number", "number", "number", "number"],
          [this._colonio, urlPtr, urlSiz, tokenPtr, tokenSiz, funcs.onSuccess, funcs.onFailure]);

        freePtr(urlPtr);
        freePtr(tokenPtr);
      });
    });

    return promise;
  }

  disconnect(): Promise<void> {
    const promise = new Promise<void>((resolve, reject): void => {
      let funcs: { onSuccess: number; onFailure: number; } = { onSuccess: 0, onFailure: 0 };

      funcs.onSuccess = addFunction((_1: number, _2: number): void => {
        removeFunction(funcs.onSuccess);
        removeFunction(funcs.onFailure);

        setTimeout((): void => {
          ccall("js_quit", null, ["number"], [this._colonio]);
          colonioMap.delete(this._colonio);
          this._colonio = 0;
          resolve();
        }, 0);
      }, "vii");

      funcs.onFailure = addFunction((_1: number, _2: number, errorPtr: number): void => {
        reject(convertError(errorPtr));
        removeFunction(funcs.onSuccess);
        removeFunction(funcs.onFailure);
      }, "viii");

      ccall("js_disconnect", null, ["number", "number", "number"], [this._colonio, funcs.onSuccess, funcs.onFailure]);
    });

    return promise;
  }

  isConnected(): boolean {
    return ccall("js_is_connected", "boolean", ["number"], [this._colonio]);
  }

  getLocalNid(): string {
    let nidPtr = allocPtr(32 + 1);
    ccall("js_get_local_nid", null, ["number", "number"], [this._colonio, nidPtr]);
    let nid = UTF8ToString(nidPtr);
    freePtr(nidPtr);
    return nid;
  }

  setPosition(x: number, y: number): ErrorEntry | { x: number, y: number } {
    let e = convertError(ccall("js_set_position", "number",
      ["number", "number", "number"],
      [this._colonio, x, y]));
    if (typeof (e) !== "undefined") {
      return e;
    }
    return { x, y }; // TODO: must return adjusted value
  }

  // messaging
  messagingPost(dst: string, name: string, val: ValueSource, opt: number): Promise<Value> {
    let promise = new Promise<Value>((resolve, reject) => {
      let [dstPtr, _] = allocPtrString(dst);
      let [namePtr, nameSiz] = allocPtrString(name);
      const value = Value.fromJsValue(val);
      if (typeof (value) === "undefined") {
        reject();
        return;
      }
      const valueC = value.write();

      let id = idMapPush(this._idVsPromiseValue, {
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_messaging_post", null,
        ["number", "number", "number", "number", "number", "number", "number"],
        [this._colonio, dstPtr, namePtr, nameSiz, valueC, opt, id]);

      freePtr(namePtr);
      freePtr(dstPtr);
      Value.free(valueC);
    });

    return promise;
  }

  messagingSetHandler(name: string, handler: (request: MessagingRequest, writer?: MessagingResponseWriter) => void): void {
    let [namePtr, nameSiz] = allocPtrString(name);
    let id = idMapPush(this._idVsMessagingHandler, handler);
    ccall("js_messaging_set_handler", null, ["number", "number", "number", "number"],
      [this._colonio, namePtr, nameSiz, id]);

    // TODO Automatically released when no longer referenced
    freePtr(namePtr);
  }

  messagingUnsetHandler(name: string): void {
    let [namePtr, nameSiz] = allocPtrString(name);
    ccall("js_messaging_unset_handler", null, ["number", "number", "number"],
      [this._colonio, namePtr, nameSiz]);
    freePtr(namePtr);
  }

  // kvs
  kvsGetLocalData(): Promise<KvsLocalData> {
    let promise = new Promise<KvsLocalData>((resolve) => {
      let id = idMapPush(this._idVsKvsLocalDataHandler, (kld: KvsLocalData): void => {
        resolve(kld);
      });

      ccall("js_kvs_get_local_data", null, ["number", "number"], [this._colonio, id]);
    });

    return promise;
  }

  kvsGet(key: string): Promise<Value> {
    let promise = new Promise<Value>((resolve, reject) => {
      let [keyPtr, keySiz] = allocPtrString(key);
      let id = idMapPush(this._idVsPromiseValue, {
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_kvs_get", null,
        ["number", "number", "number", "number"],
        [this._colonio, keyPtr, keySiz, id]);
    });

    return promise;
  }

  kvsSet(key: string, val: ValueSource, opt: number): Promise<null> {
    let promise = new Promise<null>((resolve, reject) => {
      let [keyPtr, keySiz] = allocPtrString(key);
      const value = Value.fromJsValue(val);
      if (typeof (value) === "undefined") {
        reject();
        return;
      }
      const valuePtr = value.write();

      let id = idMapPush(this._idVsPromise, {
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_kvs_set", null,
        ["number", "number", "number", "number", "number", "number"],
        [this._colonio, keyPtr, keySiz, valuePtr, opt, id]);

      freePtr(keyPtr);
      Value.free(valuePtr);
    });

    return promise;
  }

  // spread
  spreadPost(x: number, y: number, r: number, name: string, val: ValueSource, opt: number): Promise<null> {
    let promise = new Promise<null>((resolve, reject) => {
      let [namePtr, nameSiz] = allocPtrString(name);
      const value = Value.fromJsValue(val);
      if (typeof (value) === "undefined") {
        reject();
        return;
      }
      const valuePtr = value.write();

      let id = idMapPush(this._idVsPromise, {
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_spread_post", null,
        ["number", "number", "number", "number", "number", "number", "number", "number", "number"],
        [this._colonio, x, y, r, namePtr, nameSiz, valuePtr, opt, id]);

      freePtr(namePtr);
      Value.free(valuePtr);
    });

    return promise;
  }

  spreadSetHandler(name: string, handler: (request: SpreadRequest) => void): void {
    let [namePtr, nameSiz] = allocPtrString(name);
    let id = idMapPush(this._idVsSpreadHandler, handler);
    ccall("js_spread_set_handler", null, ["number", "number", "number", "number"],
      [this._colonio, namePtr, nameSiz, id]);
    freePtr(namePtr);
  }

  spreadUnsetHandler(name: string): void {
    let [namePtr, nameSiz] = allocPtrString(name);
    ccall("js_spread_unset_handler", null, ["number", "number", "number"],
      [this._colonio, namePtr, nameSiz]);
    freePtr(namePtr);
  }

  // misc, internal
  _cleanup(): void {
    /* TODO: fix cleanup
    for (const [cur, ref] of this._refKvsLocalData) {
      if (typeof (ref.deref()) !== "undefined") {
        continue;
      }
      ccall("js_kvs_local_data_free", null,
        ["number", "number"], [this._colonio, cur]);
      this._refKvsLocalData.delete(cur);
    }
    //*/
  }
}

/* Scheduler */
let schedulerTimers: Map<number, number> = new Map();

function schedulerRelease(schedulerPtr: number): void {
  if (schedulerTimers.has(schedulerPtr)) {
    window.clearTimeout(schedulerTimers.get(schedulerPtr));
    schedulerTimers.delete(schedulerPtr);
  }
}

function schedulerRequestNextRoutine(schedulerPtr: number, msec: number): void {
  if (schedulerTimers.has(schedulerPtr)) {
    window.clearTimeout(schedulerTimers.get(schedulerPtr));
  }
  schedulerTimers.set(schedulerPtr, window.setTimeout((): void => {
    let next = ccall("scheduler_invoke", "number", ["number"], [schedulerPtr]);
    if (next >= 0) {
      schedulerRequestNextRoutine(schedulerPtr, next);
    }
  }, msec));
}

/* SeedLinkWebsocket */
let availableSeedLinks: Map<number, WebSocket> = new Map();

function seedLinkWsConnect(seedLink: number, urlPtr: number, urlSiz: number) {
  let url = UTF8ToString(urlPtr, urlSiz);
  console.log("socket connect", seedLink, url);
  let socket = new WebSocket(url);
  socket.binaryType = "arraybuffer";
  availableSeedLinks.set(seedLink, socket);

  socket.onopen = (_: Event): void => {
    console.log("socket open", seedLink);
    if (availableSeedLinks.has(seedLink)) {
      ccall("seed_link_ws_on_connect", null, ["number"], [seedLink]);
    }
  };

  socket.onerror = (error: Event): void => {
    console.log("socket error", seedLink, error);
    if (availableSeedLinks.has(seedLink)) {
      let [msgPtr, msgSiz] = allocPtrString(JSON.stringify(error));

      ccall("seed_link_ws_on_error", null,
        ["number", "number", "number"],
        [seedLink, msgPtr, msgSiz]);

      freePtr(msgPtr);
    }
  };

  socket.onmessage = (message: MessageEvent<ArrayBuffer>): void => {
    console.log("socket message", seedLink /*, dumpPacket(e.data) */);
    if (availableSeedLinks.has(seedLink)) {
      let [dataPtr, dataSiz] = allocPtrArrayBuffer(message.data);

      ccall("seed_link_ws_on_recv", null,
        ["number", "number", "number"],
        [seedLink, dataPtr, dataSiz]);

      freePtr(dataPtr);
    }
  };

  socket.onclose = (_: CloseEvent): void => {
    console.log("socket close", seedLink);
    if (availableSeedLinks.has(seedLink)) {
      ccall("seed_link_ws_on_disconnect", null, ["number"], [seedLink]);
    }
  };
}

function seedLinkWsSend(seedLink: number, dataPtr: number, dataSiz: number) {
  console.log("socket send", seedLink);
  console.assert(availableSeedLinks.has(seedLink));

  // avoid error : The provided ArrayBufferView value must not be shared.
  let data = new Uint8Array(dataSiz);
  for (let idx = 0; idx < dataSiz; idx++) {
    data[idx] = HEAPU8[dataPtr + idx];
  }
  let socket = availableSeedLinks.get(seedLink);
  if (typeof (socket) === "undefined") {
    console.log("socket to seed undefined");
    return;
  }
  socket.send(new Uint8Array(data));
}

function seedLinkWsDisconnect(seedLink: number): void {
  console.log("socket, disconnect", seedLink);
  let socket = availableSeedLinks.get(seedLink);
  if (typeof (socket) === "undefined") {
    console.log("double disconnect", seedLink);
    return;
  }
  socket.close();
}

function seedLinkWsFinalize(seedLink: number): void {
  console.log("socket finalize", seedLink);
  let socket = availableSeedLinks.get(seedLink);
  if (typeof (socket) === "undefined") {
    console.log("double finalize", seedLink);
    return;
  }

  // 2 : CLOSING
  // 3 : CLOSED
  if (socket.readyState !== 2 && socket.readyState !== 3) {
    socket.close();
  }

  availableSeedLinks.delete(seedLink);
}

/* WebrtcLink */
class WebrtcLinkCb {
  onDcoError(webrtcLink: number, message: string): void {
    let [messagePtr, messageSiz] = allocPtrString(message);
    ccall("webrtc_link_on_dco_error", null,
      ["number", "number", "number"],
      [webrtcLink, messagePtr, messageSiz]);
    freePtr(messagePtr);
  }

  onDcoMessage(webrtcLink: number, message: ArrayBuffer): void {
    let [dataPtr, dataSiz] = allocPtrArrayBuffer(message);
    ccall("webrtc_link_on_dco_message", null,
      ["number", "number", "number"],
      [webrtcLink, dataPtr, dataSiz]);
    freePtr(dataPtr);
  }

  onDcoOpen(webrtcLink: number): void {
    ccall("webrtc_link_on_dco_open", null,
      ["number"],
      [webrtcLink]);
  }

  onDcoClosing(webrtcLink: number): void {
    ccall("webrtc_link_on_dco_closing", null,
      ["number"],
      [webrtcLink]);
  }

  onDcoClose(webrtcLink: number): void {
    ccall("webrtc_link_on_dco_close", null,
      ["number"],
      [webrtcLink]);
  }

  onPcoError(webrtcLink: number, message: string): void {
    let [messagePtr, messageSiz] = allocPtrString(message);
    ccall("webrtc_link_on_pco_error", null,
      ["number", "number", "number"],
      [webrtcLink, messagePtr, messageSiz]);
    freePtr(messagePtr);
  }

  onPcoIceCandidate(webrtcLink: number, iceStr: string): void {
    let [icePtr, iceSiz] = allocPtrString(iceStr);
    ccall("webrtc_link_on_pco_ice_candidate", null,
      ["number", "number", "number"],
      [webrtcLink, icePtr, iceSiz]);
    freePtr(icePtr);
  }

  onPcoStateChange(webrtcLink: number, state: string): void {
    let [statePtr, stateSiz] = allocPtrString(state);
    ccall("webrtc_link_on_pco_state_change", null,
      ["number", "number", "number"],
      [webrtcLink, statePtr, stateSiz]);
    freePtr(statePtr);
  }

  onCsdSuccess(webrtcLink: number, sdpStr: string): void {
    let [sdpPtr, sdpSiz] = allocPtrString(sdpStr);
    ccall("webrtc_link_on_csd_success", null,
      ["number", "number", "number"],
      [webrtcLink, sdpPtr, sdpSiz]);
    freePtr(sdpPtr);
  }

  onCsdFailure(webrtcLink: number): void {
    ccall("webrtc_link_on_csd_failure", null,
      ["number"],
      [webrtcLink]);
  }
}

interface WebrtcImplement {
  setCb(cb: WebrtcLinkCb): void;
  contextInitialize(): void;
  contextAddIceServer(iceServer: string): void;
  linkInitialize(webrtcLink: number, isCreateDc: boolean): void;
  linkFinalize(webrtcLink: number): void;
  linkDisconnect(webrtcLink: number): void;
  linkGetLocalSdp(webrtcLink: number, isRemoteSdpSet: boolean): void;
  linkSend(webrtcLink: number, data: string | Blob | ArrayBuffer | ArrayBufferView): void;
  linkSetRemoteSdp(webrtcLink: number, sdpStr: string, isOffer: boolean): void;
  linkUpdateIce(webrtcLink: number, iceStr: string): void;
}

class DefaultWebrtcImplement implements WebrtcImplement {
  cb?: WebrtcLinkCb;
  webrtcContextPcConfig: RTCConfiguration;
  webrtcContextDcConfig: RTCDataChannelInit;
  availableWebrtcLinks: Map<number, {
    peer: RTCPeerConnection,
    dataChannel: RTCDataChannel | undefined,
  }>;

  constructor() {
    this.webrtcContextPcConfig = <RTCConfiguration>{};
    this.webrtcContextDcConfig = <RTCDataChannelInit>{};
    this.availableWebrtcLinks = new Map();
  }

  setCb(cb: WebrtcLinkCb): void {
    this.cb = cb;
  }

  contextInitialize(): void {
    this.webrtcContextPcConfig = <RTCConfiguration>{
      iceServers: new Array<RTCIceServer>(),
    };
    this.webrtcContextDcConfig = <RTCDataChannelInit>{
      ordered: true,
      maxPacketLifeTime: 3000,
    };
  }

  contextAddIceServer(iceServer: string): void {
    this.webrtcContextPcConfig.iceServers?.push(
      JSON.parse(iceServer) as RTCIceServer
    );
  }

  linkInitialize(webrtcLink: number, isCreateDc: boolean): void {
    let setEvent = (dataChannel: RTCDataChannel): void => {
      dataChannel.onerror = (e: Event): void => {
        let event = e as RTCErrorEvent;
        console.log("rtc data error", webrtcLink, event);
        if (this.availableWebrtcLinks.has(webrtcLink)) {
          this.cb?.onDcoError(webrtcLink, event.error.message);
        }
      };

      dataChannel.onmessage = (message: MessageEvent): void => {
        if (!this.availableWebrtcLinks.has(webrtcLink)) { return; }

        if (message.data instanceof ArrayBuffer) {
          // console.log("rtc data recv", webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(event.data)));
          this.cb?.onDcoMessage(webrtcLink, message.data);
        } else if (message.data instanceof Blob) {
          let reader = new FileReader();
          reader.onload = (): void => {
            // console.log("rtc data recv", webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(reader.result)));
            this.cb?.onDcoMessage(webrtcLink, reader.result as ArrayBuffer);
          };
          reader.readAsArrayBuffer(message.data);
        } else {
          console.error("Unsupported type of message.", message.data);
        }
      };

      dataChannel.onopen = (_: Event): void => {
        console.log("rtc data open", webrtcLink);
        if (this.availableWebrtcLinks.has(webrtcLink)) {
          this.cb?.onDcoOpen(webrtcLink);
        }
      };

      dataChannel.onclosing = (_: Event): void => {
        console.log("rtc data closing", webrtcLink);
        if (this.availableWebrtcLinks.has(webrtcLink)) {
          this.cb?.onDcoClosing(webrtcLink);
        }
      };

      dataChannel.onclose = (_: Event): void => {
        console.log("rtc data close", webrtcLink);
        if (this.availableWebrtcLinks.has(webrtcLink)) {
          this.cb?.onDcoClose(webrtcLink);
        }
      };
    };

    let peer: RTCPeerConnection | undefined;
    try {
      peer = new RTCPeerConnection(this.webrtcContextPcConfig);
    } catch (e) {
      console.error(e);
    }
    if (typeof (peer) === "undefined") { return; }

    let dataChannel: RTCDataChannel | undefined;
    if (isCreateDc) {
      dataChannel = peer.createDataChannel("data_channel",
        this.webrtcContextDcConfig);
      setEvent(dataChannel);
    }

    this.availableWebrtcLinks.set(webrtcLink, { peer, dataChannel });
    peer.onicecandidate = (event: RTCPeerConnectionIceEvent): void => {
      console.log("rtc on ice candidate", webrtcLink);
      if (this.availableWebrtcLinks.has(webrtcLink)) {
        let ice;
        if (event.candidate) {
          ice = JSON.stringify(event.candidate);
        } else {
          ice = "";
        }

        this.cb?.onPcoIceCandidate(webrtcLink, ice);
      }
    };

    peer.ondatachannel = (event: RTCDataChannelEvent): void => {
      console.log("rtc peer datachannel", webrtcLink);
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") {
        return;
      }

      if (typeof (link.dataChannel) !== "undefined") {
        this.cb?.onPcoError(webrtcLink, "duplicate data channel.");
      }

      link.dataChannel = event.channel;
      setEvent(event.channel);
    };

    peer.oniceconnectionstatechange = (_: Event): void => {
      console.log("rtc peer state", webrtcLink, peer?.iceConnectionState);
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") {
        return;
      }

      this.cb?.onPcoStateChange(webrtcLink, link.peer.iceConnectionState);
    };
  }

  linkFinalize(webrtcLink: number): void {
    console.assert(this.availableWebrtcLinks.has(webrtcLink));
    this.availableWebrtcLinks.delete(webrtcLink);
  }

  linkDisconnect(webrtcLink: number): void {
    console.assert(this.availableWebrtcLinks.has(webrtcLink));

    let link = this.availableWebrtcLinks.get(webrtcLink);
    if (typeof (link) === "undefined") {
      return;
    }

    if (typeof (link.dataChannel) !== "undefined") {
      link.dataChannel.close();
    }

    link.peer.close();
  }

  linkGetLocalSdp(webrtcLink: number, isRemoteSdpSet: boolean): void {
    console.log("rtc getLocalSdp", webrtcLink);
    console.assert(this.availableWebrtcLinks.has(webrtcLink));

    try {
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") { return; }
      let peer = link.peer;
      let description!: RTCSessionDescriptionInit;

      if (isRemoteSdpSet) {
        peer.createAnswer().then((sessionDescription): Promise<void> => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then((): void => {
          console.log("rtc createAnswer", webrtcLink);
          this.cb?.onCsdSuccess(webrtcLink, description!.sdp!);

        }).catch((e): void => {
          console.log("rtc createAnswer error", webrtcLink, e);
          this.cb?.onCsdFailure(webrtcLink);
        });

      } else {
        peer.createOffer().then((sessionDescription): Promise<void> => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then((): void => {
          console.log("rtc createOffer", webrtcLink);
          this.cb?.onCsdSuccess(webrtcLink, description!.sdp!);

        }).catch((e): void => {
          console.error(e);
          this.cb?.onCsdFailure(webrtcLink);
        });
      }

    } catch (e) {
      console.error(e);
    }
  }

  linkSend(webrtcLink: number, data: string | Blob | ArrayBuffer | ArrayBufferView): void {
    try {
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") { return; }
      link.dataChannel?.send(data);
    } catch (e) {
      console.error(e);
    }
  }

  linkSetRemoteSdp(webrtcLink: number, sdpStr: string, isOffer: boolean): void {
    try {
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") { return; }
      let peer = link.peer;
      let sdp = <RTCSessionDescriptionInit>{
        type: (isOffer ? "offer" : "answer"),
        sdp: sdpStr
      };
      peer.setRemoteDescription(new RTCSessionDescription(sdp));

    } catch (e) {
      console.error(e);
    }
  }

  linkUpdateIce(webrtcLink: number, iceStr: string): void {
    try {
      let link = this.availableWebrtcLinks.get(webrtcLink);
      if (typeof (link) === "undefined") { return; }
      let peer = link.peer;
      let ice = JSON.parse(iceStr);

      peer.addIceCandidate(new RTCIceCandidate(ice));

    } catch (e) {
      console.error(e);
    }
  }
}

/**
 * Interface for WebRTC.
 * It is used when use colonio in WebWorker. Currently, WebRTC is not useable in WebWorker,
 * so developer should by-pass WebRTC methods to main worker.
 */
let webrtcImpl: WebrtcImplement | undefined;

function setWebRTCImpl(w: WebrtcImplement): void {
  webrtcImpl = w;
  w.setCb(new WebrtcLinkCb());
}

/* WebrtcContext */
function webrtcContextInitialize(): void {
  webrtcImpl?.contextInitialize();
}

function webrtcContextAddIceServer(strPtr: number, strSiz: number): void {
  webrtcImpl?.contextAddIceServer(UTF8ToString(strPtr, strSiz));
}

function webrtcLinkInitialize(webrtcLink: number, isCreateDc: boolean): void {
  console.log("rtc initialize", webrtcLink);
  webrtcImpl?.linkInitialize(webrtcLink, isCreateDc);
}

function webrtcLinkFinalize(webrtcLink: number): void {
  console.log("rtc finalize", webrtcLink);
  webrtcImpl?.linkFinalize(webrtcLink);
}

function webrtcLinkDisconnect(webrtcLink: number): void {
  console.log("rtc disconnect", webrtcLink);
  webrtcImpl?.linkDisconnect(webrtcLink);
}

function webrtcLinkGetLocalSdp(webrtcLink: number, isRemoteSdpSet: boolean): void {
  console.log("rtc getLocalSdp", webrtcLink);
  webrtcImpl?.linkGetLocalSdp(webrtcLink, isRemoteSdpSet);
}

function webrtcLinkSend(webrtcLink: number, dataPtr: number, dataSiz: number): void {
  console.log("rtc data send", webrtcLink);
  // avoid error : The provided ArrayBufferView value must not be shared.
  let data = new Uint8Array(dataSiz);
  for (let idx = 0; idx < dataSiz; idx++) {
    data[idx] = HEAPU8[dataPtr + idx];
  }
  webrtcImpl?.linkSend(webrtcLink, data);
}

function webrtcLinkSetRemoteSdp(webrtcLink: number, sdpPtr: number, sdpSiz: number, isOffer: boolean): void {
  console.log("rtc setRemoteSdp", webrtcLink);
  let sdpStr = UTF8ToString(sdpPtr, sdpSiz);
  webrtcImpl?.linkSetRemoteSdp(webrtcLink, sdpStr, isOffer);
}

function webrtcLinkUpdateIce(webrtcLink: number, icePtr: number, iceSiz: number): void {
  console.log("rtc updateIce", webrtcLink);
  let iceStr = UTF8ToString(icePtr, iceSiz);
  webrtcImpl?.linkUpdateIce(webrtcLink, iceStr);
}

// for random.cpp
function utilsGetRandomSeed(): number {
  return Math.random();
}
