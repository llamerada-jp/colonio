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

    newColonio() {
        return new ColonioWrap(this);
    }

    newValue(type, value) {
        let V = this.colonio.Value;

        switch (type) {
            case V.VALUE_TYPE_NULL:
                return V.Null();

            case V.VALUE_TYPE_BOOL:
                return V.Bool(value);

            case V.VALUE_TYPE_INT:
                return V.Int(value);

            case V.VALUE_TYPE_DOUBLE:
                return V.Double(value);

            case V.VALUE_TYPE_STRING:
                return V.String(value);

            default:
                logE("unsupported value type", type, value);
                return V.Null();
        }
    }

    // this method will be override by golang
    onEvent(key, resp) {
        logE("onEvent method must by override by golang");
    }

    // this method will be override by golang
    onResponse(key, resp) {
        logE("onResponse method must by override by golang");
    }
}

class ColonioWrap {
    constructor(suite) {
        this.suite = suite;
        this.c = new this.suite.colonio.Colonio();
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
}

class ColonioMapWrap {
    constructor(suite, m) {
        this.suite = suite;
        this.m = m;
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