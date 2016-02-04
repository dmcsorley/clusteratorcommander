FROM golang

RUN mkdir -p /go/src/clusterator
WORKDIR /go/src/clusterator

COPY . /go/src/clusterator

RUN go-wrapper download

RUN go-wrapper install

RUN GOOS=darwin GOARCH=amd64 go-wrapper install -ldflags '"-s"'

CMD ["go-wrapper", "run"]
