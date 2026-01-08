ARG NODE_VERSION=22
ARG GO_VERSION=1.24

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS frontend
WORKDIR /app
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web ./web
RUN cd web && npm run build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY Makefile ./
COPY web/components.json ./web/components.json
COPY web/tailwind.config.cjs web/postcss.config.cjs web/vite.config.ts web/tsconfig*.json ./web/
COPY web/src ./web/src
COPY --from=frontend /app/web/dist ./web/dist
RUN mkdir -p internal/static && rm -rf internal/static/dist && cp -r web/dist internal/static/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /dashboard ./cmd/dashboard

FROM --platform=$TARGETPLATFORM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=backend /dashboard /app/dashboard
ENV LISTEN_ADDR=:9090
ENV GRPC_ADDR=:9091
EXPOSE 9090 9091
ENTRYPOINT ["/app/dashboard"]
