#!/usr/bin/env python
# -*- coding: utf-8 -*-
import colonio

SERVER_URL  = "http://localdev:8080/colonio/core.json"
NODE_NUM    = 100

nodes_tmp       = set()
nodes_online    = dict()

def onSuccess(v):
    print 'onSuccess : ' + v.getLocalNid().toStr()
    nodes_online[v.getLocalNid()] = {
        'instance': v
    }
    nodes_tmp.remove(v)

def onFailure(v):
    print 'onFailure'
    nodes_tmp.remove(v)

while True:
    if len(nodes_tmp) + len(nodes_online) < NODE_NUM:
        c = colonio.Colonio()
        nodes_tmp.add(v)
        c.connect(SERVER_URL, "", onSuccess, onFailure)
    print 'run'
    colonio.Colonio.run(colonio.Colonio.RunMode.NOWAIT)
