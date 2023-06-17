"use strict";

// const WebSocket = require("ws");
// const { RTCPeerConnection, RTCSessionDescription } = require("wrtc");
// const ColonioModule = require("./colonio");

const URL = "https://localhost:8080/test";
const TOKEN = "";
const KEY = "hoge";

/*global ColonioModule*/
/* eslint no-console: ["error", { allow: ["assert", "error", "log"] }] */

function test() {
  let node1;
  let node2;
  let colonio;
  let timer;
  let stored = new Map();

  return ColonioModule().then((c) => {
    colonio = c;
    console.log("new node1");
    let config1 = new colonio.ColonioConfig();
    config1.disableSeedVerification = true;
    config1.loggerFunc = (_, log) => {
      log.node = "node1";
      console.log(log);
    };

    node1 = new colonio.Colonio(config1);

    console.log("new node2");
    let config2 = new colonio.ColonioConfig();
    config2.disableSeedVerification = true;
    config2.loggerFunc = (_, log) => {
      log.node = "node2";
      console.log(log);
    };
    node2 = new colonio.Colonio(config2);

    console.log("node1.connect");
    return node1.connect(URL, TOKEN);

  }).then(() => {
    console.log("node2.connect");
    return node2.connect(URL, TOKEN);

  }).then(() => {
    node2.messagingSetHandler("null", (request, writer) => {
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_NULL) {
        console.error(request.message);
        throw new Error("wrong request");
      }
      writer.write(null);
    });

    node2.messagingSetHandler("bool", (request, writer) => {
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_BOOL ||
        request.message.getJsValue() !== true) {
        console.error(request.message);
        throw new Error("wrong request");
      }
      writer.write(false);
    });

    node2.messagingSetHandler("int", (request, writer) => {
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_INT ||
        request.message.getJsValue() !== 1) {
        console.error(request.message);
        throw new Error("wrong request");
      }
      writer.write(colonio.Value.newInt(2));
    });

    node2.messagingSetHandler("double", (request, writer) => {
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_DOUBLE ||
        request.message.getJsValue() !== 3.14) {
        console.error(request.message);
        throw new Error("wrong request");
      }
      writer.write(colonio.Value.newDouble(2.71));
    });

    node2.messagingSetHandler("string", (request, writer) => {
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_STRING ||
        request.message.getJsValue() !== "hello") {
        console.error(request.message);
        throw new Error("wrong request");
      }
      writer.write("world");
    });

    node2.messagingSetHandler("binary", (request, writer) => {
      let arrayBuf = request.message.getJsValue();
      let bin = new Uint8Array(arrayBuf);
      if (request.message.getType() !== colonio.Value.VALUE_TYPE_BINARY ||
        bin[0] !== 0 || bin[1] !== 1 || bin[2] !== 4 || bin[3] !== 9 || bin[4] !== 16 || bin[5] !== 25) {
        console.error(request.message);
        throw new Error("wrong request");
      }
      let res = new Uint8Array([25, 16, 9, 4, 1, 0]);
      writer.write(res.buffer);
    });
    console.log("wait 5 sec");
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve();
      }, 5 * 1000);
    });

  }).then(() => {
    // check nil
    return node1.messagingPost(node2.getLocalNid(), "null", null);

  }).then((result) => {
    if (result.getJsValue() !== null) {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    // check bool
    return node1.messagingPost(node2.getLocalNid(), "bool", true);

  }).then((result) => {
    if (result.getJsValue() !== false) {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    // check int
    return node1.messagingPost(node2.getLocalNid(), "int", colonio.Value.newInt(1));
  }).then((result) => {
    if (result.getJsValue() !== 2) {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    // check double
    return node1.messagingPost(node2.getLocalNid(), "double", colonio.Value.newDouble(3.14));
  }).then((result) => {
    if (result.getJsValue() !== 2.71) {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    // check string
    return node1.messagingPost(node2.getLocalNid(), "string", "hello");
  }).then((result) => {
    if (result.getJsValue() !== "world") {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    // check binary
    let buf = new Uint8Array([0, 1, 4, 9, 16, 25]);
    return node1.messagingPost(node2.getLocalNid(), "binary", buf.buffer);
  }).then((result) => {
    let arrayBuf = result.getJsValue();
    let bin = new Uint8Array(arrayBuf);
    if (bin[0] !== 25 || bin[1] !== 16 || bin[2] !== 9 || bin[3] !== 4 || bin[4] !== 1 || bin[5] !== 0) {
      console.error("result", result);
      throw new Error("wrong result from node2.messagingSetHandler");
    }

    console.log("node1.setPosition");
    return node1.setPosition(0, 0);

  }).then((pos) => {
    // check result
    if (pos.x !== 0 || pos.y !== 0) {
      console.error("result", pos);
      throw new Error("wrong result from node1.setPosition");
    }

    console.log("node2.setPosition");
    return node2.setPosition(0, 0);

  }).then((pos) => {
    // check result
    if (pos.x !== 0 || pos.y !== 0) {
      console.error("result", pos);
      throw new Error("wrong result from node1.setPosition");
    }

    timer = setInterval(() => {
      console.log("post data");
      node1.spreadPost(0, 0, 1.0, KEY, "from node1");
      node2.spreadPost(0, 0, 1.0, KEY, "from node2");
    }, 1000);

    return new Promise((resolve, reject) => {
      console.log("node1 waiting data");
      node1.spreadSetHandler(KEY, (request) => {
        console.log("receive node1", request);
        if (request.message.getJsValue() === "from node2") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    return new Promise((resolve, reject) => {
      console.log("node2 waiting data");
      node2.spreadSetHandler(KEY, (request) => {
        clearInterval(timer);
        console.log("receive node2", request);
        if (request.message.getJsValue() === "from node1") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    console.log("set key1");
    return node1.kvsSet("key1", "value1");

  }).then(() => {
    console.log("set key2");
    return node2.kvsSet("key2", "value2");

  }).then(() => {
    console.log("get key2");
    return node1.kvsGet("key2");

  }).then((val) => {
    return new Promise((resolve, reject) => {
      if (val.getJsValue() !== "value2") {
        console.log("got value is wrong");
        reject();
      } else {
        resolve();
      }
    });

  }).then(() => {
    return node1.kvsGetLocalData();
  }).then((localData1) => {
    for (const k of localData1.getKeys()) {
      stored.set(k, localData1.getValue(k).getJsValue());
    }
    // localData1.free();

    return node2.kvsGetLocalData();

  }).then((localData2) => {
    return new Promise((resolve, reject) => {
      for (const k of localData2.getKeys()) {
        if (stored.has(k)) {
          console.log("duplicate local value");
          reject();
        }
        stored.set(k, localData2.getValue(k).getJsValue());
      }
      localData2.free();

      if (stored.get("key1") === "value1" && stored.get("key2") === "value2") {
        resolve();
      } else {
        console.log("values are wrong", stored);
        reject();
      }
    });

  }).then(() => {
    console.log("node1.disconnect");
    return node1.disconnect();

  }).then(() => {
    console.log("node2.disconnect");
    return node2.disconnect();

  }).then(() => {
    let result = document.getElementById("result");
    result.innerText = "SUCCESS";
    console.log("SUCCESS");

  }).catch((e) => {
    let result = document.getElementById("result");
    result.innerText = "FAILURE";
    console.error(e);
    console.log("FAILURE");
  });
}

test();
