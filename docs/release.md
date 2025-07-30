# 📌 Versioning

This project follows Semantic Versioning 2.0.0 to indicate release stability and compatibility:

MAJOR.MINOR.PATCH
MAJOR – breaking changes
MINOR – new features, backward compatible
PATCH – bug fixes, small improvements

## 🏷️ Tagging a Release

To cut a new release:

```bash
# 1. Commit all changes
git commit -am "Prepare release v1.0.0"

# 2. Create a version tag
git tag v1.0.0

# 3. Push the tag to origin
git push origin v1.0.0
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