# vfs-layer

`vfs-layer` exposes a FUSE view with original filenames while storing files on
disk under short, safe names. Unsafe names are preserved in colocated
`<safe-name>.meta` text files so `/storage` remains manually recoverable even
without the mount.

## Run With Docker Compose

The published image is available from GitHub Container Registry:

```text
ghcr.io/dangh/vfs-layer:latest
```

Create the host directories first:

```sh
mkdir -p /mnt/user/vfs/storage /mnt/user/vfs/view /mnt/user/vfs/config
```

Then run:

```sh
docker compose up -d
```

The included `docker-compose.yml` mounts:

- `/mnt/user/vfs/storage` as `/storage`
- `/mnt/user/vfs/view` as `/view`
- `/mnt/user/vfs/config` as `/config`

If the package is private, log in first:

```sh
echo "$GITHUB_TOKEN" | docker login ghcr.io -u USERNAME --password-stdin
```

## Local Binary

```sh
vfs mount --storage /storage --view /view
```

Useful commands:

```sh
vfs check --storage /storage
vfs repair --storage /storage
vfs ingest --storage /storage --manifest manifest.jsonl
vfs encode-name 'very long filename.mp4'
```

## Build Locally

To build the container locally instead of using the published package:

```sh
docker build -t vfs-layer:local .
```

Then update `docker-compose.yml` to use:

```yaml
image: vfs-layer:local
pull_policy: never
```

## Publishing

GitHub Actions builds and publishes multi-architecture images to GitHub
Container Registry on pushes to `master`, version tags such as `v0.1.0`, and
manual workflow dispatches.

Published tags include:

- `latest` for `master`
- branch names
- version tags
- `sha-<commit>` tags

See `SPEC.md` for the full behavior contract.
