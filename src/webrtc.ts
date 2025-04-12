/*
 * Copyright 2017- Yuji Ito <llamerada.jp@gmail.com>
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

interface WebRTCICEServerInfo {
  urls: string[];
  username: string;
  credential: string;
}

interface WebRTCLinkEventHandler {
  onUpdateLinkState(id: number, online: boolean): void;
  onUpdateICE(id: number, ice: string): void;
  onGetLocalSDP(id: number, sdp: string): void;
  onReceiveData(id: number, data: Uint8Array): void;
  onRaiseError(id: number, message: string): void;
}

interface WebRTCImplement {
  // for WebRTCConfig
  newConfig(config: Array<WebRTCICEServerInfo>): number;
  deleteConfig(id: number): void;
  // for WebRTCLink
  newLink(eventHandler: WebRTCLinkEventHandler, configId: number, createDataChannel: boolean, label: string): number;
  deleteLink(id: number): boolean;
  getLastError(id: number): string;
  getLabel(id: number): string;
  getLocalSDP(id: number): boolean;
  setRemoteSDP(id: number, sdp: string): boolean;
  updateICE(id: number, ice: string): boolean;
  send(id: number, data: Uint8Array): boolean;
}

class DefaultWebRTCImplement implements WebRTCImplement {
  linkEH: WebRTCLinkEventHandler;
  configs: Map<number, RTCConfiguration> = new Map();
  links: Map<number, {
    lastError: string,
    peer: RTCPeerConnection,
    dataChannel: RTCDataChannel | undefined,
    hasLocalSDP: boolean,
    hasRemoteSDP: boolean,
  }>;

  constructor(linkEH: WebRTCLinkEventHandler) {
    this.linkEH = linkEH;
    this.links = new Map();
  }

  newConfig(config: Array<WebRTCICEServerInfo>): number {
    let id = 0;
    while (this.configs.has(id)) {
      id++;
    }

    let rtcConfig = <RTCConfiguration>{};
    rtcConfig.iceServers = config.map((info) => {
      return {
        urls: info.urls,
        username: info.username,
        credential: info.credential,
      };
    });

    this.configs.set(id, rtcConfig);
    return id;
  }

  deleteConfig(id: number): void {
    this.configs.delete(id);
  }

  newLink(eventHandler: WebRTCLinkEventHandler, configId: number, isOffer: boolean, label:string): number {
    if (!this.configs.has(configId)) {
      console.error("Invalid config id.");
      return -1;
    }
    let config = this.configs.get(configId);

    let MAX_ID = 1024 ** 3;
    let id = Math.floor(Math.random() * MAX_ID);
    while (this.configs.has(id)) {
      id = Math.floor(Math.random() * MAX_ID);
    }

    let peer: RTCPeerConnection | undefined;
    try {
      peer = new RTCPeerConnection(config);
    } catch (e) {
      console.error(e);
      return -1;
    }

    let dataChannel: RTCDataChannel | undefined;
    if (isOffer) {
      dataChannel = peer.createDataChannel(label,
        <RTCDataChannelInit>{
          ordered: true,
          maxPacketLifeTime: 3000,
        });
      this.setDataChannelEvent(dataChannel, eventHandler, id);
    }

    this.links.set(id, {
      lastError: "",
      peer: peer,
      dataChannel: dataChannel,
      hasLocalSDP: false,
      hasRemoteSDP: false,
    });
    peer.onicecandidate = (event: RTCPeerConnectionIceEvent): void => {
      if (!this.links.has(id)) { return; }

      if (event.candidate) {
        eventHandler.onUpdateICE(id, JSON.stringify(event.candidate));
      }
    };

    peer.ondatachannel = (event: RTCDataChannelEvent): void => {
      let link = this.links.get(id);
      if (link == undefined) {
        return;
      }

      if (link.dataChannel != undefined) {
        eventHandler.onRaiseError(id, "duplicate data channel.");
      }

      link.dataChannel = event.channel;
      this.setDataChannelEvent(event.channel, eventHandler, id);
      if (link.dataChannel.readyState == "open") {
        eventHandler.onUpdateLinkState(id, true);
      }
    };

    peer.oniceconnectionstatechange = (_: Event): void => {
      let link = this.links.get(id);
      if (link == undefined) {
        return;
      }
    };

    return id;
  }

  setDataChannelEvent = (dataChannel: RTCDataChannel, eventHandler: WebRTCLinkEventHandler, id: number): void => {
    dataChannel.onerror = (e: Event): void => {
      if (!this.links.has(id)) { return; }
      let event = e as RTCErrorEvent;

      if (event.error.errorDetail === "sctp-failure" && event.error.sctpCauseCode == 12) {
        eventHandler.onUpdateLinkState(id, false);
      } else {
        eventHandler.onRaiseError(id, event.error.message);
      }
    };

    dataChannel.onmessage = (message: MessageEvent): void => {
      if (message.data instanceof ArrayBuffer) {
        if (!this.links.has(id)) { return; }
        eventHandler.onReceiveData(id, new Uint8Array(message.data));

      } else if (message.data instanceof Blob) {
        let reader = new FileReader();
        reader.onload = (): void => {
          if (!this.links.has(id)) { return; }
          eventHandler.onReceiveData(id, new Uint8Array(reader.result as ArrayBuffer));
        };
        reader.readAsArrayBuffer(message.data);

      } else {
        if (!this.links.has(id)) { return; }
        eventHandler.onRaiseError(id, "Unsupported type of message.");
      }
    };

    dataChannel.onopen = (_: Event): void => {
      if (!this.links.has(id)) { return; }
      eventHandler.onUpdateLinkState(id, true);
    };

    dataChannel.onclosing = (_: Event): void => {
      // This event could after deleteLink.
      // if (!this.links.has(id)) { return; }
      eventHandler.onUpdateLinkState(id, false);
    };

    dataChannel.onclose = (_: Event): void => {
      // This event could after deleteLink.
      // if (!this.links.has(id)) { return; }
      eventHandler.onUpdateLinkState(id, false);
    };
  };

  deleteLink(id: number): boolean {
    let link = this.links.get(id);
    if (link == undefined) {
      return true;
    }

    if (link.dataChannel != undefined) {
      link.dataChannel.close();
    }
    link.peer.close();
    this.links.delete(id);
    return true;
  }

  getLastError(id: number): string {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found when call getLastError.");
      return "";
    }
    return link.lastError;
  }

  getLabel(id: number): string {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found when call getDataChannelID.");
      return "";
    }

    if (link.dataChannel == undefined) {
      console.error("data channel not found when call getDataChannelID.");
      return "";
    }

    return link.dataChannel.label;
  }

  getLocalSDP(id: number): boolean {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found when call getLocalSDP.");
      return false;
    }

    try {
      let peer = link.peer;
      let description!: RTCSessionDescriptionInit;

      link.hasLocalSDP = true;

      if (link.hasRemoteSDP) {
        peer.createAnswer().then((sessionDescription): Promise<void> => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then((): void => {
          this.linkEH.onGetLocalSDP(id, description!.sdp!);

        }).catch((e): void => {
          link!.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
          this.linkEH.onGetLocalSDP(id, "");
        });

      } else {
        peer.createOffer().then((sessionDescription): Promise<void> => {
          description = sessionDescription;
          return peer.setLocalDescription(sessionDescription);

        }).then((): void => {
          this.linkEH.onGetLocalSDP(id, description!.sdp!);

        }).catch((e): void => {
          link!.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
          this.linkEH.onGetLocalSDP(id, "");
        });
      }

    } catch (e) {
      link.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
      return false;
    }

    return true;
  }

  setRemoteSDP(id: number, sdp: string): boolean {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found.");
      return false
    }

    try {
      link.hasRemoteSDP = true;

      let peer = link.peer;
      let sdpInit = <RTCSessionDescriptionInit>{
        type: (link.hasLocalSDP ? "answer" : "offer"),
        sdp: sdp,
      };
      peer.setRemoteDescription(new RTCSessionDescription(sdpInit));

    } catch (e) {
      link.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
      return false;
    }
    return true;
  }

  updateICE(id: number, iceStr: string): boolean {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found.");
      return false
    }

    try {
      let peer = link.peer;
      let ice = JSON.parse(iceStr);

      peer.addIceCandidate(new RTCIceCandidate(ice));

    } catch (e) {
      link.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
      return false;
    }

    return true;
  }

  send(id: number, data: Uint8Array): boolean {
    let link = this.links.get(id);
    if (link == undefined) {
      console.error("link not found.");
      return false;
    }

    try {
      link.dataChannel?.send(data);

    } catch (e) {
      link.lastError = JSON.stringify(e, Object.getOwnPropertyNames(e));
      return false;
    }

    return true;
  }
}

class WebRTCWrapper implements WebRTCLinkEventHandler {
  impl?: WebRTCImplement;

  setup() {
    if (this.impl === undefined) {
      this.impl = new DefaultWebRTCImplement(this);
    }
  }

  setImplement(iF: WebRTCImplement) {
    this.impl = iF;
  }

  newConfig(context: string): number {
    let c = JSON.parse(context) as Array<WebRTCICEServerInfo>;
    return this.impl!.newConfig(c);
  }

  deleteConfig(context: number): void {
    this.impl!.deleteConfig(context);
  }

  onUpdateLinkState(id: number, online: boolean): void {
    console.error("WebRTCWrapper::onUpdateLinkState method must by override by go module");
  }

  onUpdateICE(id: number, ice: string): void {
    console.error("WebRTCWrapper::onUpdateICE method must by override by go module");
  }

  onGetLocalSDP(id: number, sdp: string): void {
    console.error("WebRTCWrapper::onGetLocalSDP method must by override by go module");
  }

  onReceiveData(id: number, data: Uint8Array): void {
    console.error("WebRTCWrapper::onReceiveData method must by override by go module");
  }

  onRaiseError(id: number, message: string): void {
    console.error("WebRTCWrapper::onRaiseError method must by override by go module");
  }

  newLink(configId: number, createDataChannel: boolean, label:string): number {
    return this.impl!.newLink(this, configId, createDataChannel, label);
  }

  deleteLink(id: number): boolean {
    return this.impl!.deleteLink(id);
  }

  getLastError(id: number): string {
    return this.impl!.getLastError(id);
  }

  getLabel(id: number): string {
    return this.impl!.getLabel(id);
  }

  getLocalSDP(id: number): boolean {
    return this.impl!.getLocalSDP(id);
  }

  setRemoteSDP(id: number, sdp: string): boolean {
    return this.impl!.setRemoteSDP(id, sdp);
  }

  updateICE(id: number, ice: string): boolean {
    return this.impl!.updateICE(id, ice);
  }

  send(id: number, data: Uint8Array): boolean {
    return this.impl!.send(id, data);
  }
}

(globalThis as any).webrtcWrapper = new WebRTCWrapper();