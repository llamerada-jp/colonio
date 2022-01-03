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
"use strict";

/*global _free, _malloc, HEAP8, HEAPU8, Module, UTF8ToString, addFunction, assert, ccall, lengthBytesUTF8, removeFunction, stringToUTF8*/
// eslint-disable-next-line no-console
const logD = console.log;
// eslint-disable-next-line no-console
const logE = console.error;

const ID_MAX = Math.floor(Math.pow(2, 30));

/* Push/Pop object
 * TODO release when disconnect
 */
let idObjectPair = new Map();

function pushObject(obj) {
  let id = Math.floor(Math.random() * ID_MAX);
  if (idObjectPair.has(id)) {
    id = Math.floor(Math.random() * ID_MAX);
  }
  idObjectPair.set(id, obj);
  return id;
}

function popObject(id) {
  assert(idObjectPair.has(id), "Wrong id :" + id);
  let obj = idObjectPair.get(id);
  idObjectPair.delete(id);
  return obj;
}

function getObject(id) {
  assert(idObjectPair.has(id), "Wrong id :" + id);
  return idObjectPair.get(id);
}

let funcsAfterLoad = new Array();

function addFuncAfterLoad(func) {
  if (funcsAfterLoad === null) {
    func();

  } else {
    funcsAfterLoad.push(func);
  }
}

function execFuncsAfterLoad() {
  let execFuncs = funcsAfterLoad;
  funcsAfterLoad = null;

  while (execFuncs.length > 0) {
    execFuncs.shift()();
  }
}

function allocPtr(siz) {
  return _malloc(siz);
}

function allocPtrString(str) {
  assert(typeof (str) === "string");
  let siz = lengthBytesUTF8(str);
  let ptr = allocPtr(siz + 1);
  stringToUTF8(str, ptr, siz + 1);
  return [ptr, siz];
}

function allocPtrArrayBuffer(buffer) {
  let siz = buffer.byteLength;
  let ptr = allocPtr(siz === 0 ? 1 : siz);
  if (siz !== 0) {
    HEAP8.set(new Int8Array(buffer), ptr);
  }
  return [ptr, siz];
}

function freePtr(ptr) {
  _free(ptr);
}

// [<ptr or id>]["<class>/<event type>"][]
let eventFuncs = new Map();

function setEventFunc(ptr, clazz, type, func) {
  assert(ptr !== null);

  // ptr = String(ptr);

  if (!eventFuncs.has(ptr)) {
    eventFuncs.set(ptr, new Map());
  }
  let ptrFuncs = eventFuncs.get(ptr);

  let key = clazz + "/" + type;
  if (!ptrFuncs.has(key)) {
    ptrFuncs.set(key, new Array());
  }
  ptrFuncs.get(key).push(func);
}

function getEventFuncs(ptr, clazz, type) {
  //ptr = String(ptr);
  let key = clazz + "/" + type;
  if (eventFuncs.has(ptr)) {
    let ptrFuncs = eventFuncs.get(ptr);
    if (ptrFuncs.has(key)) {
      return ptrFuncs.get(key);
    }
  }
  return null;
}

function convertError(ptr) {
  if (!ptr) {
    return null;
  }

  return {
    code: ccall("js_error_get_code", "number", ["number"], [ptr]),
    message: UTF8ToString(
      ccall("js_error_get_message", "number", ["number"], [ptr]),
      ccall("js_error_get_message_length", "number", ["number"], [ptr])
    )
  };
}

class Colonio {
  // ATTENTION: Use same value with another languages.
  static get LOG_LEVEL_INFO() { return "info"; }
  static get LOG_LEVEL_WARN() { return "warn"; }
  static get LOG_LEVEL_ERROR() { return "error"; }
  static get LOG_LEVEL_DEBUG() { return "debug"; }

  // ATTENTION: Use same value with another languages.
  static get ERROR_CODE_UNDEFINED() { return 0; }
  static get ERROR_CODE_SYSTEM_ERROR() { return 1; }
  static get ERROR_CODE_CONNECTION_FAILD() { return 2; }
  static get ERROR_CODE_OFFLINE() { return 3; }
  static get ERROR_CODE_INCORRECT_DATA_FORMAT() { return 4; }
  static get ERROR_CODE_CONFLICT_WITH_SETTING() { return 5; }
  static get ERROR_CODE_NOT_EXIST_KEY() { return 6; }
  static get ERROR_CODE_EXIST_KEY() { return 7; }
  static get ERROR_CODE_CHANGED_PROPOSER() { return 8; }
  static get ERROR_CODE_COLLISION_LATE() { return 9; }
  static get ERROR_CODE_NO_ONE_RECV() { return 10; }
  static get ERROR_CODE_CALLBACK_ERROR() { return 11; }

  static get NID_THIS() { return "."; }

  constructor(logger) {
    this._colonioPtr = null;
    this._instanceCache = new Map();

    addFuncAfterLoad(() => {
      // initialize
      let logWrapper = Module.addFunction((_/*colonioPtr*/, messagePtr, messageSiz) => {
        let message = UTF8ToString(messagePtr, messageSiz);
        logger(message);
      }, "viii");

      let setPositionOnSuccess = Module.addFunction((id, newX, newY) => {
        popObject(id).onSuccess({
          x: newX,
          y: newY
        });
      }, "vidd");

      let setPositionOnFailure = Module.addFunction((id, errorPtr) => {
        popObject(id).onFailure(convertError(errorPtr));
      }, "vii");

      let callByNidOnSuccess = Module.addFunction((id, valuePtr) => {
        popObject(id).onSuccess(ColonioValue.fromCValue(valuePtr));
      }, "vii");

      let callByNidOnFailure = Module.addFunction((id, errorPtr) => {
        popObject(id).onFailure(convertError(errorPtr));
      }, "vii");

      let onOnCall = Module.addFunction((id, namePtr, nameSiz, valuePtr, opt, resultPtr) => {
        let parameter = {
          name: UTF8ToString(namePtr, nameSiz),
          value: ColonioValue.fromCValue(valuePtr),
          opt: opt,
        };
        let result = getObject(id)(parameter);
        ColonioValue.fromJsValue(result).write(resultPtr);
      }, "viiiiii");

      this._colonioPtr = ccall("js_init", "number",
        ["number", "number", "number", "number", "number", "number"],
        [logWrapper, setPositionOnSuccess, setPositionOnFailure, callByNidOnSuccess, callByNidOnFailure, onOnCall]);
    });
  }

  connect(url, token) {
    const promise = new Promise((resolve, reject) => {
      addFuncAfterLoad(() => {
        var onSuccess = Module.addFunction((_) => {
          resolve();
          removeFunction(onSuccess);
          removeFunction(onFailure);
        }, "vi");

        var onFailure = Module.addFunction((_, errorPtr) => {
          reject(convertError(errorPtr));
          removeFunction(onSuccess);
          removeFunction(onFailure);
        }, "vii");

        let [urlPtr, urlSiz] = allocPtrString(url);
        let [tokenPtr, tokenSiz] = allocPtrString(token);

        ccall("js_connect", "null",
          ["number", "number", "number", "number", "number", "number", "number"],
          [this._colonioPtr, urlPtr, urlSiz, tokenPtr, tokenSiz, onSuccess, onFailure]);

        freePtr(url);
        freePtr(token);
      });
    });

    return promise;
  }

  disconnect() {
    const promise = new Promise((resolve, reject) => {
      var onSuccess = Module.addFunction((colonioPtr) => {
        removeFunction(onSuccess);
        removeFunction(onFailure);

        setTimeout(() => {
          ccall("js_quit", "null", ["number"], [this._colonioPtr]);
          this._instanceCache = null;
          resolve();
        }, 0);
      }, "vi");

      var onFailure = Module.addFunction((colonioPtr, errorPtr) => {
        reject(convertError(errorPtr));
        removeFunction(onSuccess);
        removeFunction(onFailure);
      }, "vii");

      ccall("js_disconnect", "null", ["number", "number", "number"], [this._colonioPtr, onSuccess, onFailure]);
    });

    return promise;
  }

  isConnected() {
    let res = ccall("js_is_connected", "number", ["number"], [this._colonioPtr]);
    if (res == 0) {
      return false;
    }
    return true;
  }

  accessMap(name) {
    if (!this._instanceCache.has(name)) {
      let [namePtr, nameSiz] = allocPtrString(name);
      this._instanceCache.set(name, new ColonioMap(ccall("js_access_map", "number",
        ["number", "number", "number"],
        [this._colonioPtr, namePtr, nameSiz])));
      freePtr(namePtr);
    }
    return this._instanceCache.get(name);
  }

  accessPubsub2D(name) {
    if (!this._instanceCache.has(name)) {
      let [namePtr, nameSiz] = allocPtrString(name);
      this._instanceCache.set(name, new ColonioPubsub2D(ccall("js_access_pubsub_2d", "number",
        ["number", "number", "number"],
        [this._colonioPtr, namePtr, nameSiz])));
      freePtr(namePtr);
    }
    return this._instanceCache.get(name);
  }

  getLocalNid() {
    let nidPtr = allocPtr(32 + 1);
    ccall("js_get_local_nid", "null", ["number", "number"], [this._colonioPtr, nidPtr]);
    let nid = UTF8ToString(nidPtr);
    return nid;
  }

  setPosition(x, y) {
    const promise = new Promise((resolve, reject) => {
      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_set_position", "null",
        ["number", "number", "number", "number"],
        [this._colonioPtr, x, y, id]);
    });

    return promise;
  }

  callByNid(dst, name, val, opt) {
    let promise = new Promise((resolve, reject) => {
      let [dstPtr, _] = allocPtrString(dst);
      let [namePtr, nameSiz] = allocPtrString(name);
      const value = ColonioValue.fromJsValue(val);
      const valuePtr = value.write();

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_call_by_nid", "null",
        ["number", "number", "number", "number", "number", "number", "number"],
        [this._colonioPtr, dstPtr, namePtr, nameSiz, valuePtr, opt, id]);

      freePtr(namePtr);
      freePtr(dstPtr);
      ColonioValue.release(valuePtr);
    });

    return promise;
  }

  onCall(name, cb) {
    let id = pushObject(cb); // TODO free it
    let [namePtr, nameSiz] = allocPtrString(name);
    ccall("js_on_call", "null", ["number", "number", "number", "number"], [this._colonioPtr, namePtr, nameSiz, id]);
    freePtr(namePtr);
  }

  offCall(name) {
    let [namePtr, nameSiz] = allocPtrString(name);
    ccall("js_off_call", "null", ["number", "number", "number"], [this._colonioPtr, namePtr, nameSiz]);
    freePtr(namePtr);
  }
}

/* Scheduler */
let schedulerTimers = new Map();

function schedulerRelease(schedulerPtr) {
  if (schedulerTimers.has(schedulerPtr)) {
    clearTimeout(schedulerTimers.get(schedulerPtr));
    schedulerTimers.delete(schedulerPtr);
  }
}

function schedulerRequestNextRoutine(schedulerPtr, msec) {
  if (schedulerTimers.has(schedulerPtr)) {
    clearTimeout(schedulerTimers.get(schedulerPtr));
  }
  schedulerTimers.set(schedulerPtr, setTimeout(() => {
    let next = ccall("scheduler_invoke", "number", ["number"], [schedulerPtr]);
    if (next >= 0) {
      schedulerRequestNextRoutine(schedulerPtr, next);
    }
  }, msec));
}

/**
 * ColonioValue is wrap for Value class.
 */
class ColonioValue {
  // ATTENTION: Use same value with another languages.
  static get VALUE_TYPE_NULL() { return 0; }
  static get VALUE_TYPE_BOOL() { return 1; }
  static get VALUE_TYPE_INT() { return 2; }
  static get VALUE_TYPE_DOUBLE() { return 3; }
  static get VALUE_TYPE_STRING() { return 4; }

  static Null() {
    return new ColonioValue(0, null);
  }

  static Bool(value) {
    assert(typeof (value) === "boolean");
    return new ColonioValue(1, value);
  }

  static Int(value) {
    assert(typeof (value) === "number");
    assert(value.isInteger());
    return new ColonioValue(2, value);
  }

  static Double(value) {
    assert(typeof (value) === "number");
    return new ColonioValue(3, value);
  }

  static String(value) {
    assert(typeof (value) === "string");
    return new ColonioValue(4, value);
  }

  static fromJsValue(value) {
    if (value instanceof ColonioValue) {
      return value;
    }

    switch (typeof (value)) {
      case "object":
        assert(value === null);
        return ColonioValue.Null();

      case "boolean":
        return ColonioValue.Bool(value);

      case "string":
        return ColonioValue.String(value);

      case "number":
        assert(false, "Number is not explicit value type. Please use ColonioValue::asInt or ColonioValue::asDouble.");
        break;

      default:
        assert(false, "Unsupported value type");
        break;
    }
  }

  static fromCValue(valuePtr) {
    let type = ccall("js_value_get_type", "number", ["number"], [valuePtr]);
    let value;
    switch (type) {
      case 0: // Null
        value = null;
        break;

      case 1: // Boolean
        value = ccall("js_value_get_bool", "boolean", ["number"], [valuePtr]);
        break;

      case 2: // Integer
        value = ccall("js_value_get_int", "number", ["number"], [valuePtr]);
        break;

      case 3: // Double
        value = ccall("js_value_get_double", "number", ["number"], [valuePtr]);
        break;

      case 4: // String
        let length = ccall("js_value_get_string_length", "number", ["number"], [valuePtr]);
        let stringPtr = ccall("js_value_get_string", "number", ["number"], [valuePtr]);
        value = UTF8ToString(stringPtr, length);
        break;
    }

    return new ColonioValue(type, value);
  }

  static release(valuePtr) {
    ccall("js_value_release", "null", ["number"], [valuePtr]);
  }

  constructor(type, value) {
    this._type = type;
    this._value = value;
  }

  getType() {
    return this._type;
  }

  getJsValue() {
    return this._value;
  }

  write(valuePtr = null) {
    if (valuePtr === null) {
      valuePtr = ccall("js_value_init", "number", [], []);
    }

    switch (this._type) {
      case 0: // Null
        ccall("js_value_free", "null", ["number"], [valuePtr]);
        break;

      case 1: // Boolean
        ccall("js_value_set_bool", "null", ["number", "boolean"], [valuePtr, this._value]);
        break;

      case 2: // Integer
        ccall("js_value_set_int", "null", ["number", "number"], [valuePtr, this._value]);
        break;

      case 3: // Double
        ccall("js_value_set_double", "null", ["number", "number"], [valuePtr, this._value]);
        break;

      case 4: // String
        let [stringPtr, stringSiz] = allocPtrString(this._value);
        ccall("js_value_set_string", "null", ["number", "number", "number"], [valuePtr, stringPtr, stringSiz]);
        freePtr(stringPtr);
        break;
    }

    return valuePtr;
  }
}

function initializeMap() {
  let foreachLocalValueCb = Module.addFunction((id, keyPtr, valuePtr, attr) => {
    getObject(id)(ColonioValue.fromCValue(keyPtr), ColonioValue.fromCValue(valuePtr), attr);
  }, "viiii");

  let getValueOnSuccess = Module.addFunction((id, valuePtr) => {
    popObject(id).onSuccess(ColonioValue.fromCValue(valuePtr));
  }, "vii");

  let getValueOnFailure = Module.addFunction((id, errorPtr) => {
    popObject(id).onFailure(convertError(errorPtr));
  }, "vii");

  let setValueOnSuccess = Module.addFunction((id) => {
    popObject(id).onSuccess();
  }, "vi");

  let setValueOnFailure = Module.addFunction((id, errorPtr) => {
    popObject(id).onFailure(convertError(errorPtr));
  }, "vii");

  ccall("js_map_init", "null",
    ["number", "number", "number", "number", "number"],
    [foreachLocalValueCb, getValueOnSuccess, getValueOnFailure, setValueOnSuccess, setValueOnFailure]);
}

class ColonioMap {
  static get ERROR_WITHOUT_EXIST() { return 0x1; }
  static get ERROR_WITH_EXIST() { return 0x2; }

  constructor(mapPtr) {
    this._mapPtr = mapPtr;
  }

  foreachLocalValue(cb) {
    this.foreachLocalValueRaw((key, value, attr) => {
      cb(key.getJsValue(), value.getJsValue(), attr);
    });
  }

  foreachLocalValueRaw(cb) {
    let id = pushObject(cb);

    let errorPtr = ccall("js_map_foreach_local_value", "null", ["number", "number"], [this._mapPtr, id]);

    popObject(id);

    return convertError(errorPtr);
  }

  getValue(key) {
    let promise = new Promise((resolve, reject) => {
      this.getRawValue(key).then((val) => {
        resolve(val.getJsValue());
      }, () => {
        reject();
      });
    });

    return promise;
  }

  getRawValue(key) {
    let promise = new Promise((resolve, reject) => {
      const keyValue = ColonioValue.fromJsValue(key);
      const keyPtr = keyValue.write();

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_map_get_value", "null",
        ["number", "number", "number"],
        [this._mapPtr, keyPtr, id]);


      ColonioValue.release(keyPtr);
    });

    return promise;
  }

  setValue(key, val, opt = 0) {
    let promise = new Promise((resolve, reject) => {
      const keyValue = ColonioValue.fromJsValue(key);
      const keyPtr = keyValue.write();
      const valValue = ColonioValue.fromJsValue(val);
      const valPtr = valValue.write();

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_map_set_value", "null",
        ["number", "number", "number", "number", "number"],
        [this._mapPtr, keyPtr, valPtr, opt, id]);

      ColonioValue.release(keyPtr);
      ColonioValue.release(valPtr);
    });

    return promise;
  }
}

function initializePubsub2D() {
  let publishOnSuccess = Module.addFunction((id) => {
    popObject(id).onSuccess();
  }, "vi");

  let publishOnFailure = Module.addFunction((id, errorPtr) => {
    popObject(id).onFailure(convertError(errorPtr));
  }, "vii");

  let onOn = Module.addFunction((id, valuePtr) => {
    getObject(id)(ColonioValue.fromCValue(valuePtr));
  }, "vii");

  ccall("js_pubsub_2d_init", "null",
    ["number", "number", "number"],
    [publishOnSuccess, publishOnFailure, onOn]);
}

class ColonioPubsub2D {
  static get RAISE_NO_ONE_RECV() { return 0x1; }

  constructor(pubsub2DPtr) {
    this._pubsub2DPtr = pubsub2DPtr;
  }

  publish(name, x, y, r, val, opt) {
    let promise = new Promise((resolve, reject) => {
      let [namePtr, nameSiz] = allocPtrString(name);
      const value = ColonioValue.fromJsValue(val);
      const valuePtr = value.write();

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_pubsub_2d_publish", "null",
        ["number", "number", "number", "number", "number", "number", "number", "number", "number"],
        [this._pubsub2DPtr, namePtr, nameSiz, x, y, r, valuePtr, opt, id]);

      freePtr(namePtr);
      ColonioValue.release(valuePtr);
    });

    return promise;
  }

  on(name, cb) {
    this.onRaw(name, (val) => {
      cb(val.getJsValue());
    });
  }

  onRaw(name, cb) {
    let id = pushObject(cb); // TODO free it

    let [namePtr, nameSiz] = allocPtrString(name);

    ccall("js_pubsub_2d_on", "null",
      ["number", "number", "number", "number"],
      [this._pubsub2DPtr, namePtr, nameSiz, id]);

    freePtr(namePtr);
  }

  off(name) {
    let [namePtr, nameSiz] = allocPtrString(name);

    ccall("js_pubsub_2d_off", "null",
      ["number", "number", "number"],
      [this._pubsub2DPtr, namePtr, nameSiz]);

    freePtr(namePtr);
  }
}  // class ColonioPubsub2D

/* SeedLinkWebsocket */
let availableSeedLinks = new Map();

function seedLinkWsConnect(seedLink, urlPtr, urlSiz) {
  let url = UTF8ToString(urlPtr, urlSiz);
  logD("socket connect", seedLink, url);
  let socket = new WebSocket(url);
  socket.binaryType = "arraybuffer";
  availableSeedLinks.set(seedLink, socket);

  socket.onopen = () => {
    logD("socket open", seedLink);
    if (availableSeedLinks.has(seedLink)) {
      ccall("seed_link_ws_on_connect", "null", ["number"], [seedLink]);
    }
  };

  socket.onerror = (error) => {
    logD("socket error", seedLink, error);
    if (availableSeedLinks.has(seedLink)) {
      let [msgPtr, msgSiz] = allocPtrString(JSON.stringify(error));

      ccall("seed_link_ws_on_error", "null",
        ["number", "number", "number"],
        [seedLink, msgPtr, msgSiz]);

      freePtr(msgPtr);
    }
  };

  socket.onmessage = (e) => {
    logD("socket message", seedLink /*, dumpPacket(e.data) */);
    if (availableSeedLinks.has(seedLink)) {
      let [dataPtr, dataSiz] = allocPtrArrayBuffer(e.data);

      ccall("seed_link_ws_on_recv", "null",
        ["number", "number", "number"],
        [seedLink, dataPtr, dataSiz]);

      freePtr(dataPtr);
    }
  };

  socket.onclose = () => {
    logD("socket close", seedLink);
    if (availableSeedLinks.has(seedLink)) {
      ccall("seed_link_ws_on_disconnect", "null", ["number"], [seedLink]);
    }
  };
}

function seedLinkWsSend(seedLink, dataPtr, dataSiz) {
  logD("socket send", seedLink);
  assert(availableSeedLinks.has(seedLink));

  // avoid error : The provided ArrayBufferView value must not be shared.
  let data = new Uint8Array(dataSiz);
  for (let idx = 0; idx < dataSiz; idx++) {
    data[idx] = HEAPU8[dataPtr + idx];
  }
  availableSeedLinks.get(seedLink).send(new Uint8Array(data));
}

function seedLinkWsDisconnect(seedLink) {
  logD("socket, disconnect", seedLink);
  if (availableSeedLinks.has(seedLink)) {
    availableSeedLinks.get(seedLink).close();
  } else {
    logD("double disconnect", seedLink);
  }
}

function seedLinkWsFinalize(seedLink) {
  logD("socket finalize", seedLink);
  if (availableSeedLinks.has(seedLink)) {
    // 2 : CLOSING
    // 3 : CLOSED
    let l = availableSeedLinks.get(seedLink);
    if (l.readyState !== 2 && l.readyState !== 3) {
      l.close();
    }

    availableSeedLinks.delete(seedLink);
  } else {
    logD("double finalize", seedLink);
  }
}

function utilsGetRandomSeed() {
  return Math.random();
}

/* WebrtcContext */
let webrtcContextPcConfig;
let webrtcContextDcConfig;

function webrtcContextInitialize() {
  webrtcContextPcConfig = {
    iceServers: []
  };

  webrtcContextDcConfig = {
    orderd: true,
    // maxRetransmitTime: 3000,
    maxPacketLifeTime: 3000
  };
}

function webrtcContextAddIceServer(strPtr, strSiz) {
  webrtcContextPcConfig.iceServers.push(
    JSON.parse(UTF8ToString(strPtr, strSiz))
  );
}

/* WebrtcLink */
let availableWebrtcLinks = new Map();

function webrtcLinkInitialize(webrtcLink, isCreateDc) {
  logD("rtc initialize", webrtcLink);

  let setEvent = (dataChannel) => {
    dataChannel.onerror = (event) => {
      logD("rtc data error", webrtcLink, event);
      if (availableWebrtcLinks.has(webrtcLink)) {
        let [messagePtr, messageSiz] = allocPtrString(event.error.message);
        ccall("webrtc_link_on_dco_error", "null",
          ["number", "number", "number"],
          [webrtcLink, messagePtr, messageSiz]);
        freePtr(messagePtr);
      }
    };

    dataChannel.onmessage = (event) => {
      if (availableWebrtcLinks.has(webrtcLink)) {
        if (event.data instanceof ArrayBuffer) {
          // logD("rtc data recv", webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(event.data)));
          let [dataPtr, dataSiz] = allocPtrArrayBuffer(event.data);
          ccall("webrtc_link_on_dco_message", "null",
            ["number", "number", "number"],
            [webrtcLink, dataPtr, dataSiz]);
          freePtr(dataPtr);

        } else if (event.data instanceof Blob) {
          let reader = new FileReader();
          reader.onload = () => {
            // logD("rtc data recv", webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(reader.result)));
            let [dataPtr, dataSiz] = allocPtrArrayBuffer(reader.result);
            ccall("webrtc_link_on_dco_message", "null",
              ["number", "number", "number"],
              [webrtcLink, dataPtr, dataSiz]);
            freePtr(dataPtr);
          };
          reader.readAsArrayBuffer(event.data);

        } else {
          logE("Unsupported type of message.", event.data);
        }
      }
    };

    dataChannel.onopen = () => {
      logD("rtc data open", webrtcLink);
      if (availableWebrtcLinks.has(webrtcLink)) {
        ccall("webrtc_link_on_dco_open", "null",
          ["number"],
          [webrtcLink]);
      }
    };

    dataChannel.onclosing = () => {
      logD("rtc data closing", webrtcLink);
      if (availableWebrtcLinks.has(webrtcLink)) {
        ccall("webrtc_link_on_dco_closing", "null",
          ["number"],
          [webrtcLink]);
      }
    };

    dataChannel.onclose = () => {
      logD("rtc data close", webrtcLink);
      if (availableWebrtcLinks.has(webrtcLink)) {
        ccall("webrtc_link_on_dco_close", "null",
          ["number"],
          [webrtcLink]);
      }
    };
  };

  let peer = null;
  try {
    peer = new RTCPeerConnection(webrtcContextPcConfig);

  } catch (e) {
    logE(e);
    peer = null;
  }

  if (peer === null) {
    logE("RTCPeerConnection");
  }

  let dataChannel = null;
  if (isCreateDc) {
    dataChannel = peer.createDataChannel("data_channel",
      webrtcContextDcConfig);
    setEvent(dataChannel);
  }

  availableWebrtcLinks.set(webrtcLink, { peer, dataChannel });

  peer.onicecandidate = (event) => {
    logD("rtc on ice candidate", webrtcLink);
    if (availableWebrtcLinks.has(webrtcLink)) {
      let ice;
      if (event.candidate) {
        ice = JSON.stringify(event.candidate);
      } else {
        ice = "";
      }

      let [icePtr, iceSiz] = allocPtrString(ice);
      ccall("webrtc_link_on_pco_ice_candidate", "null",
        ["number", "number", "number"],
        [webrtcLink, icePtr, iceSiz]);
      freePtr(icePtr);
    }
  };

  peer.ondatachannel = (event) => {
    logD("rtc peer datachannel", webrtcLink);
    if (availableWebrtcLinks.has(webrtcLink)) {
      let link = availableWebrtcLinks.get(webrtcLink);

      if (link.dataChannel !== null) {
        let [messagePtr, messageSiz] = allocPtrString("duplicate data channel.");
        ccall("webrtc_link_on_dco_error", "null",
          ["number", "number", "number"],
          [webrtcLink, messagePtr, messageSiz]);
        freePtr(messagePtr);
      }

      link.dataChannel = event.channel;
      setEvent(event.channel);
    }
  };

  peer.oniceconnectionstatechange = (event) => {
    logD("rtc peer state", webrtcLink, peer.iceConnectionState);
    if (availableWebrtcLinks.has(webrtcLink)) {
      let link = availableWebrtcLinks.get(webrtcLink);
      let peer = link.peer;
      let [statePtr, stateSiz] = allocPtrString(peer.iceConnectionState);
      ccall("webrtc_link_on_pco_state_change", "null",
        ["number", "number", "number"],
        [webrtcLink, statePtr, stateSiz]);
      freePtr(statePtr);
    }
  };
}

function webrtcLinkFinalize(webrtcLink) {
  logD("rtc finalize", webrtcLink);
  assert(availableWebrtcLinks.has(webrtcLink));
  availableWebrtcLinks.delete(webrtcLink);
}

function webrtcLinkDisconnect(webrtcLink) {
  logD("rtc disconnect", webrtcLink);
  assert(availableWebrtcLinks.has(webrtcLink));

  if (availableWebrtcLinks.has(webrtcLink)) {
    try {
      let link = availableWebrtcLinks.get(webrtcLink);

      if (link.dataChannel !== null) {
        link.dataChannel.close();
      }

      if (link.peer !== null) {
        link.peer.close();
      }
    } catch (e) {
      logE(e);
    }
  }
}

function webrtcLinkGetLocalSdp(webrtcLink, isRemoteSdpSet) {
  logD("rtc getLocalSdp", webrtcLink);
  assert(availableWebrtcLinks.has(webrtcLink));

  try {
    let link = availableWebrtcLinks.get(webrtcLink);
    let peer = link.peer;
    let description;

    if (isRemoteSdpSet) {
      peer.createAnswer().then((sessionDescription) => {
        description = sessionDescription;
        return peer.setLocalDescription(sessionDescription);

      }).then(() => {
        logD("rtc createAnswer", webrtcLink);
        let [sdpPtr, sdpSiz] = allocPtrString(description.sdp);
        ccall("webrtc_link_on_csd_success", "null",
          ["number", "number", "number"],
          [webrtcLink, sdpPtr, sdpSiz]);
        freePtr(sdpPtr);

      }).catch((e) => {
        logD("rtc createAnswer error", webrtcLink, e);
        ccall("webrtc_link_on_csd_failure", "null",
          ["number"],
          [webrtcLink]);
      });

    } else {
      peer.createOffer().then((sessionDescription) => {
        description = sessionDescription;
        return peer.setLocalDescription(sessionDescription);

      }).then(() => {
        logD("rtc createOffer", webrtcLink);
        let [sdpPtr, sdpSiz] = allocPtrString(description.sdp);
        ccall("webrtc_link_on_csd_success", "null",
          ["number", "number", "number"],
          [webrtcLink, sdpPtr, sdpSiz]);
        freePtr(sdpPtr);

      }).catch((e) => {
        logE(e);
        ccall("webrtc_link_on_csd_failure", "null",
          ["number"],
          [webrtcLink]);
      });
    }

  } catch (e) {
    logE(e);
  }
}

function webrtcLinkSend(webrtcLink, dataPtr, dataSiz) {
  logD("rtc data send", webrtcLink);
  try {
    let link = availableWebrtcLinks.get(webrtcLink);
    let dataChannel = link.dataChannel;

    // avoid error : The provided ArrayBufferView value must not be shared.
    let data = new Uint8Array(dataSiz);
    for (let idx = 0; idx < dataSiz; idx++) {
      data[idx] = HEAPU8[dataPtr + idx];
    }

    dataChannel.send(data);

  } catch (e) {
    logE(e);
  }
}

function webrtcLinkSetRemoteSdp(webrtcLink, sdpPtr, sdpSiz, isOffer) {
  try {
    let link = availableWebrtcLinks.get(webrtcLink);
    let peer = link.peer;
    let sdp = {
      type: (isOffer ? "offer" : "answer"),
      sdp: UTF8ToString(sdpPtr, sdpSiz)
    };
    peer.setRemoteDescription(new RTCSessionDescription(sdp));

  } catch (e) {
    logE(e);
  }
}

function webrtcLinkUpdateIce(webrtcLink, icePtr, iceSiz) {
  try {
    let link = availableWebrtcLinks.get(webrtcLink);
    let peer = link.peer;
    let ice = JSON.parse(UTF8ToString(icePtr, iceSiz));

    peer.addIceCandidate(new RTCIceCandidate(ice));

  } catch (e) {
    logE(e);
  }
}

/* Module object for emscripten. */
Module["preRun"] = [];
Module["postRun"] = [
  initializeMap,
  initializePubsub2D,
  execFuncsAfterLoad
];

Module["print"] = function (text) {
  if (arguments.length > 1) {
    text = Array.prototype.slice.call(arguments).join(" ");
  }
  logD(text);
};

Module["printErr"] = function (text) {
  if (arguments.length > 1) {
    text = Array.prototype.slice.call(arguments).join(" ");
  }
  logE(text);
};

Module["Colonio"] = Colonio;
Module["Value"] = ColonioValue;
Module["Map"] = ColonioMap;
Module["Pubsub2D"] = ColonioPubsub2D;