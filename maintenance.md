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