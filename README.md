# junit2jira

Convert test failures to jira issues

### Build
```shell
go build ./...
```

### Test
```shell
go test ./...
```

### Usage example
```shell
JIRA_TOKEN="..." junit2jira \
  -jira-url "https://..." \
  -junit-reports-dir "..." \
  -base-link "https://..." \
  -build-id "$BUILD_ID|GITHUB_RUN_ID" \
  -build-link "https://..." \
  -build-tag "$STACKROX_BUILD_TAG|$GITHUB_SHA" \
  -job-name "$JOB_NAME|$GITHUB_WORKFLOW" \
  -orchestrator "$ORCHESTRATOR_FLAVOR" \
  -timestamp $(date --rfc-3339=seconds)
  -csv -
```
