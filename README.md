# vfs-layer

`vfs-layer` exposes a FUSE view with original filenames while storing files on
disk under short, safe names. Unsafe names are preserved in colocated
`<safe-name>.meta` text files so `/storage` remains manually recoverable even
without the mount.

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

See `SPEC.md` for the full behavior contract.
