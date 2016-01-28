FROM golang:1.5-onbuild
RUN GOOS=darwin GOARCH=amd64 go-wrapper install -ldflags '"-s"'

