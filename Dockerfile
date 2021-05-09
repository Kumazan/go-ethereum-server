FROM golang:1.16.4-alpine
RUN apk add build-base
WORKDIR /go/src
COPY . .
RUN apk add --no-cache git
RUN go get -d -v ./...
EXPOSE 8080
