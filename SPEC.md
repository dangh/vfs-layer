# vfs-layer Specification

Status: target specification for the initial implementation.

`vfs-layer` is a FUSE-based virtual filesystem that exposes human-readable,
original filenames while storing every file and directory on disk using safe
backend names. The design exists to survive hostile filename environments:
Unraid, SMB, ext4, XFS, macOS, Windows clients, remote download tools, and any
source that produces names too long or incompatible for the target storage.

The core rule is:

> Storage must remain directly usable and self-describing even when the virtual
> filesystem is gone.

No database is required. No hidden state is required. A user must be able to open
`/storage`, see safe files plus colocated `.meta` files, and manually recover the
original filenames.

## 1. Goals

- Store files and directories locally using filesystem-safe names.
- Preserve the full original name without truncation.
- Expose original names through a mounted FUSE view.
- Hide internal `.meta` files from the mounted view.
- Automatically create, update, or remove metadata as names change.
- Keep `/storage` readable, browsable, and recoverable without FUSE.
- Support files and directories with the same naming rules.
- Avoid any required database, registry, index, or opaque state.

## 2. Non-goals

- Cloud synchronization.
- Content deduplication.
- Encryption.
- Multi-writer distributed coordination.
- A required persistent database.
- Perfect POSIX compatibility beyond normal file browsing, reading, writing,
  renaming, directory creation, and deletion.

## 3. Terminology

- **Virtual name**: the name users see in `/view`. This is the original filename
  or directory name.
- **Safe name**: the actual name stored in `/storage`. This name is short,
  deterministic, and valid on the target filesystem.
- **Sidecar metadata**: a colocated text file whose name is `<safe-name>.meta`.
  Its contents are only the original virtual name.
- **Storage path**: the real backend path under `/storage`.
- **View path**: the FUSE path under `/view`.

## 4. Filesystem Shape

Example storage:

```text
/storage/
  a8f3c2.mp4
  a8f3c2.mp4.meta
  holiday/
  f91b20/
  f91b20.meta
```

Example metadata:

```text
⚡女大学生母狗嫩妹.mp4
```

Example view:

```text
/view/
  ⚡女大学生母狗嫩妹.mp4
  holiday/
  Very Long Original Directory Name ...
```

The view maps:

```text
/view/⚡女大学生母狗嫩妹.mp4
```

to:

```text
/storage/a8f3c2.mp4
/storage/a8f3c2.mp4.meta
```

## 5. Metadata Contract

Each file or directory may have one optional sidecar metadata file:

```text
<safe-name>.meta
```

The metadata file contains exactly one value:

```text
<original filename>
```

Rules:

- The metadata content is UTF-8 text.
- The metadata content is the original basename only, not a path.
- The metadata content must be human-readable in a normal text editor.
- Metadata must not contain JSON, binary data, indexes, checksums, or hidden
  implementation fields.
- Metadata is only required when the virtual name cannot be safely represented
  directly as the storage name.
- If the storage name is already safe and exact, no `.meta` file is needed.
- `.meta` files are hidden from `/view`.

Line ending:

- The implementation may tolerate a trailing newline when reading metadata.
- When writing metadata, it should write the exact original name plus a single
  trailing newline for editor friendliness.
- The trailing newline is not part of the virtual name.

## 6. Safe Name Rules

A name is safe only if it can be stored directly on the backend without risk of
rejection, truncation, ambiguity, or cross-client incompatibility.

A name is unsafe if any of the following are true:

- It is empty.
- It is `.` or `..`.
- It contains `/` or the platform path separator.
- It exceeds the configured safe length limit.
- Its byte length exceeds the backend limit.
- It contains characters forbidden by common SMB or Windows clients.
- It is a reserved device name such as `CON`, `PRN`, `AUX`, `NUL`, `COM1`, or
  `LPT1`, case-insensitively.
- It has trailing spaces or dots.
- It normalizes ambiguously under Unicode normalization.
- It conflicts case-insensitively with another entry in the same directory when
  case-insensitive clients are expected.

Default safe length:

```text
max_safe_basename_bytes = 120
```

This limit is intentionally lower than common filesystem limits so there is room
for suffixes, extensions, `.meta`, and SMB/client quirks.

## 7. Safe Name Generation

Unsafe virtual names are converted to deterministic safe names.

Recommended format for files:

```text
<hash><extension>
```

Example:

```text
a8f3c2.mp4
```

Recommended format for directories:

```text
<hash>
```

Example:

```text
f91b20
```

Directory type is determined from the storage entry itself, not from a naming
suffix.

Hash requirements:

- Deterministic for the original virtual name.
- Short enough for conservative filesystems.
- Long enough to make accidental collisions unlikely.
- Case-stable and path-safe.
- Uses only `[a-z0-9_-]`.

Recommended hash:

```text
base32(sha256(original_name))[0:16]
```

Extension handling:

- For regular files, preserve the final extension when it is itself safe and not
  too long.
- If the extension is unsafe or too long, omit it from the safe name.
- The full original extension remains recoverable from `.meta`.

Collision handling:

- If generated safe name already exists for a different virtual name, append a
  deterministic suffix.
- Example: `a8f3c2-2.mp4`, `a8f3c2-3.mp4`.
- The chosen storage name must be stable once created.
- Collision resolution must never modify another existing file.

## 8. Directory Support

Directories follow the same rules as files.

Safe directory:

```text
/storage/Movies/
```

Unsafe directory:

```text
/storage/f91b20/
/storage/f91b20.meta
```

If `f91b20.meta` contains:

```text
Very Long Original Directory Name
```

then `/view` exposes:

```text
/view/Very Long Original Directory Name/
```

Rules:

- Directory metadata is stored beside the safe directory, in the parent
  directory.
- Children are stored inside the safe directory.
- Directory `.meta` files are hidden from `/view`.
- Deleting a directory through `/view` deletes its sidecar metadata too.
- Renaming a directory through `/view` updates the safe directory name and
  sidecar metadata according to the same rules as files.

## 9. View Mapping

The system maps names at each directory level:

```text
virtual name <-> storage safe name
```

When listing a directory in `/view`:

1. Read entries from the corresponding `/storage` directory.
2. Ignore entries ending in `.meta`.
3. For each remaining entry, check whether `<entry>.meta` exists.
4. If metadata exists, display metadata content as the virtual name.
5. If metadata does not exist, display the storage basename.

When resolving a path from `/view`:

1. Split the view path into components.
2. For each component, scan the corresponding storage directory.
3. Compare the requested component with each entry's virtual name.
4. Descend into the matched storage entry.
5. Return not found if no entry maps to that virtual name.

The implementation may use in-memory caches for performance, but caches are
optional and rebuildable by scanning `/storage`.

## 10. Create Behavior

When a user creates a file or directory through `/view`:

1. Validate the requested virtual basename.
2. If the name is safe, create the storage entry with that exact basename and no
   metadata.
3. If the name is unsafe, generate a safe name, create the storage entry, and
   write `<safe-name>.meta` containing the original basename.
4. Ensure the generated safe name does not collide with an existing storage
   entry or sidecar.

Create must be atomic enough that partial failures are recoverable from
`/storage`.

## 11. Rename Behavior

When a user renames an entry through `/view`:

1. Resolve the source virtual path to its storage entry.
2. Determine whether the new virtual basename is safe.
3. If safe:
   - Rename the storage entry to the exact new basename.
   - Remove the old sidecar metadata if present.
4. If unsafe:
   - Generate a new safe storage basename.
   - Rename the storage entry to the new safe basename.
   - Write or update `<new-safe-name>.meta` with the full new virtual basename.
   - Remove the old sidecar metadata if the safe basename changed.
5. Preserve file contents and directory children.
6. Never expose `.meta` files as normal entries in `/view`.

Crash recovery expectation:

- If a crash leaves an orphan `.meta` file, `/storage` remains manually
  understandable.
- If a crash leaves a safe file without metadata, the file remains visible in
  `/view` under its safe name.
- A future `check` or `repair` command may clean orphan metadata, but the core
  design must not require it for basic access.

## 12. Delete Behavior

When deleting through `/view`:

- Delete the storage entry.
- Delete its colocated sidecar metadata if present.
- Deleting a `.meta` path directly through `/view` is impossible because metadata
  is hidden.

If a sidecar remains after a crash, it is an orphan text file in `/storage` and
can be inspected or removed manually.

## 13. Read and Write Behavior

File reads and writes pass through to the mapped storage file.

Rules:

- File contents are not stored in metadata.
- Metadata changes only when the virtual basename changes.
- Writing to a file must not rewrite or remove its sidecar metadata.
- Truncation, append, chmod, and timestamp behavior should follow the backend
  filesystem where practical.

## 14. Hidden Metadata in View

The following entries must not appear in `/view`:

- Any file whose name ends with `.meta`.
- Any implementation temporary file prefix used for atomic metadata writes.

If a user tries to access a hidden metadata path through `/view`, the filesystem
must return not found.

Metadata remains visible in `/storage` by design.

## 15. Caveman Recoverability

This is a hard requirement.

If the binary, config, caches, mount, or host service all disappear, the user can
still recover names manually by opening `/storage`.

For every unsafe entry:

```text
safe-file.ext
safe-file.ext.meta
```

The user can:

1. Open `safe-file.ext.meta`.
2. Read the original filename.
3. Rename `safe-file.ext` manually if desired.

For every unsafe directory:

```text
safe-dir/
safe-dir.meta
```

The user can:

1. Open `safe-dir.meta`.
2. Read the original directory name.
3. Rename `safe-dir` manually if desired.

Therefore:

- No required registry.
- No required database.
- No required WAL.
- No required hidden mapping file.
- No required external tool to decode metadata.

## 16. CLI

The `vfs` binary should provide:

```text
vfs mount --storage /storage --view /view
vfs check --storage /storage
vfs repair --storage /storage
vfs encode-name <name>
vfs version
```

`mount`:

- Mounts `/view`.
- Serves original names from sidecar metadata.
- Creates sidecars automatically when needed.

`check`:

- Scans `/storage`.
- Reports orphan `.meta` files.
- Reports entries with invalid metadata.
- Reports virtual name collisions within the same directory.
- Reports generated safe-name collisions.
- Does not require FUSE.

`repair`:

- Optional command.
- Cleans orphan metadata only with explicit confirmation or a dry-run report.
- Must not delete user data silently.

`encode-name`:

- Shows the safe storage name that would be generated for a virtual basename.
- Useful for debugging and documentation.

## 17. Configuration

The service may be configured by flags or YAML.

Example:

```yaml
storage_root: /storage
view_root: /view
max_safe_basename_bytes: 120
hash_length: 16
metadata_suffix: .meta
preserve_safe_extensions: true
case_insensitive_collision_checks: true
```

All configuration must be reproducible from plain text. No config value may be
required to decode an existing `.meta` file, though hash settings affect future
safe-name generation.

## 18. Implementation Layout

Recommended Go layout:

```text
cmd/vfs/
  main.go
  cli.go
  mount.go
  check.go
  repair.go

internal/storage/
  localfs.go
  paths.go

internal/meta/
  sidecar.go

internal/naming/
  safe.go
  hash.go
  normalize.go

internal/view/
  fs.go
  lookup.go
  readdir.go
  rename.go
  create.go
  delete.go

internal/check/
  scan.go
  repair.go
```

The implementation should keep FUSE-specific code in `internal/view` and keep
sidecar/naming logic testable without mounting FUSE.

## 19. FUSE Semantics

Minimum supported operations:

- lookup,
- readdir,
- open,
- read,
- write,
- create,
- mkdir,
- rename,
- unlink,
- rmdir,
- getattr,
- setattr where needed for truncation and timestamps.

Behavior:

- `/view` displays virtual names.
- `/view` hides `.meta`.
- File handles point to storage files.
- Directory handles list translated entries.
- Rename and create operations update sidecar metadata automatically.

Inode stability is nice to have but is secondary to caveman recoverability. A
restart may reconstruct inodes from storage paths unless stronger stability is
implemented without required hidden state.

## 20. Atomicity and Failure Handling

Metadata writes should use:

```text
write <sidecar>.tmp
fsync file where supported
rename <sidecar>.tmp -> <sidecar>
fsync parent directory where supported
```

Storage renames should use backend atomic rename where available.

Failure rules:

- Never delete the original storage entry until the replacement path exists.
- Never hide user data because metadata is missing or corrupt.
- If metadata cannot be read, expose the safe storage name in `/view`.
- If two entries map to the same virtual name, expose neither ambiguously; report
  the collision in logs and `vfs check`.

## 21. Collision Rules

Collisions are evaluated per directory.

Types:

- Two storage entries with metadata that produce the same virtual name.
- One safe storage name and one sidecar-backed entry with the same virtual name.
- Two unsafe names whose hashes generate the same safe storage name.

Required behavior:

- Create and rename must reject virtual name collisions.
- Safe-name hash collisions must be resolved by suffixing or rejected before
  writing.
- `readdir` must avoid returning duplicate names for the same directory.
- `check` must report all collisions with storage paths.

## 22. Security and Safety

- View paths must not escape `/storage`.
- Metadata content must be treated as a basename, never as a path.
- Metadata containing `/`, NUL, or path traversal names is invalid.
- Symlink handling must be explicit. The safest initial behavior is to expose
  symlinks as symlinks only if the backend target remains inside `/storage`.
- The implementation must not follow malicious sidecars outside the storage
  root.

## 23. Docker / Unraid Deployment

The service should run in a container with FUSE access.

Required mounts:

```text
/storage  real backend storage
/view     FUSE mountpoint
/config   optional plain-text config only
```

Container requirements:

- `/dev/fuse`
- `SYS_ADMIN` capability or equivalent host setting
- `rshared` or appropriate mount propagation if the host must see `/view`

Example command:

```text
vfs mount --storage /storage --view /view --config /config/vfs.yaml
```

## 24. Tests

Unit tests:

- safe-name detection,
- hash generation,
- extension preservation,
- sidecar read/write,
- metadata newline handling,
- invalid metadata rejection,
- virtual-to-storage lookup,
- storage-to-virtual display mapping,
- collision detection.

Integration tests:

- safe file appears without metadata,
- unsafe file creates `.meta`,
- unsafe directory creates `.meta`,
- `.meta` files are hidden from `/view`,
- read/write passes through to storage file,
- rename unsafe to safe removes metadata,
- rename safe to unsafe creates metadata,
- rename unsafe to unsafe updates safe name and metadata,
- delete removes sidecar,
- missing metadata falls back to safe name,
- orphan metadata does not break directory listing,
- duplicate virtual names are detected.

Manual recovery test:

1. Create an unsafe file through `/view`.
2. Stop FUSE.
3. Open `/storage`.
4. Confirm the storage file is present.
5. Open `<safe-name>.meta`.
6. Confirm the original filename is present as readable text.

## 25. Acceptance Criteria

The initial implementation is acceptable when:

- `go test ./...` passes.
- `vfs mount --storage /storage --view /view` mounts successfully.
- A long or incompatible filename can be created through `/view`.
- The actual storage file uses a short safe name.
- The sidecar contains only the original filename as readable text.
- `/view` shows the original filename.
- `/view` does not show `.meta` files.
- Renaming through `/view` updates safe storage and metadata correctly.
- Directly browsing `/storage` is enough to manually reconstruct original names.
- Removing any optional cache does not affect recoverability.

## 26. Design Priority

When tradeoffs appear, choose in this order:

1. Preserve user data.
2. Preserve original names.
3. Keep `/storage` self-describing.
4. Keep behavior predictable across SMB/Unraid/local filesystems.
5. Keep the implementation simple.
6. Optimize performance with optional rebuildable caches only after correctness.
