FROM golang:1.22 AS build

WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /vfs ./cmd/vfs

FROM debian:bookworm

RUN apt-get update \
	&& apt-get install -y --no-install-recommends fuse3 ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=build /vfs /usr/local/bin/vfs
CMD ["vfs", "mount", "--storage", "/storage", "--view", "/view"]
