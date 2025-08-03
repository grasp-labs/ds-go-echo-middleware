# 📌 Versioning

This project follows Semantic Versioning 2.0.0 to indicate release stability and compatibility:

MAJOR.MINOR.PATCH
MAJOR – breaking changes
MINOR – new features, backward compatible
PATCH – bug fixes, small improvements

## 🏷️ Tagging & Releasing a Service (With Pre-Release Lifecycle)

Each service/component follows Semantic Versioning (SemVer) and tags its releases accordingly.

| Stage  | Purpose                            | Tag format       | Example          |
| ------ | ---------------------------------- | ---------------- | ---------------- |
| Alpha  | Internal dev/testing               | `vX.Y.Z-alpha.N` | `v1.2.0-alpha.1` |
| Beta   | Feature complete, QA/staging ready | `vX.Y.Z-beta.N`  | `v1.2.0-beta.1`  |
| RC     | Final QA, candidate for production | `vX.Y.Z-rc.N`    | `v1.2.0-rc.1`    |
| Stable | Production release                 | `vX.Y.Z`         | `v1.2.0`         |


To cut a new release:

```bash
# 1. Commit all changes
git commit -am "Prepare release v1.2.0-beta.1"

# 2. Create a version tag (e.g., alpha, beta, rc, or stable)
git tag v1.2.0-beta.1

# 3. Push the tag to origin (this triggers CI/CD & DynamoDB fragment creation)
git push origin v1.2.0-beta.1
```

To dry run locally:

```bash
# Install if needed
brew install goreleaser

# Run dry-run release
goreleaser release --snapshot --skip-publish --rm-dist

# Local release - assumes GITHUB_TOKEN is defined as environment variable
goreleaser release --clean --config .goreleaser.yml
```

## 🔁 Example Release Flow per Tag Type:

| Tag              | Pipeline Behavior                                            |
| ---------------- | ------------------------------------------------------------ |
| `v1.4.0-alpha.1` | Pushes metadata to DynamoDB → Deploys to Dev Environments    |
| `v1.4.0-beta.1`  | Pushes metadata to DynamoDB → Deploys to Staging/QA          |
| `v1.4.0-rc.1`    | Pushes metadata to DynamoDB → Deployed to Production Preview |
| `v1.4.0`         | Pushes metadata to DynamoDB → Deployed to Production         |

## 🏷️ Simple Command to Get Latest Tag (by commit date):

```bash
git describe --tags --abbrev=0
```

This will give you the latest reachable tag in history, e.g., v1.4.2.
It works even if the current commit is ahead of the tag.

## 🏷️ Get Latest Tag Sorted by Creation (Lexical Order)

```git
git tag --sort=-creatordate | head -n 1
```

This will list tags sorted by creation date, with the newest first.
Useful if you want the most recently created tag, not necessarily the latest reachable tag.
