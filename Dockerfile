# Runtime image for goreleaser
# The binary is pre-built by goreleaser and copied into the context
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 dlia \
    && adduser -D -u 1000 -G dlia dlia
COPY dlia /usr/local/bin/dlia
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
