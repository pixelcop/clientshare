# Immutable object storage

## Storage layout

Fresh installations default to **`readable_view.mode: none`**, which does not
create a readable filesystem view. Authoritative local files live in `objects/<tenant>/<object-id>`;
S3 uses the same layout. File names and folders live in the database.

Existing local files still need an offline copy and database identity cutover.
The legacy directories remain intact for rollback. The migration uses verified
copies, never hardlinks to the rollback source. There is no S3 legacy migration.

```yaml
storage:
  type: local
  layout: immutable-v1
  local:
    root: /app/storage
    readable_view:
      mode: none
```

`storage_key` is nullable only for folders, unmigrated records, or explicitly
acknowledged pre-existing missing files. Normal startup refuses retained,
unmigrated records. Reads never fall back to `File.Path` or the readable view.
`Path` and `Client.FolderPath` are logical display paths. Activity paths remain
historical snapshots; existing public file IDs and links are preserved.

## Upgrading an existing local installation

Stop all application instances, workers and other database/storage writers.
Take and verify a consistent backup of the database, configuration and actual
storage contents, including any mounted filesystem targets. Rehearse restoration
before migrating. Retain the previous application version and configuration.

Run the new version's additive schema migrations, then use the storage commands
with the same database, configuration and storage mounts:

```sh
/app/goose -dir /app/migrations up
/app/clientshare storage inventory > /app/data/immutable-manifest.json
/app/clientshare storage migrate -manifest /app/data/immutable-manifest.json -view-mode none
/app/clientshare storage migrate -manifest /app/data/immutable-manifest.json -view-mode none -apply
/app/clientshare storage verify
```

Choose the view mode explicitly and configure it consistently before starting
the application. Container installations can use a one-off container with the
server entrypoint overridden. Do not run the application alongside migration.
Keep the manifest with the backup and verify file access before accepting writes.

Inventory does not write files or database rows; shell redirection creates only
the requested report. `migrate` without `-apply` reports the plan. Applied copies
use temporary files, fsync and no-replace publication. Rerunning with the **same
manifest** rehashes sources and destinations; it never overwrites an object.
After all copies verify, one transaction sets keys, sizes and lowercase SHA-256
checksums and refreshes logical paths. An interruption before that transaction
leaves the database on the old identities. An interruption during it rolls back.
There is no separate checkpoint service: the manifest and verified destinations
are the checkpoints.

### Inventory exceptions

The report includes retained soft-deleted files, counts of folders and already
physically deleted records, and unreferenced legacy entries. Nothing is removed.
Missing files, size/checksum mismatches and duplicate source mappings block
cutover. Check free space for a complete second copy plus the largest temporary
upload and the backup; do not place the only backup on the same NAS volume.

For a stale path, inspect the client's complete logical parent chain and source
metadata. Provide a reviewed source map; never use a basename-only search:

```json
{"TENANT_ID/FILE_ID": "tenants/TENANT_ID/Current Client/Nested/report.pdf"}
```

```sh
/app/clientshare storage inventory -sources /app/data/reviewed-sources.json > /app/data/immutable-manifest.json
```

Ambiguous duplicate source mappings need operator resolution before migration.
Do not assume a file on disk represents every historical record with that name.
If bytes were already missing and cannot be recovered, an operator may explicitly
acknowledge those specific records using a JSON array of `TENANT_ID/FILE_ID`:

```sh
/app/clientshare storage inventory -acknowledge-missing /app/data/reviewed-missing.json > /app/data/immutable-manifest.json
```

Those records keep their IDs, metadata and deletion state and are marked
`storage_missing`. Downloads still report missing; no replacement content is
invented. Verification reports their count separately from verified objects.
This is an operator assertion, not an automatic response to an inventory error.

### Rollback and retirement

Before reopening writes, restore the full pre-cutover database and old
configuration/image, pointing to the untouched legacy tree. Do not rely on
schema `down`: the identity migrations intentionally preserve recovery metadata.
Test this restoration on the rehearsal copy. After reopening writes, restoring
the old database loses new uploads and mutations; recovery then requires an
explicit reverse export/migration.

Agree on an observation period before retiring legacy files. There is no startup
cleanup. With verified backups retained, inspect then explicitly apply:

```sh
/app/clientshare storage legacy-cleanup -manifest /app/data/immutable-manifest.json
/app/clientshare storage legacy-cleanup -manifest /app/data/immutable-manifest.json -apply
/app/clientshare storage orphan-cleanup -grace 168h
/app/clientshare storage orphan-cleanup -grace 168h -apply
```

Legacy cleanup verifies both the original source and its referenced object and
removes only manifest-listed source files. It leaves directories and unknown
files for review. Orphan cleanup is local-only, checks database references,
requires at least 24 hours' grace, and excludes upload/migration temporary files.
Run cleanup during maintenance to avoid racing manual repair tools.

## Optional readable views

Modes: `none`, `hardlink`, `symlink`, `export`. Set `root` to a distinct managed
directory (default `<storage-root>/readable`) and `owner: true` on exactly one app
instance. A filesystem lock prevents competing owners of the same readable root.
Other instances leave reconciliation to that owner. Persistent desired/applied
revisions and manifests survive restarts and client deletion.

- `hardlink`: same filesystem required. In-place edits affect authoritative bytes.
  Never chmod links independently. Protect the readable share with read-only ACLs.
- `symlink`: relative links; validate that actual NAS clients can resolve them.
- `export`: independent copies with checksum verification and atomic publication.
  May use another filesystem. External edits are conflicts, never imports.
  Temporary copies use a private `.clientshare-staging` directory outside the
  tenant trees, on the destination filesystem. Exclude it from the public share.

Roots may not overlap objects, staging, view state or legacy directories. The
selected mode is capability-probed without fallback. Local object publication
requires atomic no-replace hardlinks within the object filesystem, even with
view mode `none`. **Test this on your storage filesystem before migrating.**
The application's tests cannot establish SMB/NFS server crash guarantees.

```sh
/app/clientshare storage view-status
/app/clientshare storage view-reconcile -tenant TENANT -client CLIENT
/app/clientshare storage view-reconcile -tenant TENANT -client CLIENT -apply
/app/clientshare storage verify
```

Reconciliation without `-apply` reports proposed creation/removal and conflicts.
Explicit reconciliation verifies all selected manifests, including clean ones;
the ordinary worker processes dirty clients only. Portal access does not depend
on reconciliation success. Retention waits for the client's applied revision
before deleting object bytes, so managed links/copies cannot retain them silently.

Names preserve logical spelling. Invalid/reserved NAS names and case-insensitive
collisions are reported; there are no silent ID suffixes. Unknown files, modified
exports and directory symlinks block reconciliation. Empty folders are managed.
Ordinary renames publish new entries before removing old owned entries. Same-name
replacement and case-only renames remove verified old entries first.

To switch mode/root or rebuild, stop the owner, run `storage view-clear -apply`
using the old manifest ownership, change configuration, then run
`storage view-reconcile -apply` and restart the owner. Clear also supports dry-run
and tenant/client filters. It refuses changed or unmanaged contents. Never delete
the manifest to bypass a conflict. Keep the authoritative tree outside the shared
root; grant read-only share/export access rather than relying on documentation.

S3 uploads spool to a temporary file to keep memory bounded and provide seekable
retry bodies. Conditional PUT prevents replacement and sends SHA-256; retries
are bounded to four SDK attempts. Current request limits do not need multipart
uploads. S3 ETags are never treated as content checksums.
