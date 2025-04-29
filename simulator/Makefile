SHELL := /bin/bash -o pipefail

TAG := ghcr.io/llamerada-jp/colonio-simulator
STORY := circle
# node pod count for random simulation
NODES := 5
# areas for geo simulation
AREAS := 32
DUMP_FILE := dump.json
RENDER_FILE := out.mp4
RENDER_DIR := render

# command
KUBECTL := sudo k3s kubectl
export KUBECTL
SIMULATOR := work/simulator

# set font
FONT_PATH=/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf
export FONT_PATH

.PHONY: simulate-random
simulate-random:
	$(MAKE) apply-base
	$(KUBECTL) apply -k deploy/node/random/
	$(KUBECTL) scale deployment -n colonio-simulator node --replicas=$(NODES)

.PHONY: simulate-geo
simulate-geo:
	$(MAKE) apply-base
	go run ./cmd/ generate -a $(AREAS) -v deploy/node/geo/values.yaml | $(KUBECTL) apply -f -

# When used with the delay target, it generates a location-based delay between the nodes.
# For delay, useing the tc command. To run in a wsl environment, rebuild the kernel
# with reference to the following document.
#
# sudo apt install build-essential flex bison libssl-dev libelf-dev dwarves libncurses-dev
# git clone --depth=1 -b linux-msft-wsl-5.15.y  https://github.com/microsoft/WSL2-Linux-Kernel.git
# cd WSL2-Linux-Kernel.git
# cp Microsoft/config-wsl .config
# make menuconfig
# # enable all options of Queueing/Scheduling and Classification
# cf. https://github.com/microsoft/WSL/issues/8584#issuecomment-1507828323
# make
# cp vmlinux /mnt/c/Users/<username>/
# make modules_install
# vi /mnt/c/Users/<username>/.wslconfig
# # [wsl2]
# # kernel=C:\\Users\\<username>\\vmlinux
# (powershell) wsl --shutdown
.PHONY: delay
delay:
	go run ./cmd/ delay -d ./material

.PHONY: apply-base
apply-base: colonio-simulator.tar
	sudo k3s ctr images import colonio-simulator.tar
	$(MAKE) stop
	$(KUBECTL) apply -k deploy/overlays/$(STORY)

.PHONY: stop
stop:
	$(KUBECTL) delete namespace colonio-simulator || true

.PHONY: export
export: $(SIMULATOR)
	$(SIMULATOR) export --mongodb-uri mongodb://localhost:30017 --output $(DUMP_FILE)

.PHONY: render
render: $(SIMULATOR)
	rm -rf $(RENDER_DIR)
	mkdir -p $(RENDER_DIR)
	$(SIMULATOR) render --story $(STORY) --input $(DUMP_FILE) --output-prefix $(RENDER_DIR)/image-
	ffmpeg -y -r 10 -i $(RENDER_DIR)/image-%06d.png -vcodec libx264 -pix_fmt yuv420p -r 10 $(RENDER_FILE)

.PHONY: view
view: $(SIMULATOR)
	$(SIMULATOR) render --story $(STORY) --input $(DUMP_FILE)

colonio-simulator.tar: Dockerfile $(SIMULATOR)
	openssl req -x509 -out work/seed.crt -keyout work/seed.key \
    -newkey rsa:2048 -nodes -sha256 \
    -subj '/CN=seed.colonio-simulator.svc.cluster.local' -extensions EXT -config <( \
    printf "[dn]\nCN=seed.colonio-simulator.svc.cluster.local\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:seed.colonio-simulator.svc.cluster.local\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")
	docker build . -t $(TAG)
	docker save $(TAG) -o $@

$(SIMULATOR): $(shell find ../ -type f -name '*.go')
	$(MAKE) -C ..
	mkdir -p work
	go build -o $@ ./cmd/

.PHONY: setup
setup:
	sudo apt install -y pkgconf ffmpeg fonts-noto libsdl2{,-image,-ttf}-dev

.PHONY: download
download:
	mkdir -p material
# cf. https://wonderproxy.com/blog/a-day-in-the-life-of-the-internet/
	curl -L https://wp-public.s3.amazonaws.com/pings/pings-2020-07-19-2020-07-20.csv.gz -o material/pings.csv.gz
	gzip -d material/pings.csv.gz
	curl -L https://wp-public.s3.amazonaws.com/pings/servers-2020-07-19.csv -o material/servers.csv

.PHONY: clean
clean:
	$(MAKE) stop
	rm -rf colonio-simulator.tar dump.json work $(DUMP_FILE) $(RENDER_DIR) $(RENDER_FILE)
