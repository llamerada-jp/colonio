'use strict';

const URL = 'ws://localhost:8080/test';
const TOKEN = '';
const KEY = 'hoge';

function test() {
  let node1;
  let node2;
  let ps1;
  let ps2;
  let timer;

  return ColonioModule().then(colonio => {
    console.log('new node1');
    node1 = new colonio.Colonio();
    node1.on('log', (l) => {
      l.node = 'node1';
      if (l.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        l.level === colonio.Colonio.LOG_LEVEL_WARN) {
        console.error(l);
      } else {
        console.log(l);
      }
    })

    console.log('new node2');
    node2 = new colonio.Colonio();
    node2.on('log', (l) => {
      l.node = 'node2';
      if (l.level === colonio.Colonio.LOG_LEVEL_ERROR ||
        l.level === colonio.Colonio.LOG_LEVEL_WARN) {
        console.error(l);
      } else {
        console.log(l);
      }
    })

    console.log('node1.connect');
    return node1.connect(URL, TOKEN);

  }).then(() => {
    console.log('node2.connect');
    return node2.connect(URL, TOKEN);

  }).then(() => {
    console.log('get accessor');
    ps1 = node1.accessPubsub2D('ps');
    node1.setPosition(0, 0);

    ps2 = node2.accessPubsub2D('ps');
    node2.setPosition(0, 0);

    timer = setInterval(() => {
      console.log('publish data');
      ps1.publish(KEY, 0, 0, 1.0, 'from ps1');
      ps2.publish(KEY, 0, 0, 1.0, 'from ps2');
    }, 1000);

    return new Promise((resolve, reject) => {
      console.log('ps1 waiting data');
      ps1.on(KEY, (data) => {
        console.log('receive ps1', data);
        if (data === 'from ps2') {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    return new Promise((resolve, reject) => {
      console.log('ps2 waiting data');
      ps2.on(KEY, (data) => {
        console.log('receive ps2', data);
        if (data === 'from ps1') {
          resolve();
        } else {
          reject();
        }
      });
    });

  }).then(() => {
    clearInterval(timer);

    console.log('node1.disconnect');
    return node1.disconnect();

  }).then(() => {
    console.log('node2.disconnect');
    return node2.disconnect();

  }).then(() => {
    let result = document.getElementById('result');
    result.innerText = "SUCCESS";

  }).catch((e) => {
    let result = document.getElementById('result');
    result.innerText = "FAILURE";
    console.log(e);
  });
}

test();
