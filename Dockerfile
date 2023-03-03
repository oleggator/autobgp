FROM golang:1.20-alpine3.17 as build

RUN apk add upx

WORKDIR /opt/autobgp

COPY go.mod go.sum ./
RUN go mod download

COPY *.go .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/local/bin/autobgp autobgp \
    && upx /usr/local/bin/autobgp


FROM alpine:3.17

LABEL org.opencontainers.image.source=https://github.com/oleggator/autobgp
LABEL org.opencontainers.image.description="autobgp"
LABEL org.opencontainers.image.licenses=MIT

COPY --from=build /usr/local/bin/autobgp /usr/local/bin/autobgp

CMD ["/usr/local/bin/autobgp"]
