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

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /out/server /app/server
COPY --from=builder --chown=nonroot:nonroot /out/data /data

ENV APP_ADDR=:8080
ENV APP_SETTINGS_FILE=/data/settings.json
ENV APP_SKILLS_DIR=/data/skills
ENV APP_SKILLS_STATE_FILE=/data/skills_state.json
ENV APP_CONVERSATION_FILE=/data/conversation.json
ENV APP_LLM_LOG_FILE=/data/llm_logs.json
EXPOSE 8080
VOLUME ["/data"]

USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
