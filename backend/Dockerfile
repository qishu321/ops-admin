FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/library/golang:1.24-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ops-admin .

FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/library/alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S opsadmin \
    && adduser -S -G opsadmin -h /app opsadmin \
    && mkdir -p /app/uploads \
    && chown -R opsadmin:opsadmin /app

WORKDIR /app
COPY --from=builder --chown=opsadmin:opsadmin /out/ops-admin /app/ops-admin

USER opsadmin

EXPOSE 8082 53/tcp 53/udp
CMD ["/app/ops-admin"]
