## `arcgate-go`
## Ark Media Platform "arcgate" headless gateway and name service for Go

---

### Deployment

  - Build the `arcgate` binary:
```
    % make build
```

- Add content items per "org" in `cmd/orgs/{AppDomain}`:
    - wildcard certificates files (`.crt` `.key`) 
    - `apple-app-site-association` (no `.json` extension; MIME type via served header)
    - `windows-app-web-link` (no `.json` extension; MIME type via served header)
    - `assetlinks.json` (for Android)

### TODO
  - Merge `archost` and `arcgate` into single binary? `arccell`? or just add as arcgate-go/cmd/arccell
