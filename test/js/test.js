"use strict";

// const WebSocket = require("ws");
// const { RTCPeerConnection, RTCSessionDescription } = require("wrtc");
// const ColonioModule = require("./colonio");

const URL = "ws://localhost:8080/test";
const TOKEN = "";
const KEY = "hoge";

/*global ColonioModule*/
// eslint-disable-next-line no-console
let logD = console.log;
// eslint-disable-next-line no-console
let logE = console.error;
// eslint-disable-next-line no-console
let assert = console.assert;

function test() {
  let node1;
  let node2;
  let ps1;
  let ps2;
  let map1;
  let map2;
  let timer1;
  let timer2;

  return ColonioModule().then((colonio) => {
    logD("new node1");
    node1 = new colonio.Colonio((str) => {
      let j = JSON.parse(str);
      j.node = "node1";
      if (j.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        j.level === colonio.Colonio.LOG_LEVEL_WARN) {
        logE(j);
      } else {
        logD(j);
      }
    });

    logD("new node2");
    node2 = new colonio.Colonio((str) => {
      let j = JSON.parse(str);
      j.node = "node2";
      if (j.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        j.level === colonio.Colonio.LOG_LEVEL_WARN) {
        logE(j);
      } else {
        logD(j);
      }
    });

    logD("node1.connect");
    return node1.connect(URL, TOKEN);

  }).then(() => {
    logD("node2.connect");
    return node2.connect(URL, TOKEN);

  }).then(() => {
    timer1 = setInterval(() => {
      logD("send data");
      node1.send(node2.getLocalNid(), "from node1");
    }, 1000);

    return new Promise((resolve, reject) => {
      logD("node2 waiting data");
      node2.on((data) => {
        clearInterval(timer1);
        if (data === "from node1") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    logD("node1.setPosition");
    return node1.setPosition(0, 0);

  }).then((pos) => {
    // check result
    if (pos.x !== 0 || pos.y !== 0) {
      logE("result", pos);
      throw new Error("wrong result from node1.setPosition");
    }

    logD("node2.setPosition");
    return node2.setPosition(0, 0);

  }).then((pos) => {
    // check result
    if (pos.x !== 0 || pos.y !== 0) {
      logE("result", pos);
      throw new Error("wrong result from node1.setPosition");
    }

    logD("get accessor");
    ps1 = node1.accessPubsub2D("ps");
    ps2 = node2.accessPubsub2D("ps");

    timer2 = setInterval(() => {
      logD("publish data");
      ps1.publish(KEY, 0, 0, 1.0, "from ps1");
      ps2.publish(KEY, 0, 0, 1.0, "from ps2");
    }, 1000);

    return new Promise((resolve, reject) => {
      logD("ps1 waiting data");
      ps1.on(KEY, (data) => {
        logD("receive ps1", data);
        if (data === "from ps2") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    return new Promise((resolve, reject) => {
      logD("ps2 waiting data");
      ps2.on(KEY, (data) => {
        clearInterval(timer2);
        logD("receive ps2", data);
        if (data === "from ps1") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    logD("get map accessor");
    map1 = node1.accessMap("mp");
    map2 = node2.accessMap("mp");

    logD("set key1");
    return map1.setValue("key1", "value1");

  }).then(() => {
    logD("set key2");
    return map2.setValue("key2", "value2");

  }).then(() => {
    logD("get key2");
    return map1.getValue("key2");

  }).then((val) => {
    return new Promise((resolve, reject) => {
      if (val !== "value2") {
        logD("got value is wrong");
        reject();
      }

      let stored = new Map();
      map1.foreachLocalValue((key, value, _) => {
        stored.set(key, value);
      });
      map2.foreachLocalValue((key, value, _) => {
        stored.set(key, value);
      });

      if (stored.get("key1") !== "value1" || stored.get("key2") !== "value2") {
        logD("values are wrong");
        reject();
      }

      resolve();
    });

  }).then(() => {
    logD("node1.disconnect");
    return node1.disconnect();

  }).then(() => {
    logD("node2.disconnect");
    return node2.disconnect();

  }).then(() => {
    let result = document.getElementById("result");
    result.innerText = "SUCCESS";

  }).catch((e) => {
    let result = document.getElementById("result");
    result.innerText = "FAILURE";
    logE(e);
  });
}

test();
