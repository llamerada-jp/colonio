SHELL := /bin/bash -o pipefail

TAG := ghcr.io/llamerada-jp/colonio-simulator
SIMULATION_STORY := circle
NODES := 5
DUMP_FILE := dump.json
RENDER_FILE := out.mp4
RENDER_DIR := render

# command
KUBECTL := sudo k3s kubectl

.PHONY: apply
apply: colonio-simulator.tar
	sudo k3s ctr images import colonio-simulator.tar
	$(KUBECTL) delete namespace colonio-simulator || true
	$(KUBECTL) apply -k deploy/overlays/$(SIMULATION_STORY)
	$(KUBECTL) scale deployment -n colonio-simulator node --replicas=$(NODES)

.PHONY: stop
stop:
	$(KUBECTL) delete namespace colonio-simulator || true

.PHONY: export
export: work/simulator
	work/simulator export --mongodb-uri mongodb://localhost:30017 --output $(DUMP_FILE)

.PHONY: render
render: work/simulator
	rm -rf $(RENDER_DIR)
	mkdir -p $(RENDER_DIR)
	FONT_PATH=/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf work/simulator render --story $(SIMULATION_STORY) --input $(DUMP_FILE) --output-prefix $(RENDER_DIR)/image-
	ffmpeg -y -r 10 -i $(RENDER_DIR)/image-%06d.png -vcodec libx264 -pix_fmt yuv420p -r 10 $(RENDER_FILE)

.PHONY: view
view: work/simulator
	work/simulator render --input $(DUMP_FILE)

colonio-simulator.tar: Dockerfile work/simulator
	openssl req -x509 -out work/seed.crt -keyout work/seed.key \
    -newkey rsa:2048 -nodes -sha256 \
    -subj '/CN=seed.colonio-simulator.svc.cluster.local' -extensions EXT -config <( \
    printf "[dn]\nCN=seed.colonio-simulator.svc.cluster.local\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:seed.colonio-simulator.svc.cluster.local\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")
	docker build . -t $(TAG)
	docker save $(TAG) -o $@

work/simulator: $(shell find ../ -type f -name '*.go')
	$(MAKE) -C ..
	mkdir -p work
	go build -o $@ ./cmd/

.PHONY: setup
setup:
	sudo apt install -y pkgconf ffmpeg fonts-noto libsdl2{,-image,-ttf}-dev

.PHONY: clean
clean:
	$(MAKE) stop
	rm -rf colonio-simulator.tar dump.json work $(DUMP_FILE) $(RENDER_DIR) $(RENDER_FILE)
