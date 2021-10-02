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
  let timer;

  return ColonioModule().then((colonio) => {
    logD("new node1");
    node1 = new colonio.Colonio();
    node1.on("log", (l) => {
      l.node = "node1";
      if (l.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        l.level === colonio.Colonio.LOG_LEVEL_WARN) {
        logE(l);
      } else {
        logD(l);
      }
    })

    logD("new node2");
    node2 = new colonio.Colonio();
    node2.on("log", (l) => {
      l.node = "node2";
      if (l.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        l.level === colonio.Colonio.LOG_LEVEL_WARN) {
        logE(l);
      } else {
        logD(l);
      }
    });

    logD("node1.connect");
    return node1.connect(URL, TOKEN);

  }).then(() => {
    logD("node2.connect");
    return node2.connect(URL, TOKEN);

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

    timer = setInterval(() => {
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
        logD("receive ps2", data);
        if (data === "from ps1") {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    clearInterval(timer);

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
