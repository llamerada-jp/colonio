"use strict";

// const WebSocket = require("ws");
// const { RTCPeerConnection, RTCSessionDescription } = require("wrtc");
// const ColonioModule = require("./colonio");

const URL = "ws://localhost:8080/test";
const TOKEN = "";
const KEY = "hoge";

/*global ColonioModule*/
/* eslint no-console: ["error", { allow: ["assert", "error", "log"] }] */

function test() {
  let node1;
  let node2;
  let timer;
  let stored = new Map();

  return ColonioModule().then((colonio) => {
    console.log("new node1");
    let config1 = new colonio.ColonioConfig();
    config1.loggerFunc = (_, log) => {
      log.node = "node1";
      console.log(log);
    };

    node1 = new colonio.Colonio(config1);

    console.log("new node2");
    let config2 = new colonio.ColonioConfig();
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
    node2.messagingSetHandler("test", (request, writer) => {
      writer.write("reply " + request.message.getJsValue());
    });

    console.log("wait 5 sec");
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve();
      }, 5 * 1000);
    });

  }).then(() => {
    return node1.messagingPost(node2.getLocalNid(), "test", "from node1");

  }).then((result) => {
    // check result
    if (result.getJsValue() !== "reply from node1") {
      console.error("result", result);
      throw new Error("wrong result from node2.onCall");
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

  }).catch((e) => {
    let result = document.getElementById("result");
    result.innerText = "FAILURE";
    console.error(e);
  });
}

test();
