# Build stage
FROM golang:1.25.5-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X github.com/zorak1103/dlia/internal/version.Version=${VERSION} \
    -X github.com/zorak1103/dlia/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/zorak1103/dlia/internal/version.BuildDate=${BUILD_DATE}" \
    -o dlia .

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 dlia \
    && adduser -D -u 1000 -G dlia dlia
COPY --from=builder /build/dlia /usr/local/bin/dlia
RUN mkdir -p /data/reports /data/knowledge_base/services /data/logs/llm /data/config \
    && chown -R dlia:dlia /data
WORKDIR /data
USER dlia
VOLUME ["/data", "/var/run/docker.sock"]
ENV DLIA_OUTPUT_REPORTS_DIR=/data/reports \
    DLIA_OUTPUT_KNOWLEDGE_BASE_DIR=/data/knowledge_base \
    DLIA_OUTPUT_STATE_FILE=/data/state.json
ENTRYPOINT ["dlia"]
CMD ["--help"]
