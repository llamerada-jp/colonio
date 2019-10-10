import sys
import os
sys.path.append('/Library/Python/2.7/site-packages/')
sys.path.append('/usr/local/lib/python2.7/site-packages/')
sys.path.append('/usr/lib/python2.7/dist-packages/')

import atexit
import json
import math
import random
import redis
import subprocess
from threading  import Thread
import traceback
from Queue import Queue, Empty

WINDOW_WIDTH = 800
NODE_CIRCLE_R = 300
NODES_NUM = 30
MAP_CIRCLE_R  = 350
NODE_RECT_W = 10
VALUE_CIRCLE_R = 350
COMMAND = '/Users/llamerada/develop/colonio/build/vscode/sample/simulate'
RANDOM_KILL_RATE = 0
# COMMAND = ['valgrind', '--leak-check=full', COMMAND]

COLOR_BLACK     = (  0,   0,   0)
COLOR_BLUE      = (  0,   0, 255)
COLOR_GREEN     = (  0, 255,   0)
COLOR_CYAN      = (  0, 255, 255)
COLOR_RED       = (255,   0,   0)
COLOR_MAGENTA   = (255,   0, 255)
COLOR_YELLOW    = (255, 127,   0)
COLOR_WHITE     = (255, 255, 255)

class NodeInfo:
    def __init__(self, proc, pid, nid):
        self.proc   = proc
        self.pid    = pid
        self.nid    = nid
        self.map_set    = []
        self.links  = []
        self.nexts  = []
        self.position   = None
        self.required1d = []
        self.required2d = {}
        self.known1d    = []
        self.known2d    = {}

# NID vs NodeInfo
node_infos = {}
# PID vs proc
node_inits = {}
queue = Queue()
#
selected_nid = None
#
create_count = 0

def setup():
    global db
    global log_file
    size(WINDOW_WIDTH * 2, WINDOW_WIDTH)
    colorMode(RGB, 256)
    db = redis.Redis(host='localdev', port=6379, db=0)
    log_file = open('/tmp/simulate.log', 'w')

def draw():
    try:
        checkNodeTerminate()
        updateNode()
        updateInits()
        updateInfos()

        background(255)
        drawProperties()
        drawLinks1D()
        drawLinks2D()
        drawNodes1D()
        drawNodes2D()
        drawMaps()
        if selected_nid != None:
            drawSelectedProp()
    except:
        outputLog(traceback.format_exc())
        exit()

def mouseClicked():
    min_d2 = 10000
    is_matched = False
    global selected_nid

    for nid, info in node_infos.items():
        pos = getNodePos1D(nid)
        d2 = (mouseX - pos[0]) ** 2 + (mouseY - pos[1]) ** 2
        if d2 < 100 and d2 < min_d2:
            min_d2 = d2
            selected_nid = nid
            is_matched = True
        if info.position:
            pos = getNodePos2D(info.position)
            d2 = (mouseX - pos[0]) ** 2 + (mouseY - pos[1]) ** 2
            if d2 < 100 and d2 < min_d2:
                min_d2 = d2
                selected_nid = nid
                is_matched = True
    if not is_matched:
        selected_nid = None

def drawProperties():
    stroke(0, 0, 0, 127)
    fill(0, 0, 0, 127)
    rect(0, 0, 200, 72)
    fill(255, 255, 255)
    text('nodes : %d  inits : %d' % (len(node_infos), len(node_inits)), 10, 16)

def drawSelectedProp():
    stroke(0, 0, 0, 127)
    fill(0, 0, 0, 127)
    rect(WINDOW_WIDTH - 320, 0, 320, WINDOW_WIDTH)
    fill(255, 255, 255)
    text(selected_nid, WINDOW_WIDTH - 320 + 10, 16)
    if selected_nid in node_infos:
        info = node_infos[selected_nid]
        text('%d' % info.pid, WINDOW_WIDTH - 320 + 10, 32)
        for i in range(len(info.known1d)):
            nid = info.known1d[i]
            if nid in info.links:
                nid_text = '%s*' % nid
            else:
                nid_text = nid
            if nid in node_infos:
                fill(255, 255, 255)
            else:
                fill(255, 0, 0)
            text(nid_text, WINDOW_WIDTH - 320 + 10, 48 + 16 * i)
    
def drawNodes1D():
    if selected_nid == None or not selected_nid in node_infos:
        for nid, info in node_infos.items():
            drawNode1D(nid, COLOR_BLACK, COLOR_WHITE)
    else:
        s_info = node_infos[selected_nid]
        for nid, info in node_infos.items():
            if selected_nid == nid:
                drawNode1D(nid, COLOR_RED, COLOR_RED)
            else:
                if nid in s_info.nexts:
                    color = COLOR_BLUE
                elif nid in s_info.required1d:
                    color = COLOR_GREEN
                elif nid in s_info.known1d:
                    color = COLOR_YELLOW
                else:
                    color = COLOR_BLACK
                if nid in s_info.links:
                    color_fill = color
                else:
                    color_fill = COLOR_WHITE
                drawNode1D(nid, color, color_fill)

def drawNodes2D():
    if selected_nid == None or not selected_nid in node_infos:
        for nid, info in node_infos.items():
            if info.position != None:
                drawNode2D(info.position, COLOR_BLACK, COLOR_WHITE)
    else:
        s_info = node_infos[selected_nid]
        for nid, info in node_infos.items():
            if info.position == None:
                continue
            if selected_nid == nid:
                drawNode2D(info.position, COLOR_RED, COLOR_RED)
            else:
                if nid in s_info.required2d:
                    color = COLOR_GREEN
                    drawOffset2D(nid, s_info.required2d[nid], COLOR_MAGENTA)
                elif nid in s_info.known2d:
                    color = COLOR_YELLOW
                    drawOffset2D(nid, s_info.known2d[nid], COLOR_CYAN)
                else:
                    color = COLOR_BLACK
                if nid in s_info.links:
                    color_fill = color
                else:
                    color_fill = COLOR_WHITE
                drawNode2D(info.position, color, color_fill)

def drawNode1D(nid, st, fi):
    pos = getNodePos1D(nid)
    stroke(*st)
    fill(*fi)
    rect(pos[0] - NODE_RECT_W / 2, pos[1] - NODE_RECT_W / 2, NODE_RECT_W, NODE_RECT_W)

def drawNode2D(position, st, fi):
    pos = getNodePos2D(position)
    stroke(*st)
    fill(*fi)
    rect(pos[0] - NODE_RECT_W / 2, pos[1] - NODE_RECT_W / 2, NODE_RECT_W, NODE_RECT_W)

def drawOffset2D(nid, position, color):
    if nid in node_infos:
        info = node_infos[nid]
        if info.position[0] != position[0] or info.position[1] != position[1]:
            a = getNodePos2D(position)
            b = getNodePos2D(info.position)
            stroke(*color)
            fill(*color)
            ellipse(a[0], a[1], 5, 5)
            line(a[0], a[1], b[0], b[1])
    else:
        pos = getNodePos2D(position)
        stroke(*color)
        fill(COLOR_WHITE)
        ellipse(pos[0], pos[1], 5, 5)

def drawMap(hash, r, g, b):
    pos = getMapPos(hash)
    stroke(r, g, b)
    fill(r, g, b)
    rect(pos[0] - NODE_RECT_W / 2, pos[1] - NODE_RECT_W / 2, NODE_RECT_W, NODE_RECT_W)

def drawMaps():
    for nid, info in node_infos.items():
        for map in info.map_set:
            a = getNodePos1D(nid)
            b = getMapPos(map['hash'])
            drawMap(map['hash'], 0, 255, 0)
            stroke(0, 0, 0)
            line(a[0], a[1], b[0], b[1])

def drawLinks1D():
    for nid, info in node_infos.items():
        for pair in info.links:
            a = getNodePos1D(nid)
            b = getNodePos1D(pair)
            if pair in node_infos and nid in node_infos[pair].links:
                stroke(0, 0, 0)
                line(a[0], a[1], b[0], b[1])
            else:
                stroke(255, 0, 0)
                line(a[0], a[1], (a[0] + b[0]) / 2, (a[1] + b[1]) / 2)

def drawLinks2D():
    for nid, info in node_infos.items():
        for pair in info.links:
            if pair in node_infos and info.position != None and node_infos[pair].position != None:
                a = getNodePos2D(info.position)
                b = getNodePos2D(node_infos[pair].position)
                if nid in node_infos[pair].links:
                    stroke(0, 0, 0)
                    line(a[0], a[1], b[0], b[1])
                else:
                    stroke(255, 0, 0)
                    line(a[0], a[1], (a[0] + b[0]) / 2, (a[1] + b[1]) / 2)

def getMapPos(hash):
    rad = (0.5 - (2.0 * int(hash[0:8], 16) / 0xFFFFFFFF)) * math.pi
    return (
        WINDOW_WIDTH / 2 + cos(rad) * MAP_CIRCLE_R,
        WINDOW_WIDTH / 2 - sin(rad) * MAP_CIRCLE_R)

def getNodePos1D(nid):
    rad = (0.5 - (2.0 * int(nid[0:8], 16) / 0xFFFFFFFF)) * math.pi
    return (
        WINDOW_WIDTH / 2 + cos(rad) * NODE_CIRCLE_R,
        WINDOW_WIDTH / 2 - sin(rad) * NODE_CIRCLE_R)

def getNodePos2D(info):
    return (
        WINDOW_WIDTH + WINDOW_WIDTH * (info[0] + math.pi    ) / (2 * math.pi),
        0            + WINDOW_WIDTH * (info[1] + math.pi / 2) / (2 * math.pi))

def updateNode():
    if len(node_infos) + len(node_inits) < NODES_NUM and len(node_inits) < 10:
        createNode()
        randomKill()

def updateInits():
    inits = db.hgetall('inits')
    for pid, nid in inits.items():
        pid = int(pid)
        if pid in node_inits:
            node_infos[nid] = NodeInfo(node_inits[pid], pid, nid)
            del node_inits[pid]
        db.hdel('inits', pid)

def updateInfos():
    for key in ['map_set', 'links', 'nexts', 'position', 'required1d', 'required2d', 'known1d', 'known2d']:
        data = db.hgetall(key)
        for nid, json_str in data.items():
            if nid in node_infos:
                setattr(node_infos[nid], key, json.loads(json_str))
            db.hdel(key, nid)

def outputLog(str):
    # sys.stdout.write(str)
    log_file.write(str)

def outputNode(proc):
    global queue
    while True:
        line = proc.stdout.readline()
        if line:
            outputLog(str(proc.pid) + ":" + line)
        else:
            break
    
def createNode():
    global create_count
    proc = subprocess.Popen(COMMAND,
                            stdout = subprocess.PIPE,
                            stderr = subprocess.STDOUT)
    node_inits[proc.pid] = (proc)
    t = Thread(target=outputNode, args=(proc,))
    t.daemon = True
    t.start()
    create_count += 1
    outputLog("create proc %d\n" % proc.pid)

def checkNodeTerminate():
    for pid in node_inits.keys():
        proc = node_inits[pid]
        if proc.poll():
            del node_inits[pid]
            outputLog("terminate inits %d(%d)\n" % (pid, proc.returncode))
    for nid in node_infos.keys():
        proc = node_infos[nid].proc
        if proc.poll():
            del node_infos[nid]
            outputLog("terminate nodes %d(%d)\n" % (proc.pid, proc.returncode))

def randomKill():
    if RANDOM_KILL_RATE == 0:
        return
    if len(node_infos) > RANDOM_KILL_RATE and create_count % RANDOM_KILL_RATE == 0:
        info = node_infos[random.choice(node_infos.keys())]
        outputLog("kill %d\n" % info.proc.pid)
        info.proc.kill()

@atexit.register
def killAll():
    for pid in node_inits.keys():
        proc = node_inits[pid]
        proc.terminate()
    for nid in node_infos.keys():
        proc = node_infos[nid].proc
        proc.terminate()
    log_file.close()
