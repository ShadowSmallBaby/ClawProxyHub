# 多阶段构建：仪表盘（node）→ 核心（go）→ 运行镜像
# 镜像只含核心；插件在仪表盘「插件市场」在线安装，落在 data/ 卷里持久化
FROM node:22-alpine AS web
WORKDIR /src/web
RUN npm install -g pnpm@10
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ .
RUN pnpm build

FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/web/dist ./web/dist
# mattn/go-sqlite3 需要 CGO（换 modernc 纯 Go 驱动后可 CGO_ENABLED=0）
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /out/cph ./cmd/cph

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app/
COPY --from=builder /out/cph ./cph

ENV CPH_ADDR=":8080" CPH_DATA_DIR="/app/data"
VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["./cph"]
