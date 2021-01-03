FROM golang:1.12-alpine
RUN apk update
RUN apk add git
WORKDIR /usr/src
ADD go.mod .
RUN go mod download
ADD $PWD /usr/src/app
WORKDIR /usr/src/app
RUN go build github.com/apoorvprecisely/envoy-poc/build/app -o out/app

FROM alpine:3.9.2
WORKDIR /opt/app
COPY --from=0 usr/src/app/out/app .
RUN apk update

ENV PORT=8053 \
	LOG_LEVEL=INFO \
	CONSUL_CLIENT_PORT=8500 \
	CONSUL_CLIENT_HOST= \
	CONSUL_DC=dc1 \
	CONSUL_TOKEN= \
	WATCHED_SERVICE=
EXPOSE ${PORT}
CMD ./app