ARG GO_VERSION=1.26.1

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=2.2.0
ARG COMMIT=unknown

RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/dotfiles-mcp ./cmd/dotfiles-mcp

FROM alpine:3.22

ARG VERSION=2.2.0
ARG COMMIT=unknown

LABEL org.opencontainers.image.title="dotfiles-mcp"
LABEL org.opencontainers.image.description="Hyprland, Wayland desktop, GitHub org, and fleet automation for Linux workstations."
LABEL org.opencontainers.image.source="https://github.com/hairglasses-studio/dotfiles-mcp"
LABEL org.opencontainers.image.url="https://github.com/hairglasses-studio/dotfiles-mcp"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL io.modelcontextprotocol.server.name="io.github.hairglasses-studio/dotfiles-mcp"

RUN apk add --no-cache bash ca-certificates git openssh-client procps

COPY --from=build /out/dotfiles-mcp /usr/local/bin/dotfiles-mcp

ENV DOTFILES_MCP_PROFILE=default
ENV HOME=/tmp

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/dotfiles-mcp"]
