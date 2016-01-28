IMAGE=dmcsorley/ccdr

help:
	cat Makefile

partial:
	docker build -f Dockerfile.partial -t $(IMAGE) .

image:
	docker build -t $(IMAGE) .

deps:
	docker tag -f $(IMAGE) dmcsorley/cdrdeps

run:
	docker run -it --rm -v $$PWD:/to --name=ccdr $(IMAGE) cp /go/bin/darwin_amd64/app /to/app

bash:
	docker run -it --rm -v $$HOME:/home/me -e HOME=/home/me --name=ccdr $(IMAGE) bash

dangling:
	docker images --filter dangling=true -q | xargs docker rmi