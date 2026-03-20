# Maintenance Guide

### Update dependent libraries

#### go.mod

Update `go.mod` by selecting `Upgrade direct dependencies` on VSCode and run `go mod tidy`.

#### package.json

```
pnpm update
pnpm run build
```

#### Makefile

Go to the site commented in the Makefile and manually rewrite the file if a newer version is available.

#### .github/workflows/ci.yaml

Update the version of `pnpm` to the latest one. in the `ci.yaml` file.