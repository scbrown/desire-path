# Releasing and re-proving the README

The README pins a release archive, checksum file, and exact version output.
Changing the release tag requires a new proof; updating the URL alone is not enough.

After publishing a release:

1. Download the new archive and checksum file from the published release into an
   empty directory. Verify the matching SHA-256 before extracting or executing it.
2. Update the tag and asset names in both the README and
   [Getting started](getting-started.md), and update the pinned source-install
   version in `.github/workflows/ci.yml`.
3. Extract the README's install, first-success, and agent-setup shell blocks.
   Run them under `env -i` with a fresh temporary HOME and a PATH containing only
   the release binary and the commands the example requires. Use copies of
   binaries, never symlinks to a production installation. Agent setup must write
   only inside that temporary HOME.
4. Compare `dp version` with the documented version/commit. Diff the demo's stdout
   against the README's own `text` block. Run every command in the question table
   after seeding a synthetic failure in the isolated default database.
5. Record the tag, archive, checksum, platform, exact stdout comparison, and
   resulting hook configuration in the documentation PR. Other platforms need
   their own checks; a Linux amd64 run proves only that archive.
6. Run `python3 scripts/test-md-links.py`, `python3 scripts/check-md-links.py`,
   `pre-commit run --all-files`, and `mdbook build docs/book`. Require the PR's
   source-install, vet, unit, and integration CI steps to finish successfully.

Publish the documentation update only after the new release artifacts exist.
A source build does not prove a release archive, and a release archive does not
prove the source-install instructions; retain both checks.
