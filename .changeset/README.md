# Changesets

This directory contains [Changesets](https://github.com/changesets/changesets) metadata.

## Workflow

1. After making user-facing changes in a workspace package, run:

   ```bash
   pnpm changeset
   ```

   Select the affected packages and describe the change.

2. Before releasing, version packages:

   ```bash
   pnpm version-packages
   ```

3. Publish (when ready):

   ```bash
   pnpm release
   ```

In this repository Changesets is used primarily for the TypeScript workspace
(`hnsx-console`, `@hnsx/observability`, `@hnsx/sdk-node`). Go and Python
modules are versioned independently via their own manifests and the top-level
`VERSION` make variable.
