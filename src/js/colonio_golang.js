//@ts-check
"use strict";

// eslint-disable-next-line no-console
const logD = console.log;
// eslint-disable-next-line no-console
const logE = console.error;

function convertError(err) {
    return err;
}

class ColonioSuite {
    constructor(colonio) {
        this.colonio = colonio;
    }

    newColonio(logger) {
        return new ColonioWrap(this, logger);
    }

    newValue(type, value) {
        return new this.colonio.Value(type, value);
    }

    // this method will be override by golang
    onEvent() {
        logE("onEvent method must by override by golang");
    }

    // this method will be override by golang
    onResponse() {
        logE("onResponse method must by override by golang");
    }

    outputDefaultLog(message) {
        logD(JSON.parse(message));
    }
}

class ColonioWrap {
    constructor(suite, logger) {
        this.suite = suite;
        this.c = new this.suite.colonio.Colonio(logger);
    }

    connect(key, url, token) {
        this.c.connect(url, token).then(() => {
            this.suite.onResponse(key);
        }, (err) => {
            this.suite.onResponse(key, convertError(err));
        });
    }

    disconnect(key) {
        this.c.disconnect().then(() => {
            this.suite.onResponse(key);
        }, (err) => {
            this.suite.onResponse(key, convertError(err));
        });
    }

    isConnected() {
        return this.c.isConnected();
    }

    accessMap(name) {
        return new ColonioMapWrap(this.suite, this.c.accessMap(name));
    }

    accessPubsub2D(name) {
        return new ColonioPubusb2DWrap(this.suite, this.c.accessPubsub2D(name));
    }

    getLocalNid() {
        return this.c.getLocalNid();
    }

    setPosition(key, x, y) {
        this.c.setPosition(x, y).then((pos) => {
            this.suite.onResponse(key, {
                x: pos.x,
                y: pos.y,
                err: null
            });
        }, (err) => {
            this.suite.onResponse(key, {
                x: 0,
                y: 0,
                err: convertError(err)
            });
        });
    }

    callByNid(key, dst, name, valueType, valueValue, opt) {
        this.c.callByNid(dst, name, this.suite.newValue(valueType, valueValue), opt).then((value) => {
            this.suite.onResponse(key, {
                type: value.getType(),
                value: value.getJsValue()
            });
        }, (err) => {
            this.suite.onResponse(key, {
                err: convertError(err)
            });
        });
    }

    onCall(name, key) {
        this.c.onCall(name, (parameter) => {
            let result = this.suite.onEvent(key, {
                name: parameter.name,
                valueType: parameter.value.getType(),
                value: parameter.value.getJsValue(),
                options: parameter.options
            });
            return this.suite.newValue(result.valueType, result.value);
        });
    }

    offCall(name) {
        this.c.offCall(name);
    }
}

class ColonioMapWrap {
    constructor(suite, m) {
        this.suite = suite;
        this.m = m;
    }

    foreachLocalValue(key) {
        return this.m.foreachLocalValueRaw((rKey, rValue, attr) => {
            this.suite.onEvent(key, {
                keyType: rKey.getType(),
                keyValue: rKey.getJsValue(),
                valType: rValue.getType(),
                valValue: rValue.getJsValue(),
                attr: attr
            });
        });
    }

    get(key, keyType, keyValue) {
        this.m.getRawValue(this.suite.newValue(keyType, keyValue)).then((value) => {
            this.suite.onResponse(key, {
                type: value.getType(),
                value: value.getJsValue(),
                err: null
            });
        }, (err) => {
            this.suite.onResponse(key, {
                value: null,
                err: convertError(err)
            });
        });
    }

    set(key, keyType, keyValue, valueType, valueValue, opt) {
        this.m.setValue(this.suite.newValue(keyType, keyValue),
            this.suite.newValue(valueType, valueValue), opt).then(() => {
                this.suite.onResponse(key);
            }, (err) => {
                this.suite.onResponse(key, convertError(err));
            });
    }
}

class ColonioPubusb2DWrap {
    constructor(suite, p) {
        this.suite = suite;
        this.p = p;
    }

    publish(key, name, x, y, r, valueType, valueValue, opt) {
        this.p.publish(name, x, y, r, this.suite.newValue(valueType, valueValue), opt).then(() => {
            this.suite.onResponse(key);
        }, (err) => {
            this.suite.onResponse(key, convertError(err));
        });
    }

    on(key, name) {
        this.p.onRaw(name, (data) => {
            this.suite.onEvent(key, {
                type: data.getType(),
                value: data.getJsValue()
            });
        });
    }

    off(name) {
        this.p.off(name);
    }
}