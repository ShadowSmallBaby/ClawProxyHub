# syntax=docker/dockerfile:1
FROM node:22-bookworm AS web
ARG CPH_REPOSITORY_URL
ENV CPH_REPOSITORY_URL=${CPH_REPOSITORY_URL}
WORKDIR /src/web
RUN npm install -g pnpm@10
RUN apt-get update && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*
COPY build-config.json /src/build-config.json
COPY project.toml /src/project.toml
COPY scripts/project_config.py /src/scripts/project_config.py
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ .
RUN pnpm build

FROM web AS builder
COPY --from=golang:1.26-bookworm /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
RUN apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 本地镜像使用独立构建身份，最终镜像只保留公钥与已签名包。
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/src/.cache \
    python3 scripts/build-release.py --platform "linux/$(go env GOARCH)" --distribution full --prebuilt-web --development-key --out /out/archives \
    && python3 -c "from pathlib import Path; import zipfile; zipfile.ZipFile(next(Path('/out/archives').glob('*-full.zip'))).extractall('/out/app')" \
    && chmod 755 /out/app/ClawProxyHub/cph /out/app/ClawProxyHub/cli

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /out/app/ClawProxyHub/ ./
RUN mkdir -p data/packages /opt/clawproxyhub && mv data/packages /opt/clawproxyhub/packages && rmdir data
ENV CPH_ADDR=":8080" CPH_DATA_DIR="/app/data" CPH_INSTALL_PACKAGES="true" CPH_PACKAGE_DIRS="/opt/clawproxyhub/packages:/app/data/packages"
VOLUME ["/app/data"]
EXPOSE 8080
ENTRYPOINT ["./cph"]
