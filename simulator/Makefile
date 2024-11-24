SHELL := /bin/bash -o pipefail

TAG := ghcr.io/llamerada-jp/colonio-simulator
DEPLOY_OVERLAY := sphere
NODES := 3

# command
KUBECTL := sudo k3s kubectl

.PHONY: apply
apply: colonio-simulator.tar
	sudo k3s ctr images import colonio-simulator.tar
	$(KUBECTL) delete namespace colonio-simulator || true
	$(KUBECTL) apply -k deploy/overlays/$(DEPLOY_OVERLAY)
	$(KUBECTL) scale deployment -n colonio-simulator node --replicas=$(NODES)

.PHONY: export
export: work/simulator
	work/simulator export --mongodb-uri mongodb://localhost:30017 --output dump.json

colonio-simulator.tar: ../output/libcolonio.a work/libwebrtc.a work/simulator work/seed
	openssl req -x509 -out work/seed.crt -keyout work/seed.key \
    -newkey rsa:2048 -nodes -sha256 \
    -subj '/CN=seed.colonio-simulator.svc.cluster.local' -extensions EXT -config <( \
    printf "[dn]\nCN=seed.colonio-simulator.svc.cluster.local\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:seed.colonio-simulator.svc.cluster.local\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")
	docker build . -t $(TAG)
	docker save $(TAG) -o $@

work/libcolonio.a: ../output/libcolonio.a
	$(MAKE) -C ..
	mkdir -p work
	cp -a $< $@

work/libwebrtc.a: ../dep/lib/libwebrtc.a
	$(MAKE) -C ..
	mkdir -p work
	cp -a $< $@

work/simulator: $(shell find . -type f -name '*.go')
	mkdir -p work
	go build -o $@ ./cmd/

work/seed: ../output/seed
	mkdir -p work
	cp -a $< $@

clean:
	rm -rf work
