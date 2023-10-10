FROM golang:1.20-alpine as buildbase

WORKDIR /go/src/github.com/rarimo/near-saver-svc
RUN apk add build-base
COPY vendor .
COPY . .

ENV GO111MODULE="on"
ENV CGO_ENABLED=1
ENV GOOS="linux"

RUN go build -o /usr/local/bin/near-saver-svc github.com/rarimo/near-saver-svc

###

FROM alpine:3.9

COPY --from=buildbase /usr/local/bin/near-saver-svc /usr/local/bin/near-saver-svc
RUN apk add --no-cache ca-certificates

ENTRYPOINT ["near-saver-svc"]
