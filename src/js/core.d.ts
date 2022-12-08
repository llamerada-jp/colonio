declare var HEAP8: Int8Array;
declare var HEAPU8: Uint8Array;
declare function _malloc(size: number): number;
declare function _free(ptr: number): void;
declare function allocPtr(siz: number): number;
declare function allocPtrString(str: string): [number, number];
declare function allocPtrArrayBuffer(buffer: ArrayBuffer): [number, number];
declare function freePtr(ptr: number): void;
declare function convertError(ptr: number): ErrorEntry | undefined;
declare let funcsAfterLoad: Array<() => void> | null;
declare function addFuncAfterLoad(func: () => void): void;
declare function execFuncsAfterLoad(): void;
declare const ID_MAX: number;
declare function idMapPush<T>(map: Map<number, T>, v: T): number;
declare function idMapPop<T>(map: Map<number, T>, id: number): T | undefined;
declare function idMapGet<T>(map: Map<number, T>, id: number): T | undefined;
declare class LogLevel {
    static readonly ERROR: string;
    static readonly WARN: string;
    static readonly INFO: string;
    static readonly DEBUG: string;
}
declare class LogEntry {
    file: string;
    level: string;
    line: number;
    message: string;
    param: object;
    time: string;
    constructor(f: string, lv: string, l: number, m: string, p: object, t: string);
}
declare class ErrorCode {
    static readonly UNDEFINED: number;
    static readonly SYSTEM_INCORRECT_DATA_FORMAT: number;
    static readonly SYSTEM_CONFLICT_WITH_SETTING: number;
    static readonly CONNECTION_FAILED: number;
    static readonly CONNECTION_OFFLINE: number;
    static readonly PACKET_NO_ONE_RECV: number;
    static readonly PACKET_TIMEOUT: number;
    static readonly MESSAGING_HANDLER_NOT_FOUND: number;
    static readonly KVS_NOT_FOUND: number;
    static readonly KVS_PROHIBIT_OVERWRITE: number;
    static readonly KVS_COLLISION: number;
    static readonly SPREAD_NO_ONE_RECEIVE: number;
}
declare class ErrorEntry {
    fatal: boolean;
    code: number;
    message: string;
    line: number;
    file: string;
    constructor(f: boolean, c: number, m: string, l: number, fi: string);
}
type ValueSource = null | boolean | number | string | Value;
/**
 * Value is wrap for Value class.
 */
declare class Value {
    static readonly VALUE_TYPE_NULL: number;
    static readonly VALUE_TYPE_BOOL: number;
    static readonly VALUE_TYPE_INT: number;
    static readonly VALUE_TYPE_DOUBLE: number;
    static readonly VALUE_TYPE_STRING: number;
    static readonly VALUE_TYPE_BINARY: number;
    _type: number;
    _value: null | boolean | number | string;
    static newNull(): Value;
    static newBool(value: boolean): Value;
    static newInt(value: number): Value;
    static newDouble(value: number): Value;
    static newString(value: string): Value;
    static fromJsValue(value: ValueSource): Value | undefined;
    static fromCValue(valueC: number): Value;
    static free(valueC: number): void;
    constructor(type: number, value: null | boolean | number | string);
    getType(): number;
    getJsValue(): string | number | boolean | null;
    write(valueC?: number): number;
}
declare class ColonioConfig {
    loggerFuncRaw: (c: Colonio, json: string) => void;
    loggerFunc: (c: Colonio, log: LogEntry) => void;
    constructor();
}
declare enum FuncID {
    LOGGER = 0,
    MESSAGING_POST_ON_SUCCESS = 1,
    MESSAGING_POST_ON_FAILURE = 2,
    MESSAGING_HANDLER = 3,
    KVS_GET_ON_SUCCESS = 4,
    KVS_GET_ON_FAILURE = 5,
    KVS_SET_ON_SUCCESS = 6,
    KVS_SET_ON_FAILURE = 7,
    KVS_LOCAL_DATA_HANDLER = 8,
    SPREAD_POST_ON_SUCCESS = 9,
    SPREAD_POST_ON_FAILURE = 10,
    SPREAD_HANDLER = 11
}
declare const MESSAGING_ACCEPT_NEARBY: number;
declare const MESSAGING_IGNORE_RESPONSE: number;
declare const SPREAD_SOMEONE_MUST_RECEIVE: number;
declare let loggerMap: Map<number, (json: string) => void>;
declare let colonioMap: Map<number, Colonio>;
declare class MessagingRequest {
    sourceNid: string;
    message: Value;
    options: number;
    constructor(s: string, m: Value, o: number);
}
declare class MessagingResponseWriter {
    _colonio: Colonio;
    _writer: number;
    constructor(c: Colonio, w: number);
    write(v: ValueSource): void;
}
declare class KvsLocalData {
    _parent: Colonio;
    _cur: number;
    _keys: Array<string>;
    constructor(p: Colonio, c: number, k: Array<string>);
    getKeys(): Array<string>;
    getValue(key: string): Value | undefined;
    free(): void;
}
declare class SpreadRequest {
    sourceNid: string;
    message: Value;
    options: number;
    constructor(s: string, m: Value, o: number);
}
declare class Colonio {
    _colonio: number;
    _idVsPromiseValue: Map<number, {
        onSuccess: (value: Value) => void;
        onFailure: (error: ErrorEntry | undefined) => void | undefined;
    }>;
    _idVsPromise: Map<number, {
        onSuccess: (_: null) => void;
        onFailure: (error: ErrorEntry | undefined) => void | undefined;
    }>;
    _idVsMessagingHandler: Map<number, (request: MessagingRequest, writer?: MessagingResponseWriter) => void>;
    _idVsKvsLocalDataHandler: Map<number, (kld: KvsLocalData) => void>;
    _refKvsLocalData: Map<number, WeakRef<KvsLocalData>>;
    _idVsSpreadHandler: Map<number, (request: SpreadRequest) => void>;
    constructor(config: ColonioConfig);
    connect(url: string, token: string): Promise<void>;
    disconnect(): Promise<void>;
    isConnected(): boolean;
    getLocalNid(): string;
    setPosition(x: number, y: number): ErrorEntry | {
        x: number;
        y: number;
    };
    quit(): void;
    messagingPost(dst: string, name: string, val: ValueSource, opt: number): Promise<Value>;
    messagingSetHandler(name: string, handler: (request: MessagingRequest, writer?: MessagingResponseWriter) => void): void;
    messagingUnsetHandler(name: string): void;
    kvsGetLocalData(): Promise<KvsLocalData>;
    kvsGet(key: string): Promise<Value>;
    kvsSet(key: string, val: ValueSource, opt: number): Promise<null>;
    spreadPost(x: number, y: number, r: number, name: string, val: ValueSource, opt: number): Promise<null>;
    spreadSetHandler(name: string, handler: (request: SpreadRequest) => void): void;
    spreadUnsetHandler(name: string): void;
    _cleanup(): void;
}
declare let schedulerTimers: Map<number, number>;
declare function schedulerRelease(schedulerPtr: number): void;
declare function schedulerRequestNextRoutine(schedulerPtr: number, msec: number): void;
declare let availableSeedLinks: Map<number, WebSocket>;
declare function seedLinkWsConnect(seedLink: number, urlPtr: number, urlSiz: number): void;
declare function seedLinkWsSend(seedLink: number, dataPtr: number, dataSiz: number): void;
declare function seedLinkWsDisconnect(seedLink: number): void;
declare function seedLinkWsFinalize(seedLink: number): void;
declare class WebrtcLinkCb {
    onDcoError(webrtcLink: number, message: string): void;
    onDcoMessage(webrtcLink: number, message: ArrayBuffer): void;
    onDcoOpen(webrtcLink: number): void;
    onDcoClosing(webrtcLink: number): void;
    onDcoClose(webrtcLink: number): void;
    onPcoError(webrtcLink: number, message: string): void;
    onPcoIceCandidate(webrtcLink: number, iceStr: string): void;
    onPcoStateChange(webrtcLink: number, state: string): void;
    onCsdSuccess(webrtcLink: number, sdpStr: string): void;
    onCsdFailure(webrtcLink: number): void;
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
declare class DefaultWebrtcImplement implements WebrtcImplement {
    cb?: WebrtcLinkCb;
    webrtcContextPcConfig: RTCConfiguration;
    webrtcContextDcConfig: RTCDataChannelInit;
    availableWebrtcLinks: Map<number, {
        peer: RTCPeerConnection;
        dataChannel: RTCDataChannel | undefined;
    }>;
    constructor();
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
/**
 * Interface for WebRTC.
 * It is used when use colonio in WebWorker. Currently, WebRTC is not useable in WebWorker,
 * so developer should by-pass WebRTC methods to main worker.
 */
declare let webrtcImpl: WebrtcImplement | undefined;
declare function setWebRTCImpl(w: WebrtcImplement): void;
declare function webrtcContextInitialize(): void;
declare function webrtcContextAddIceServer(strPtr: number, strSiz: number): void;
declare function webrtcLinkInitialize(webrtcLink: number, isCreateDc: boolean): void;
declare function webrtcLinkFinalize(webrtcLink: number): void;
declare function webrtcLinkDisconnect(webrtcLink: number): void;
declare function webrtcLinkGetLocalSdp(webrtcLink: number, isRemoteSdpSet: boolean): void;
declare function webrtcLinkSend(webrtcLink: number, dataPtr: number, dataSiz: number): void;
declare function webrtcLinkSetRemoteSdp(webrtcLink: number, sdpPtr: number, sdpSiz: number, isOffer: boolean): void;
declare function webrtcLinkUpdateIce(webrtcLink: number, icePtr: number, iceSiz: number): void;
declare function utilsGetRandomSeed(): number;
