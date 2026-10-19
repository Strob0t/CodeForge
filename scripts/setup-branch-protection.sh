#!/usr/bin/env bash
# ---------------------------------------------------------------------------
# Configure GitHub branch protection rules for the main branch.
#
# Prerequisites:
#   - gh CLI installed and authenticated (gh auth login)
#   - Admin access to the repository
#
# Usage:
#   ./scripts/setup-branch-protection.sh
# ---------------------------------------------------------------------------

set -euo pipefail

REPO="Strob0t/CodeForge"
BRANCH="main"

echo "Configuring branch protection for ${REPO}@${BRANCH}..."

# Check gh auth
if ! gh auth status >/dev/null 2>&1; then
  echo "ERROR: gh is not authenticated. Run 'gh auth login' first."
  exit 1
fi

# Apply branch protection ruleset via the GitHub API.
# Uses the newer rulesets API (available on all plans including free).
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/${REPO}/branches/${BRANCH}/protection" \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Go",
      "Python",
      "Frontend"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "block_creations": false,
  "required_conversation_resolution": true
}
JSON

echo ""
echo "Branch protection configured for ${BRANCH}:"
echo "  - Require PR with 1 approving review"
echo "  - Dismiss stale reviews on new commits"
echo "  - Require status checks to pass (Go, Python, Frontend)"
echo "  - Require branches to be up to date before merging"
echo "  - Require linear history (no merge commits)"
echo "  - Require conversation resolution"
echo "  - No force pushes"
echo "  - No branch deletion"
echo ""
echo "Done."
