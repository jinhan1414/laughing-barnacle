# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN mkdir -p /out/data

FROM debian:12 AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      curl \
      wget \
      git \
      nodejs \
      npm \
      jq \
      vim-tiny \
      nano \
      less \
      procps \
      iproute2 \
      iputils-ping \
      net-tools \
      dnsutils \
      unzip \
      zip \
      tzdata \
      tree \
      file \
      lsof && \
    if ! command -v npx >/dev/null 2>&1; then npm install -g npx; fi && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd --system app && \
    useradd --system --gid app --create-home --home-dir /home/app app

WORKDIR /app

COPY --from=builder --chown=app:app /out/server /app/server
COPY --from=builder --chown=app:app /out/data /data

ENV APP_ADDR=:8080
ENV APP_SETTINGS_FILE=/data/settings.json
ENV APP_SKILLS_DIR=/data/skills
ENV APP_SKILLS_STATE_FILE=/data/skills_state.json
ENV APP_CONVERSATION_FILE=/data/conversation.json
ENV APP_LLM_LOG_FILE=/data/llm_logs.json
EXPOSE 8080
VOLUME ["/data"]

USER app:app
ENTRYPOINT ["/app/server"]
