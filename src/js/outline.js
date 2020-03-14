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
(function (exports) {
  "use strict";

  #ifdef NDEBUG
  #  define logd(...) // do nothing
  #else
  #  define logd(...) console.log((new Date()).toISOString(), __VA_ARGS__)
  #  define dumpPacket(str) function() { let j = JSON.parse(str); /* j.content = JSON.parse(j.content); */return j; } ()
  #endif

  const ID_MAX = 2147483647;

  /* Push/Pop object */
  let idObjectPair = new Object();

  function pushObject(obj) {
    let id = Math.floor(Math.random() * ID_MAX);
    if (id in idObjectPair) {
      id = Math.floor(Math.random() * ID_MAX);
    }
    idObjectPair[id] = obj;
    return id;
  }

  function popObject(id) {
    assert((id in idObjectPair), 'Wrong id :' + id);
    let obj = idObjectPair[id];
    delete idObjectPair[id];
    return obj;
  }

  function getObject(id) {
    assert((id in idObjectPair), 'Wrong id :' + id);
    return idObjectPair[id];
  }

  let funcsAfterLoad = [];

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

    for (let idx = 0; idx < execFuncs.length; idx++) {
      execFuncs[idx]();
    }
  }

  function allocPtr(siz) {
    return _malloc(siz);
  }

  function allocPtrString(str) {
    assert(typeof (str) === 'string');
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

  // [<ptr or id>]['<class>/<event type>'][]
  let eventFuncs = {};

  function setEventFunc(ptr, clazz, type, func) {
    assert(ptr !== null);

    // ptr = String(ptr);

    if (!(ptr in eventFuncs)) {
      eventFuncs[ptr] = {};
    }

    let key = clazz + '/' + type;
    if (!(key in eventFuncs[ptr])) {
      eventFuncs[ptr][key] = [];
    }

    eventFuncs[ptr][key].push(func);
  }

  function getEventFuncs(ptr, clazz, type) {
    //ptr = String(ptr);
    let key = clazz + '/' + type;
    if (ptr in eventFuncs &&
      key in eventFuncs[ptr]) {
      return eventFuncs[ptr][key];

    } else {
      return [];
    }
  }

  /**
   * This function replaces Pointer_stringify.
   * Pointer_strinfigy has a bug. It return wrong string when pass the value
   * that contain multi byte string and not have 0-terminate.
   * Pointer_stringify ignore length when use it with multi byte string.
   */
  function convertPointerToString(ptr, length) {
    if (length === 0 || !ptr) return '';
    // Find the length, and check for UTF while doing so
    var hasUtf = 0;
    var t;
    var i = 0;
    while (1) {
      #ifdef NDEBUG
      t = HEAPU8[ptr + i >> 0];
      #else
      t = ((SAFE_HEAP_LOAD((((ptr) + (i)) | 0), 1, 1)) | 0);
      #endif
      hasUtf |= t;
      if (t == 0 && !length) break;
      i++;
      if (length && i == length) break;
    }
    if (!length) length = i;

    var ret = '';

    if (hasUtf < 128) {
      var MAX_CHUNK = 1024; // split up into chunks, because .apply on a huge string can overflow the stack
      var curr;
      while (length > 0) {
        curr = String.fromCharCode.apply(String, HEAPU8.subarray(ptr, ptr + Math.min(length, MAX_CHUNK)));
        ret = ret ? ret + curr : curr;
        ptr += MAX_CHUNK;
        length -= MAX_CHUNK;
      }
      return ret;
    }

    let u8Array = Module.HEAPU8;
    let idx = ptr;
    var endPtr = idx + length;
    // TextDecoder needs to know the byte length in advance, it doesn't stop on null terminator by itself.
    // Also, use the length info to avoid running tiny strings through TextDecoder, since .subarray() allocates garbage.

    if (endPtr - idx > 16 && u8Array.subarray && UTF8Decoder) {
      return UTF8Decoder.decode(u8Array.subarray(idx, endPtr));
    } else {
      var u0, u1, u2, u3, u4, u5;

      var str = '';
      while (1) {
        // For UTF8 byte structure, see http://en.wikipedia.org/wiki/UTF-8#Description and https://www.ietf.org/rfc/rfc2279.txt and https://tools.ietf.org/html/rfc3629
        u0 = u8Array[idx++];
        if (!u0) return str;
        if (!(u0 & 0x80)) { str += String.fromCharCode(u0); continue; }
        u1 = u8Array[idx++] & 63;
        if ((u0 & 0xE0) == 0xC0) { str += String.fromCharCode(((u0 & 31) << 6) | u1); continue; }
        u2 = u8Array[idx++] & 63;
        if ((u0 & 0xF0) == 0xE0) {
          u0 = ((u0 & 15) << 12) | (u1 << 6) | u2;
        } else {
          u3 = u8Array[idx++] & 63;
          if ((u0 & 0xF8) == 0xF0) {
            u0 = ((u0 & 7) << 18) | (u1 << 12) | (u2 << 6) | u3;
          } else {
            u4 = u8Array[idx++] & 63;
            if ((u0 & 0xFC) == 0xF8) {
              u0 = ((u0 & 3) << 24) | (u1 << 18) | (u2 << 12) | (u3 << 6) | u4;
            } else {
              u5 = u8Array[idx++] & 63;
              u0 = ((u0 & 1) << 30) | (u1 << 24) | (u2 << 18) | (u3 << 12) | (u4 << 6) | u5;
            }
          }
        }
        if (u0 < 0x10000) {
          str += String.fromCharCode(u0);
        } else {
          var ch = u0 - 0x10000;
          str += String.fromCharCode(0xD800 | (ch >> 10), 0xDC00 | (ch & 0x3FF));
        }
      }
    }
  }

  class Colonio {
    // ATTENTION: Use same value with another languages.
    static get LOG_LEVEL_INFO() { return 0; }
    static get LOG_LEVEL_ERROR() { return 1; }
    static get LOG_LEVEL_DEBUG() { return 2; }

    // ATTENTION: Use same value with another languages.
    static get DEBUG_EVENT_MAP_SET() { return 0; }
    static get DEBUG_EVENT_LINKS() { return 1; }
    static get DEBUG_EVENT_NEXTS() { return 2; }
    static get DEBUG_EVENT_POSITION() { return 3; }
    static get DEBUG_EVENT_REQUIRED_1D() { return 4; }
    static get DEBUG_EVENT_REQUIRED_2D() { return 5; }
    static get DEBUG_EVENT_KNOWN_1D() { return 6; }
    static get DEBUG_EVENT_KNOWN_2D() { return 7; }

    // ATTENTION: Use same value with another languages.
    static get MAP_FAILURE_REASON_NONE() { return 0; }
    static get MAP_FAILURE_REASON_SYSTEM_ERROR() { return 1; }
    static get MAP_FAILURE_REASON_NOT_EXIST_KEY() { return 2; }
    // static get MAP_FAILURE_REASON_EXIST_KEY() { return 3; }
    static get MAP_FAILURE_REASON_CHANGED_PROPOSER() { return 3; }

    static get PUBSUB_2D_FAILURE_REASON_NONE() { return 0; }
    static get PUBSUB_2D_FAILURE_REASON_SYSTEM_ERROR() { return 1; }
    static get PUBSUB_2D_FAILURE_REASON_NOONE_RECV() { return 2; }

    static get NID_THIS() { return '.'; }

    constructor() {
      this._timerInvoke = false;
      this._colonioPtr = null;

      this._instanceCache = new Object();
    }

    init() {
      const promise = new Promise((resolve, reject) => {
        addFuncAfterLoad(() => {
          // Initialize module.
          this._colonioPtr = ccall('js_init', 'number',
            ['number'],
            [addFunction(this._onRequireInvoke.bind(this), 'vii')]);

          resolve();
        });
      });

      return promise;
    }

    connect(url, token) {
      const promise = new Promise((resolve, reject) => {
        let onSuccess = addFunction((colonioPtr) => {
          resolve();
        }, 'vi');

        let onFailure = addFunction((colonioPtr) => {
          reject();
        }, 'vi');

        let [urlPtr] = allocPtrString(url);
        let [tokenPtr] = allocPtrString(token);

        ccall('js_connect', 'null',
          ['number', 'number', 'number', 'number'],
          [this._colonioPtr, urlPtr, tokenPtr, onSuccess, onFailure]);

        freePtr(url);
        freePtr(token);
      });

      return promise;
    }

    _onRequireInvoke(colonioPtr, msec) {
      function invoke() {
        let next = ccall('js_invoke', 'number', ['number'], [colonioPtr]);
        if (this._timerInvoke !== false) {
          clearTimeout(this._timerInvoke);
        }
        this._timerInvoke = setTimeout(invoke.bind(this), next);
      }

      if (this._timerInvoke !== false) {
        clearTimeout(this._timerInvoke);
      }
      this._timerInvoke = setTimeout(invoke.bind(this), msec);
    }

    accessMap(name) {
      if (!(name in this._instanceCache)) {
        let [namePtr] = allocPtrString(name);
        this._instanceCache[name] = new ColonioMap(ccall('js_access_map', 'number',
          ['number', 'number'],
          [this._colonioPtr, namePtr]));
        freePtr(namePtr);
      }
      return this._instanceCache[name];
    }

    accessPubsub2D(name) {
      if (!(name in this._instanceCache)) {
        let [namePtr] = allocPtrString(name);
        this._instanceCache[name] = new Pubsub2D(ccall('js_access_pubsub_2d', 'number',
          ['number', 'number'],
          [this._colonioPtr, namePtr]));
        freePtr(namePtr);
      }
      return this._instanceCache[name];
    }

    disconnect() {
      if (this._timerInvoke) {
        clearTimeout(this._timerInvoke);
        this._timerInvoke = false;
      }

      ccall('js_disconnect', 'null', ['number'], [this._colonioPtr]);

      delete this._instanceCache;
    }

    getLocalNid() {
      let nidPtr = allocPtr(32 + 1);
      ccall('js_get_local_nid', 'null', ['number', 'number'], [this._colonioPtr, nidPtr]);
      let nid = UTF8ToString(nidPtr);
      freePtr(nidPtr);
      return nid;
    }

    setPosition(x, y) {
      ccall('js_set_position', 'null',
        ['number', 'number', 'number'],
        [this._colonioPtr, x, y]);
    }

    on(type, func) {
      const TYPES = ['log', 'debug'];
      assert(TYPES.indexOf(type) >= 0);

      switch (type) {
        case 'log':
          ccall('js_enable_output_log', 'null', ['number'], [this._colonioPtr]);
          break;

        case 'debug':
          ccall('js_enable_debug_event', 'null', ['number'], [this._colonioPtr]);
          break;
      }

      setEventFunc(this._colonioPtr, 'colonio', type, func);
    }
  };


  /* log */
  function jsOnOutputLog(colonioPtr, level, messagePtr, messageSiz) {
    let message = convertPointerToString(messagePtr, messageSiz);
    let funcs = getEventFuncs(colonioPtr, 'colonio', 'log');
    for (let idx = 0; idx < funcs.length; idx++) {
      funcs[idx]({
        type: 'log',
        level: level,
        content: message
      });
    }
  }

  /* debug event */
  function jsOnDebugEvent(colonioPtr, event, messagePtr, messageSiz) {
    let message = convertPointerToString(messagePtr, messageSiz);
    let json = JSON.parse(message);
    let funcs = getEventFuncs(colonioPtr, 'colonio', 'debug');
    for (let idx = 0; idx < funcs.length; idx++) {
      funcs[idx]({
        type: 'debug',
        event: event,
        content: json
      });
    }
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
      let valuePtr = ccall('js_value_init', 'number', []);
      return new ColonioValue(valuePtr);
    }

    static Bool(value) {
      let valuePtr = ccall('js_value_init', 'number', []);
      ccall('js_value_set_bool', 'null',
        ['number', 'boolean'],
        [valuePtr, value]);
      return new ColonioValue(valuePtr);
    }

    static Int(value) {
      let valuePtr = ccall('js_value_init', 'number', []);
      ccall('js_value_set_int', 'null',
        ['number', 'number'],
        [valuePtr, value]);
      return new ColonioValue(valuePtr);
    }

    static Double(value) {
      let valuePtr = ccall('js_value_init', 'number', []);
      ccall('js_value_set_double', 'null',
        ['number', 'number'],
        [valuePtr, value]);
      return new ColonioValue(valuePtr);
    }

    static String(value) {
      let valuePtr = ccall('js_value_init', 'number', []);
      let [stringPtr, stringSiz] = allocPtrString(value);
      ccall('js_value_set_string', 'null',
        ['number', 'number', 'number'],
        [valuePtr, stringPtr, stringSiz]);
      freePtr(stringPtr);
      return new ColonioValue(valuePtr);
    }

    constructor(valuePtr) {
      this._valuePtr = valuePtr;
    }

    getJsValue() {
      assert(this.isEnable(), 'Released value.');

      switch (ccall('js_value_get_type', 'number', ['number'], [this._valuePtr])) {
        case 0: // this.VALUE_TYPE_NULL:
          return null;

        case 1: // this.VALUE_TYPE_BOOL:
          return ccall('js_value_get_bool', 'boolean', ['number'], [this._valuePtr]);

        case 2: // this.VALUE_TYPE_INT:
          return ccall('js_value_get_int', 'number', ['number'], [this._valuePtr]);

        case 3: // this.VALUE_TYPE_DOUBLE:
          return ccall('js_value_get_double', 'number', ['number'], [this._valuePtr]);

        case 4: // this.VALUE_TYPE_STRING:
          let length = ccall('js_value_get_string_length', 'number', ['number'], [this._valuePtr]);
          let stringPtr = ccall('js_value_get_string', 'number', ['number'], [this._valuePtr]);
          return convertPointerToString(stringPtr, length);
      }
    }

    isEnable() {
      return !isNaN(this._valuePtr);
    }

    release() {
      assert(this.isEnable(), 'Double release error.');

      ccall('js_value_free', 'null', ['number'], [this._valuePtr]);
      this._valuePtr = NaN;
    }
  }

  let toString = Object.prototype.toString;
  function typeOf(obj) {
    return toString.call(obj).slice(8, -1).toLowerCase();
  }

  function convertValue(value) {
    if (value instanceof ColonioValue) {
      assert(value.isEnable(), 'Value is released.');
      return value;
    }

    switch (typeOf(value)) {
      case 'null':
        return ColonioValue.Null();

      case 'boolean':
        return ColonioValue.Bool(value)

      case 'string':
        return ColonioValue.String(value);

      case 'number':
        assert(false, 'Number is not explicit value type. Please use ColonioValue::Int or ColonioValue::Double.');
        break;

      default:
        assert(false, 'Unsupported value type');
        break;
    }
  }

  function initializeMap() {
    let onMapGet = addFunction((id, reason, valuePtr) => {
      popObject(id)(reason, valuePtr);
    }, 'viii');

    let onMapSet = addFunction((id, reason) => {
      popObject(id)(reason);
    }, 'vii');

    ccall('js_map_init', 'null', ['number', 'number'], [onMapGet, onMapSet]);
  }

  class ColonioMap {
    constructor(mapPtr) {
      this._mapPtr = mapPtr;
    }

    getValue(key) {
      let promise = new Promise((resolve, reject) => {
        const keyValue = convertValue(key);

        let id = pushObject((reason, valuePtr) => {
          if (reason === Colonio.MAP_FAILURE_REASON_NONE) {
            const val = new ColonioValue(valuePtr);
            resolve(val.getJsValue());
            // val.release(); valuePtr will release on colonio_map_get@export_c.cpp

          } else {
            reject(reason);
          }
        });

        ccall('js_map_get_value', 'null',
          ['number', 'number', 'number'],
          [this._mapPtr, keyValue._valuePtr, id]);

        keyValue.release();
      });

      return promise;
    }

    setValue(key, val, opt = 0) {
      let promise = new Promise((resolve, reject) => {
        const keyValue = convertValue(key);
        const valValue = convertValue(val);

        let id = pushObject((reason) => {
          if (reason === Colonio.MAP_FAILURE_REASON_NONE) {
            resolve();
          } else {
            reject(reason);
          }
        });

        ccall('js_map_set_value', 'null',
          ['number', 'number', 'number', 'number', 'number'],
          [this._mapPtr, keyValue._valuePtr, valValue._valuePtr, id, opt]);

        keyValue.release();
        valValue.release();
      });

      return promise;
    }
  }

  function initializePubsub2D() {
    let onPublish = addFunction((id, reason) => {
      popObject(id)(reason);
    }, 'vii');

    let onOn = addFunction((id, valuePtr) => {
      getObject(id)(valuePtr);
    }, 'vii');

    ccall('js_pubsub_2d_init', 'null', ['number', 'number'], [onPublish, onOn]);
  }

  class Pubsub2D {
    constructor(pubsub2DPtr) {
      this._pubsub2DPtr = pubsub2DPtr;
    }

    publish(name, x, y, r, val) {
      let promise = new Promise((resolve, reject) => {
        let [namePtr, nameSiz] = allocPtrString(name);
        const value = convertValue(val);

        let id = pushObject((reason) => {
          if (reason === Colonio.PUBSUB_2D_FAILURE_REASON_NONE) {
            resolve();
          } else {
            reject(reason);
          }
        });

        ccall('js_pubsub_2d_publish', 'null',
          ['number', 'number', 'number', 'number', 'number', 'number', 'number', 'number'],
          [this._pubsub2DPtr, namePtr, nameSiz, x, y, r, value._valuePtr, id]);

        freePtr(namePtr);
        value.release();
      });

      return promise;
    }

    on(name, cb) {
      let id = pushObject((valuePtr) => {
        const val = new ColonioValue(valuePtr);
        cb(val.getJsValue());
      });

      let [namePtr, nameSiz] = allocPtrString(name);

      ccall('js_pubsub_2d_on', 'null',
        ['number', 'number', 'number', 'number'],
        [this._pubsub2DPtr, namePtr, nameSiz, id]);

      freePtr(namePtr);
    }

    off(name) {
      let [namePtr, nameSiz] = allocPtrString(name);

      ccall('js_pubsub_2d_off', 'null',
        ['number', 'number', 'number'],
        [this._pubsub2DPtr, namePtr, nameSiz]);

      freePtr(namePtr);
    }
  }  // class Pubsub2D

  /* SeedLinkWebsocket */
  let availableSeedLinks = {};

  function seedLinkWsConnect(seedLink, urlPtr, urlSiz) {
    let url = convertPointerToString(urlPtr, urlSiz);
    logd('socket connect', seedLink, url);
    let socket = new WebSocket(url);
    socket.binaryType = 'arraybuffer';
    availableSeedLinks[seedLink] = socket;

    socket.onopen = () => {
      logd('socket open', seedLink);
      if (seedLink in availableSeedLinks) {
        ccall('seed_link_ws_on_connect', 'null', ['number'], [seedLink]);
      }
    };

    socket.onerror = (error) => {
      logd('socket error', seedLink, error);
      if (seedLink in availableSeedLinks) {
        let [msgPtr, msgSiz] = allocPtrString(JSON.stringify(error));

        ccall('seed_link_ws_on_error', 'null',
          ['number', 'number', 'number'],
          [seedLink, msgPtr, msgSiz]);

        freePtr(msgPtr);
      }
    };

    socket.onmessage = (e) => {
      //logd('socket recv', seedLink, dumpPacket(e.data));
      if (seedLink in availableSeedLinks) {
        let [dataPtr, dataSiz] = allocPtrArrayBuffer(e.data);

        ccall('seed_link_ws_on_recv', 'null',
          ['number', 'number', 'number'],
          [seedLink, dataPtr, dataSiz]);

        freePtr(dataPtr);
      }
    };

    socket.onclose = () => {
      logd('socket close', seedLink);
      if (seedLink in availableSeedLinks) {
        ccall('seed_link_ws_on_disconnect', 'null', ['number'], [seedLink]);
      }
    }
  }

  function seedLinkWsSend(seedLink, dataPtr, dataSiz) {
    logd('socket send', seedLink);
    assert(seedLink in availableSeedLinks);

    let data = new Uint8Array(HEAP8.buffer, dataPtr, dataSiz);
    availableSeedLinks[seedLink].send(data);
  }

  function seedLinkWsDisconnect(seedLink) {
    logd('socket, disconnect', seedLink);
    if (seedLink in availableSeedLinks) {
      availableSeedLinks[seedLink].close();
    } else {
      logd('double disconnect', seedLink);
    }
  }

  function seedLinkWsFinalize(seedLink) {
    logd('socket finalize', seedLink);
    if (seedLink in availableSeedLinks) {
      // 2 : CLOSING
      // 3 : CLOSED
      if (availableSeedLinks[seedLink].readyState != 2 &&
        availableSeedLinks[seedLink].readyState != 3) {
        availableSeedLinks[seedLink].close();
      }

      delete availableSeedLinks[seedLink];
    } else {
      logd('double finalize', seedLink);
    }
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
      JSON.parse(convertPointerToString(strPtr, strSiz))
    );
  }

  /* WebrtcLink */
  let availableWebrtcLinks = {};

  function webrtcLinkInitialize(webrtcLink, isCreateDc) {
    logd('rtc initialize', webrtcLink);

    let setEvent = (dataChannel) => {
      dataChannel.onerror = (error) => {
        logd('rtc data error', webrtcLink, error);
        if (webrtcLink in availableWebrtcLinks) {
          let [messagePtr, messageSiz] = allocPtrString(error.message);
          ccall('webrtc_link_on_dco_error', 'null',
            ['number', 'number', 'number'],
            [webrtcLink, messagePtr, messageSiz]);
          freePtr(messagePtr);
        }
      };

      dataChannel.onmessage = (event) => {
        if (webrtcLink in availableWebrtcLinks) {
          if (event.data instanceof ArrayBuffer) {
            // logd('rtc data recv', webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(event.data)));
            let [dataPtr, dataSiz] = allocPtrArrayBuffer(event.data);
            ccall('webrtc_link_on_dco_message', 'null',
              ['number', 'number', 'number'],
              [webrtcLink, dataPtr, dataSiz]);
            freePtr(dataPtr);

          } else if (event.data instanceof Blob) {
            let reader = new FileReader();
            reader.onload = () => {
              // logd('rtc data recv', webrtcLink, dumpPacket(new TextDecoder("utf-8").decode(reader.result)));
              let [dataPtr, dataSiz] = allocPtrArrayBuffer(reader.result);
              ccall('webrtc_link_on_dco_message', 'null',
                ['number', 'number', 'number'],
                [webrtcLink, dataPtr, dataSiz]);
              freePtr(dataPtr);
            };
            reader.readAsArrayBuffer(event.data);

          } else {
            console.error("Unsupported type of message.");
            console.error(event.data);
          }
        }
      };

      dataChannel.onopen = () => {
        logd('rtc data open', webrtcLink);
        if (webrtcLink in availableWebrtcLinks) {
          ccall('webrtc_link_on_dco_open', 'null',
            ['number'],
            [webrtcLink]);
        }
      };

      dataChannel.onclose = () => {
        logd('rtc data close', webrtcLink);
        if (webrtcLink in availableWebrtcLinks) {
          ccall('webrtc_link_on_dco_close', 'null',
            ['number'],
            [webrtcLink]);
        }
      };
    };

    let peer = null;
    try {
      peer = new RTCPeerConnection(webrtcContextPcConfig);

    } catch (e) {
      console.error(e);
      peer = null;
    }

    if (peer === null) {
      console.error('RTCPeerConnection');
    }

    let dataChannel = null;
    if (isCreateDc) {
      dataChannel = peer.createDataChannel('data_channel',
        webrtcContextDcConfig);
      setEvent(dataChannel);
    }

    availableWebrtcLinks[webrtcLink] = {
      peer: peer,
      dataChannel: dataChannel
    };

    peer.onicecandidate = (event) => {
      logd('rtc on ice candidate', webrtcLink);
      if (webrtcLink in availableWebrtcLinks) {
        let ice;
        if (event.candidate) {
          ice = JSON.stringify(event.candidate);
        } else {
          ice = '';
        }

        let [icePtr, iceSiz] = allocPtrString(ice);
        ccall('webrtc_link_on_pco_ice_candidate', 'null',
          ['number', 'number', 'number'],
          [webrtcLink, icePtr, iceSiz]);
        freePtr(icePtr);
      }
    };

    peer.ondatachannel = (event) => {
      logd('rtc peer datachannel', webrtcLink);
      if (webrtcLink in availableWebrtcLinks) {
        let link = availableWebrtcLinks[webrtcLink];

        if (link.dataChannel !== null) {
          let [messagePtr, messageSiz] = allocPtrString("duplicate data channel.");
          ccall('webrtc_link_on_dco_error', 'null',
            ['number', 'number', 'number'],
            [webrtcLink, messagePtr, messageSiz]);
          freePtr(messagePtr);
        }

        link.dataChannel = event.channel;
        setEvent(event.channel);
      }
    };

    peer.oniceconnectionstatechange = (event) => {
      logd('rtc peer state', webrtcLink, peer.iceConnectionState);
      if (webrtcLink in availableWebrtcLinks) {
        let link = availableWebrtcLinks[webrtcLink];
        let peer = link.peer;
        let [statePtr, stateSiz] = allocPtrString(peer.iceConnectionState);
        ccall('webrtc_link_on_pco_state_change', 'null',
          ['number', 'number', 'number'],
          [webrtcLink, statePtr, stateSiz]);
        freePtr(statePtr);
      }
    };
  }

  function webrtcLinkFinalize(webrtcLink) {
    logd('rtc finalize', webrtcLink);
    assert((webrtcLink in availableWebrtcLinks));
    delete availableWebrtcLinks[webrtcLink];
  }

  function webrtcLinkDisconnect(webrtcLink) {
    logd('rtc disconnect', webrtcLink);
    assert((webrtcLink in availableWebrtcLinks));

    if (webrtcLink in availableWebrtcLinks) try {
      let link = availableWebrtcLinks[webrtcLink];

      if (link.dataChannel !== null) {
        link.dataChannel.close();
      }

      if (link.peer !== null) {
        link.peer.close();
      }
    } catch (e) {
      console.error(e);
    }
  }

  function webrtcLinkGetLocalSdp(webrtcLink, isRemoteSdpSet) {
    logd('rtc getLocalSdp', webrtcLink);
    assert((webrtcLink in availableWebrtcLinks));

    try {
      let link = availableWebrtcLinks[webrtcLink];
      let peer = link.peer;
      let description;

      if (isRemoteSdpSet) {
        peer.createAnswer().then((sessionDescription) => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then(() => {
          logd('rtc createAnswer', webrtcLink);
          let [sdpPtr, sdpSiz] = allocPtrString(description.sdp);
          ccall('webrtc_link_on_csd_success', 'null',
            ['number', 'number', 'number'],
            [webrtcLink, sdpPtr, sdpSiz]);
          freePtr(sdpPtr);

        }).catch((e) => {
          logd('rtc createAnswer error', webrtcLink, e);
          ccall('webrtc_link_on_csd_failure', 'null',
            ['number'],
            [webrtcLink]);
        });

      } else {
        peer.createOffer().then((sessionDescription) => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then(() => {
          logd('rtc createOffer', webrtcLink);
          let [sdpPtr, sdpSiz] = allocPtrString(description.sdp);
          ccall('webrtc_link_on_csd_success', 'null',
            ['number', 'number', 'number'],
            [webrtcLink, sdpPtr, sdpSiz]);
          freePtr(sdpPtr);

        }).catch((e) => {
          console.error(e);
          ccall('webrtc_link_on_csd_failure', 'null',
            ['number'],
            [webrtcLink]);
        });
      }

    } catch (e) {
      console.error(e);
    }
  }

  function webrtcLinkSend(webrtcLink, dataPtr, dataSiz) {
    logd('rtc data send', webrtcLink);
    try {
      let link = availableWebrtcLinks[webrtcLink];
      let dataChannel = link.dataChannel;

      dataChannel.send(new Uint8Array(Module.HEAPU8.buffer, dataPtr, dataSiz));

    } catch (e) {
      console.error(e);
    }
  }

  function webrtcLinkSetRemoteSdp(webrtcLink, sdpPtr, sdpSiz, isOffer) {
    try {
      let link = availableWebrtcLinks[webrtcLink];
      let peer = link.peer;
      let sdp = {
        type: (isOffer ? 'offer' : 'answer'),
        sdp: convertPointerToString(sdpPtr, sdpSiz)
      };
      peer.setRemoteDescription(new RTCSessionDescription(sdp));

    } catch (e) {
      console.error(e);
    }
  }

  function webrtcLinkUpdateIce(webrtcLink, icePtr, iceSiz) {
    try {
      let link = availableWebrtcLinks[webrtcLink];
      let peer = link.peer;
      let ice = JSON.parse(convertPointerToString(icePtr, iceSiz));

      peer.addIceCandidate(new RTCIceCandidate(ice));

    } catch (e) {
      console.error(e);
    }
  }

  // Export classes.
  exports.Colonio = Colonio;

  /* Module object for emscripten. */
  var Module = new Object();
  Module['preRun'] = [];
  Module['postRun'] = [
    initializeMap,
    initializePubsub2D,
    execFuncsAfterLoad
  ];

  Module['print'] = function (text) {
    if (arguments.length > 1) {
      text = Array.prototype.slice.call(arguments).join(' ');
    }
    console.log(text);
  };

  Module['printErr'] = function (text) {
    if (arguments.length > 1) {
      text = Array.prototype.slice.call(arguments).join(' ');
    }
    console.error(text);
  };

  #include "colonio.js"
})(typeof exports === 'undefined' ? this.colonio = {} : exports);
