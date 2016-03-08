IMAGE=dmcsorley/ccdr
DEPS=dmcsorley/cdrdeps

help:
	cat Makefile

partial:
	docker build -f Dockerfile.partial -t $(IMAGE) .

image:
	docker pull golang
	docker build -t $(IMAGE) .

deps:
	docker build -f Dockerfile.deps -t $(DEPS) .

tagdeps:
	docker tag $(IMAGE) $(DEPS)

run:
	docker run -it --rm -v $$PWD:/to --name=ccdr $(IMAGE) cp /go/bin/darwin_amd64/clusterator /to/clusterator

bash:
	docker run -it --rm -v $$HOME:/home/me -e HOME=/home/me --name=ccdr $(IMAGE) bash

dangling:
	docker ps -aq | xargs docker rm -fv
	docker images --filter dangling=true -q | xargs docker rmi
