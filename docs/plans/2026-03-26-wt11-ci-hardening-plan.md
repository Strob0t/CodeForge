# Worktree 11: fix/ci-hardening — CI/CD Security

**Branch:** `fix/ci-hardening`
**Priority:** Hoch
**Scope:** 3 findings (F-INF-005, F-INF-009, F-INF-019)
**Estimated effort:** Small (1 day)

## Research Summary

- Trivy supply chain compromise (March 2026, CVE-2026-33634) — 75/76 tags poisoned
- tj-actions/changed-files compromise (March 2025, CVE-2025-30066) — 23K repos affected
- Grype (Anchore) safer than Trivy post-compromise; SBOM-first, Apache 2.0
- StepSecurity / pinact for automated SHA pinning
- GitHub now supports SHA pinning enforcement policy (August 2025)
- OpenSSF Scorecard: average score 3.5/10 for security projects

## Steps

### Phase 1: Permissions + SHA Pinning (immediate)

**1a. F-INF-019: Add top-level permissions to docker-build.yml**

```yaml
permissions: {}  # Default: no permissions. Jobs declare what they need.
```

**1b. F-INF-005: SHA-pin all 12 third-party actions**

Use `pinact` CLI for bulk conversion:
```bash
go install github.com/suzuki-shunsuke/pinact/v3/cmd/pinact@latest
pinact run
```

Result format:
```yaml
- uses: actions/checkout@692973e3d937129bcbf40652eb9f2f61becf3332 # v4.1.7
```

**Actions to pin (ci.yml):**
- `actions/checkout@v4`
- `actions/setup-go@v5`
- `actions/setup-python@v5`
- `actions/setup-node@v4`
- `golangci/golangci-lint-action@v7.0.0`
- `treosh/lighthouse-ci-action@v12.1.0`
- `actions/upload-artifact@v4`
- `anchore/sbom-action@v0`

**Actions to pin (docker-build.yml):**
- `actions/checkout@v4`
- `docker/setup-buildx-action@v3`
- `docker/login-action@v3`
- `docker/metadata-action@v5`
- `docker/build-push-action@v6`

Configure Dependabot or Renovate for ongoing SHA pin maintenance.

### Phase 2: Container Image Scanning

**F-INF-009: Add Grype scanning after each image build**

Add scan job to `docker-build.yml`:
```yaml
scan-images:
  needs: [build-core, build-worker, build-frontend]
  runs-on: ubuntu-latest
  permissions:
    contents: read
    security-events: write
  strategy:
    matrix:
      image: [codeforge-core, codeforge-worker, codeforge-frontend]
  steps:
    - uses: actions/checkout@<SHA>
    - name: Scan image
      uses: anchore/scan-action@<SHA>
      id: scan
      with:
        image: "ghcr.io/${{ github.repository_owner }}/${{ matrix.image }}:sha-${{ github.sha }}"
        fail-build: true
        severity-cutoff: critical
        output-format: sarif
    - name: Upload SARIF
      if: always()
      uses: github/codeql-action/upload-sarif@<SHA>
      with:
        sarif_file: ${{ steps.scan.outputs.sarif }}
```

Add `.grype.yaml` for accepted risk exceptions:
```yaml
ignore:
  - vulnerability: CVE-XXXX-XXXXX
    package:
      name: some-package
```

### Phase 3: Optional enhancements

- **StepSecurity Harden-Runner:** Add in `audit` mode first, then `block` mode
- **SLSA Provenance:** `slsa-github-generator` for container images
- **OpenSSF Scorecard:** Add to CI for ongoing security posture tracking
- Enable GitHub's SHA pinning enforcement policy at org level

## Verification

- Both workflows pass with SHA-pinned actions
- Image scan catches known CVEs (test with intentionally vulnerable base image)
- SARIF reports visible in GitHub Security tab
- `permissions: {}` at workflow level confirmed in both files

## Sources

- [StepSecurity: Pinning GitHub Actions](https://www.stepsecurity.io/blog/pinning-github-actions-for-enhanced-security-a-complete-guide)
- [CrowdStrike: Trivy Compromise Analysis](https://www.crowdstrike.com/en-us/blog/from-scanner-to-stealer-inside-the-trivy-action-supply-chain-compromise/)
- [Anchore Grype](https://github.com/anchore/grype)
- [GitHub: Actions Policy SHA Pinning](https://github.blog/changelog/2025-08-15-github-actions-policy-now-supports-blocking-and-sha-pinning-actions/)
- [OpenSSF Scorecard](https://scorecard.dev/)
