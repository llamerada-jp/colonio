/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
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
  static get ERROR_CODE_OFFLINE() { return 2; }
  static get ERROR_CODE_INCORRECT_DATA_FORMAT() { return 3; }
  static get ERROR_CODE_CONFLICT_WITH_SETTING() { return 4; }
  static get ERROR_CODE_NOT_EXIST_KEY() { return 5; }
  static get ERROR_CODE_EXIST_KEY() { return 6; }
  static get ERROR_CODE_CHANGED_PROPOSER() { return 7; }
  static get ERROR_CODE_COLLISION_LATE() { return 8; }
  static get ERROR_CODE_NO_ONE_RECV() { return 9; }

  static get NID_THIS() { return "."; }

  constructor() {
    this._colonioPtr = null;
    this._instanceCache = new Map();

    addFuncAfterLoad(() => {
      // initialize
      let setPositionOnSuccess = Module.addFunction((id, newX, newY) => {
        popObject(id).onSuccess({
          x: newX,
          y: newY
        });
      }, "vidd");

      let setPositionOnFailure = Module.addFunction((id, errorPtr) => {
        popObject(id).onFailure(convertError(errorPtr));
      }, "vii");

      this._colonioPtr = ccall("js_init", "number",
        ["number", "number"],
        [setPositionOnSuccess, setPositionOnFailure]);
    });
  }

  connect(url, token) {
    const promise = new Promise((resolve, reject) => {
      addFuncAfterLoad(() => {
        var onSuccess = Module.addFunction((colonioPtr) => {
          resolve();
          removeFunction(onSuccess);
          removeFunction(onFailure);
        }, "vi");

        var onFailure = Module.addFunction((colonioPtr, errorPtr) => {
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

  on(type, func) {
    addFuncAfterLoad(() => {
      const TYPES = ["log"];
      assert(TYPES.indexOf(type) >= 0);

      switch (type) {
        case "log":
          ccall("js_enable_output_log", "null", ["number"], [this._colonioPtr]);
          break;
      }

      setEventFunc(this._colonioPtr, "colonio", type, func);
    });
  }
}

/* log */
function jsOnOutputLog(colonioPtr, jsonPtr, jsonSiz) {
  let json = UTF8ToString(jsonPtr, jsonSiz);
  let funcs = getEventFuncs(colonioPtr, "colonio", "log");
  if (funcs === null) {
    return;
  }
  for (let idx = 0; idx < funcs.length; idx++) {
    funcs[idx](JSON.parse(json));
  }
}

/* APIGate */
let gateTimers = new Map();

function apiGateRelease(gatePtr) {
  if (gateTimers.has(gatePtr)) {
    clearTimeout(gateTimers.get(gatePtr));
    gateTimers.delete(gatePtr);
  }
}

function apiGateRequireCallAfter(gatePtr, id) {
  setTimeout(() => {
    if (gateTimers.has(gatePtr)) {
      ccall("api_gate_call", "null", ["number", "number"], [gatePtr, id]);
    }
  }, 0);
}

function apiGateRequireInvoke(gatePtr, msec) {
  if (gateTimers.has(gatePtr)) {
    clearTimeout(gateTimers.get(gatePtr));
  }
  gateTimers.set(gatePtr, setTimeout(() => {
    ccall("api_gate_invoke", "null", ["number"], [gatePtr]);
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
    let valuePtr = ccall("js_value_init", "number", []);
    return new ColonioValue(valuePtr, false);
  }

  static Bool(value) {
    let valuePtr = ccall("js_value_init", "number", []);
    ccall("js_value_set_bool", "null",
      ["number", "boolean"],
      [valuePtr, value]);
    return new ColonioValue(valuePtr, false);
  }

  static Int(value) {
    let valuePtr = ccall("js_value_init", "number", []);
    ccall("js_value_set_int", "null",
      ["number", "number"],
      [valuePtr, value]);
    return new ColonioValue(valuePtr, false);
  }

  static Double(value) {
    let valuePtr = ccall("js_value_init", "number", []);
    ccall("js_value_set_double", "null",
      ["number", "number"],
      [valuePtr, value]);
    return new ColonioValue(valuePtr, false);
  }

  static String(value) {
    let valuePtr = ccall("js_value_init", "number", []);
    let [stringPtr, stringSiz] = allocPtrString(value);
    ccall("js_value_set_string", "null",
      ["number", "number", "number"],
      [valuePtr, stringPtr, stringSiz]);
    freePtr(stringPtr);
    return new ColonioValue(valuePtr, false);
  }

  constructor(valuePtr, load = true) {
    this._valuePtr = valuePtr;
    if (load) {
      this.reload();
    }
  }

  getType() {
    return this._type;
  }

  getJsValue() {
    return this._value;
  }

  isEnable() {
    return !isNaN(this._valuePtr);
  }

  release() {
    assert(this.isEnable(), "Double release error.");

    ccall("js_value_free", "null", ["number"], [this._valuePtr]);
    this._valuePtr = NaN;
  }

  reload() {
    assert(this.isEnable(), "Released value.");

    this._type = ccall("js_value_get_type", "number", ["number"], [this._valuePtr]);
    
    switch (this.getType()) {
      case 0: // this.VALUE_TYPE_NULL:
        this._value = null;
        break;

      case 1: // this.VALUE_TYPE_BOOL:
        this._value = ccall("js_value_get_bool", "boolean", ["number"], [this._valuePtr]);
        break;

      case 2: // this.VALUE_TYPE_INT:
        this._value = ccall("js_value_get_int", "number", ["number"], [this._valuePtr]);
        break;

      case 3: // this.VALUE_TYPE_DOUBLE:
        this._value = ccall("js_value_get_double", "number", ["number"], [this._valuePtr]);
        break;

      case 4: // this.VALUE_TYPE_STRING:
        let length = ccall("js_value_get_string_length", "number", ["number"], [this._valuePtr]);
        let stringPtr = ccall("js_value_get_string", "number", ["number"], [this._valuePtr]);
        this._value = UTF8ToString(stringPtr, length);
        break;
    }
  }
}

let toString = Object.prototype.toString;
function typeOf(obj) {
  return toString.call(obj).slice(8, -1).toLowerCase();
}

function convertValue(value) {
  if (value instanceof ColonioValue) {
    assert(value.isEnable(), "Value is released.");
    return value;
  }

  switch (typeOf(value)) {
    case "null":
      return ColonioValue.Null();

    case "boolean":
      return ColonioValue.Bool(value);

    case "string":
      return ColonioValue.String(value);

    case "number":
      assert(false, "Number is not explicit value type. Please use ColonioValue::Int or ColonioValue::Double.");
      break;

    default:
      assert(false, "Unsupported value type");
      break;
  }
}

function initializeMap() {
  let getValueOnSuccess = Module.addFunction((id, valuePtr) => {
    popObject(id).onSuccess(new ColonioValue(valuePtr));
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
    ["number", "number", "number", "number"],
    [getValueOnSuccess, getValueOnFailure, setValueOnSuccess, setValueOnFailure]);
}

class ColonioMap {
  static get ERROR_WITHOUT_EXIST() { return 0x1; }
  static get ERROR_WITH_EXIST() { return 0x2; }

  constructor(mapPtr) {
    this._mapPtr = mapPtr;
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
      const keyValue = convertValue(key);

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_map_get_value", "null",
        ["number", "number", "number"],
        [this._mapPtr, keyValue._valuePtr, id]);

      keyValue.release();
    });

    return promise;
  }

  setValue(key, val, opt = 0) {
    let promise = new Promise((resolve, reject) => {
      const keyValue = convertValue(key);
      const valValue = convertValue(val);

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_map_set_value", "null",
        ["number", "number", "number", "number", "number"],
        [this._mapPtr, keyValue._valuePtr, valValue._valuePtr, opt, id]);

      keyValue.release();
      valValue.release();
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
    getObject(id)(new ColonioValue(valuePtr));
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
      const value = convertValue(val);

      let id = pushObject({
        onSuccess: resolve,
        onFailure: reject,
      });

      ccall("js_pubsub_2d_publish", "null",
        ["number", "number", "number", "number", "number", "number", "number", "number", "number"],
        [this._pubsub2DPtr, namePtr, nameSiz, x, y, r, value._valuePtr, opt, id]);

      freePtr(namePtr);
      value.release();
    });

    return promise;
  }

  on(name, cb) {
    this.onRaw(name, (val) => {
      cb(val.getJsValue());
    });
  }

  onRaw(name, cb) {
    let id = pushObject(cb);

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